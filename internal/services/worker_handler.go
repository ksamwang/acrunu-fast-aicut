package services

import (
	"context"
	"fmt"

	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

type WorkerHandler struct {
	taskService            *TaskService
	assetProcessingService *AssetProcessingService
	voiceoverService       *VoiceoverService
	generationRunService   *GenerationRunService
	generationPlanning     *GenerationPlanningService
	queueClient            *queue.Client
}

func (h *WorkerHandler) WithVoiceoverService(voiceoverService *VoiceoverService) *WorkerHandler {
	h.voiceoverService = voiceoverService
	return h
}

func (h *WorkerHandler) WithGenerationPipeline(runs *GenerationRunService, planning *GenerationPlanningService, queueClient *queue.Client) *WorkerHandler {
	h.generationRunService = runs
	h.generationPlanning = planning
	h.queueClient = queueClient
	return h
}

func NewWorkerHandler(taskService *TaskService, assetProcessingService *AssetProcessingService) *WorkerHandler {
	return &WorkerHandler{
		taskService:            taskService,
		assetProcessingService: assetProcessingService,
	}
}

func (h *WorkerHandler) HandleTestTask(ctx context.Context, payload queue.TestTaskPayload) error {
	return h.taskService.HandleTestTask(ctx, payload)
}

func (h *WorkerHandler) HandleAssetExtractFrames(ctx context.Context, payload queue.AssetExtractFramesPayload) error {
	if h.assetProcessingService == nil {
		return nil
	}
	return h.assetProcessingService.HandleAssetExtractFrames(ctx, payload)
}

func (h *WorkerHandler) HandleAssetAnalyze(ctx context.Context, payload queue.AssetAnalyzePayload) error {
	if h.assetProcessingService == nil {
		return nil
	}
	return h.assetProcessingService.HandleAssetAnalyze(ctx, payload)
}

func (h *WorkerHandler) HandleAssetEmbedding(ctx context.Context, payload queue.AssetEmbeddingPayload) error {
	if h.assetProcessingService == nil {
		return nil
	}
	return h.assetProcessingService.HandleAssetEmbedding(ctx, payload)
}

func (h *WorkerHandler) HandleVoiceProfilePreview(ctx context.Context, payload queue.VoiceProfilePreviewPayload) error {
	return h.runVoiceTask(ctx, payload.TaskID, "voice_profile_preview", func(runCtx context.Context) error {
		if h.voiceoverService == nil {
			return fmt.Errorf("voiceover service is not configured")
		}
		return h.voiceoverService.ProcessVoiceProfilePreview(runCtx, payload.VoiceProfileID)
	})
}

func (h *WorkerHandler) HandleVoiceAudition(ctx context.Context, payload queue.VoiceAuditionPayload) error {
	return h.runVoiceTask(ctx, payload.TaskID, "voice_audition", func(runCtx context.Context) error {
		if h.voiceoverService == nil {
			return fmt.Errorf("voiceover service is not configured")
		}
		return h.voiceoverService.ProcessVoiceAudition(runCtx, payload.AuditionID)
	})
}

func (h *WorkerHandler) HandleVoiceoverGenerate(ctx context.Context, payload queue.VoiceoverGeneratePayload) error {
	if payload.GenerationRunID != "" && h.generationRunService != nil {
		_ = h.generationRunService.UpdateStage(ctx, payload.GenerationRunID, generationRunStageVoicing, 16)
	}
	err := h.runVoiceTask(ctx, payload.TaskID, "voiceover_generate", func(runCtx context.Context) error {
		if h.voiceoverService == nil {
			return fmt.Errorf("voiceover service is not configured")
		}
		return h.voiceoverService.ProcessVoiceoverGenerate(runCtx, payload)
	})
	if err != nil {
		h.markGenerationRunFailed(payload.GenerationRunID, err)
		return err
	}
	return h.enqueueEditPlan(ctx, payload)
}

func (h *WorkerHandler) HandleEditPlanGenerate(ctx context.Context, payload queue.EditPlanGeneratePayload) error {
	err := h.runVoiceTask(ctx, payload.TaskID, "edit_plan_generate", func(runCtx context.Context) error {
		if h.generationPlanning == nil {
			return fmt.Errorf("generation planning service is not configured")
		}
		if h.voiceoverService != nil && h.generationRunService != nil && payload.GenerationRunID != "" {
			run, err := h.generationRunService.Get(runCtx, payload.GenerationRunID)
			if err != nil {
				return err
			}
			if err := h.voiceoverService.EnsureCurrentNarrationSegments(runCtx, run.VoiceoverTaskID); err != nil {
				return err
			}
		}
		_, err := h.generationPlanning.Generate(runCtx, GenerateEditPlanInput{
			GenerationRunID: payload.GenerationRunID,
			ScriptVariantID: payload.ScriptVariantID,
			VoiceoverID:     payload.VoiceoverID,
		})
		return err
	})
	if err != nil {
		h.markGenerationRunFailed(payload.GenerationRunID, err)
	}
	return err
}

func (h *WorkerHandler) enqueueEditPlan(ctx context.Context, payload queue.VoiceoverGeneratePayload) error {
	if payload.GenerationRunID == "" {
		return nil
	}
	if h.generationRunService == nil || h.taskService == nil || h.queueClient == nil {
		err := fmt.Errorf("generation pipeline is not configured")
		h.markGenerationRunFailed(payload.GenerationRunID, err)
		return err
	}
	if _, exists, err := h.generationRunService.FindTaskByStage(ctx, payload.GenerationRunID, generationRunTaskStageEditPlan); err != nil {
		return err
	} else if exists {
		return nil
	}
	run, err := h.generationRunService.Get(ctx, payload.GenerationRunID)
	if err != nil {
		return err
	}
	if err := h.generationRunService.UpdateStage(ctx, run.ID, generationRunStageRetrieving, 76); err != nil {
		return err
	}
	task, err := h.taskService.CreateEditPlanGenerateTask(ctx, run.CreatedByUserID, run.ProductID, queue.EditPlanGeneratePayload{
		GenerationRunID: run.ID,
		ScriptVariantID: payload.ScriptVariantID,
		VoiceoverID:     payload.VoiceoverID,
	})
	if err != nil {
		h.markGenerationRunFailed(run.ID, err)
		return err
	}
	if err := h.generationRunService.LinkTask(ctx, run.ID, task.ID, generationRunTaskStageEditPlan); err != nil {
		_ = h.taskService.MarkFailed(context.Background(), task.ID, err.Error())
		h.markGenerationRunFailed(run.ID, err)
		return err
	}
	planPayload := queue.EditPlanGeneratePayload{
		TaskID:          task.ID,
		GenerationRunID: run.ID,
		ScriptVariantID: payload.ScriptVariantID,
		VoiceoverID:     payload.VoiceoverID,
	}
	if err := h.queueClient.EnqueueEditPlanGenerate(planPayload); err != nil {
		_ = h.taskService.MarkFailed(context.Background(), task.ID, err.Error())
		h.markGenerationRunFailed(run.ID, err)
		return err
	}
	return nil
}

func (h *WorkerHandler) markGenerationRunFailed(runID string, cause error) {
	if h.generationRunService == nil || runID == "" || cause == nil {
		return
	}
	if err := h.generationRunService.MarkFailed(context.Background(), runID, cause); err != nil {
		return
	}
}

func (h *WorkerHandler) runVoiceTask(ctx context.Context, taskID string, taskType string, run func(context.Context) error) error {
	if h.taskService != nil && taskID != "" {
		task, err := h.taskService.GetTask(ctx, taskID)
		if err != nil {
			return err
		}
		if task.Status == "completed" {
			return nil
		}
		if err := h.taskService.MarkRunning(ctx, taskID); err != nil {
			return err
		}
	}

	err := run(ctx)
	if err != nil {
		if h.taskService != nil && taskID != "" {
			if markErr := h.taskService.MarkFailed(context.Background(), taskID, err.Error()); markErr != nil {
				return markErr
			}
		}
		return fmt.Errorf("%s: %w", taskType, err)
	}
	if h.taskService != nil && taskID != "" {
		if err := h.taskService.MarkCompleted(ctx, taskID); err != nil {
			return err
		}
	}
	return nil
}
