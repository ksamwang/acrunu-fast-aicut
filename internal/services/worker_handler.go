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
}

func (h *WorkerHandler) WithVoiceoverService(voiceoverService *VoiceoverService) *WorkerHandler {
	h.voiceoverService = voiceoverService
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
	return h.runVoiceTask(ctx, payload.TaskID, "voiceover_generate", func(runCtx context.Context) error {
		if h.voiceoverService == nil {
			return fmt.Errorf("voiceover service is not configured")
		}
		return h.voiceoverService.ProcessVoiceoverGenerate(runCtx, payload)
	})
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
