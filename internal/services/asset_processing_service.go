package services

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
	"github.com/ksamwang/acrunu-fast-aicut/internal/storage"
)

type assetAnalysisTaskEnqueuer interface {
	EnqueueAssetAnalyze(taskID string, assetID string) error
}

const (
	keyframeFallbackFrameCount = 3
	keyframeWindowMs           = 500
)

type AssetProcessingService struct {
	storageRoot         string
	localStore          *storage.LocalStore
	productAssetService *ProductAssetService
	taskService         *TaskService
	queueClient         assetAnalysisTaskEnqueuer
	analyzer            modelgateway.AssetAnalyzer
	logger              *slog.Logger
}

func NewAssetProcessingService(
	storageRoot string,
	productAssetService *ProductAssetService,
	taskService *TaskService,
	queueClient assetAnalysisTaskEnqueuer,
	analyzer modelgateway.AssetAnalyzer,
	logger *slog.Logger,
) *AssetProcessingService {
	if analyzer == nil {
		analyzer = modelgateway.NewMockAssetAnalyzer()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &AssetProcessingService{
		storageRoot:         storageRoot,
		localStore:          storage.NewLocalStore(storageRoot),
		productAssetService: productAssetService,
		taskService:         taskService,
		queueClient:         queueClient,
		analyzer:            analyzer,
		logger:              logger,
	}
}

func (s *AssetProcessingService) HandleAssetExtractFrames(ctx context.Context, payload queue.AssetExtractFramesPayload) error {
	return s.runTrackedTask(ctx, payload.TaskID, payload.AssetID, "asset_extract_frames", func(runCtx context.Context) error {
		return s.handleAssetExtractFrames(runCtx, payload)
	})
}

func (s *AssetProcessingService) handleAssetExtractFrames(ctx context.Context, payload queue.AssetExtractFramesPayload) error {
	inputPath := s.localStore.FullPath(payload.StorageKey)
	outputDir := filepath.Join(s.storageRoot, "frames", payload.AssetID)
	strategy := normalizeFrameExtractionStrategy(payload.DurationMs, payload.Strategy)
	timestamps := resolveFrameTimestamps(payload.DurationMs, strategy)

	frames, err := ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestamps)
	if err != nil {
		if s.productAssetService != nil {
			updateErr := s.productAssetService.UpdateAssetAnalysis(payload.AssetID, AssetAnalysisUpdate{
				AnalysisStatus:  "failed",
				UsabilityStatus: "needs_review",
				AnalysisError:   err.Error(),
				AnalyzedAt:      time.Now(),
			})
			if updateErr != nil {
				return fmt.Errorf("extract frames failed: %v; failed to persist analysis error: %w", err, updateErr)
			}
		}
		return err
	}

	if s.productAssetService != nil && s.productAssetService.queries != nil {
		queries := s.productAssetService.queries
		assetID := assetNullableUUIDParam(payload.AssetID)
		if err := queries.DeleteAssetFrameSnapshotsByAsset(ctx, assetID); err != nil {
			return err
		}

		for _, frame := range frames {
			storageKey := filepath.ToSlash(filepath.Join("frames", payload.AssetID, filepath.Base(frame.OutputPath)))
			if _, err := queries.UpsertAssetFrameSnapshot(ctx, db.UpsertAssetFrameSnapshotParams{
				AssetID:     assetID,
				FrameIndex:  int32(frame.FrameIndex),
				TimestampMs: int32(frame.TimestampMs),
				StorageKey:  storageKey,
				Width:       pgtype.Int4{},
				Height:      pgtype.Int4{},
			}); err != nil {
				return err
			}
		}
	}

	if s.queueClient != nil {
		asset, ok := s.productAssetService.GetAsset(payload.AssetID)
		if !ok {
			return ErrAssetNotFound
		}

		analyzeTaskID := ""
		if s.taskService != nil {
			analyzeTask, err := s.taskService.CreateAssetAnalyzeTask(ctx, asset.CreatedByUserID, asset.ProductID, queue.AssetAnalyzePayload{
				AssetID: payload.AssetID,
			})
			if err != nil {
				return err
			}
			analyzeTaskID = analyzeTask.ID
		}

		if err := s.queueClient.EnqueueAssetAnalyze(analyzeTaskID, payload.AssetID); err != nil {
			if analyzeTaskID != "" && s.taskService != nil {
				_ = s.taskService.MarkFailed(context.Background(), analyzeTaskID, err.Error())
			}
			return err
		}
	}
	return nil
}

func (s *AssetProcessingService) HandleAssetAnalyze(ctx context.Context, payload queue.AssetAnalyzePayload) error {
	return s.runTrackedTask(ctx, payload.TaskID, payload.AssetID, "asset_analyze", func(runCtx context.Context) error {
		return s.handleAssetAnalyze(runCtx, payload)
	})
}

func (s *AssetProcessingService) handleAssetAnalyze(ctx context.Context, payload queue.AssetAnalyzePayload) error {
	if s.productAssetService == nil {
		return nil
	}

	asset, ok := s.productAssetService.GetAsset(payload.AssetID)
	if !ok {
		return ErrAssetNotFound
	}

	frameSnapshots := s.productAssetService.ListAssetFrameSnapshots(payload.AssetID)
	frames := make([]modelgateway.FrameReference, 0, len(frameSnapshots))
	for _, frame := range frameSnapshots {
		frames = append(frames, modelgateway.FrameReference{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: frame.TimestampMs,
			StorageKey:  frame.StorageKey,
		})
	}

	result, err := s.analyzer.AnalyzeAsset(ctx, modelgateway.AnalyzeAssetInput{
		AssetID:        asset.ID,
		SourceType:     asset.SourceType,
		DurationMs:     asset.DurationMs,
		Width:          asset.Width,
		Height:         asset.Height,
		HasAudio:       asset.HasAudio,
		AudioCodec:     asset.AudioCodec,
		FrameSnapshots: frames,
	})
	if err != nil {
		updateErr := s.productAssetService.UpdateAssetAnalysis(asset.ID, AssetAnalysisUpdate{
			AnalysisStatus:  "failed",
			UsabilityStatus: "needs_review",
			AnalysisError:   err.Error(),
			AnalyzedAt:      time.Now(),
		})
		if updateErr != nil {
			return fmt.Errorf("analysis failed: %v; failed to persist analysis error: %w", err, updateErr)
		}
		return err
	}

	update := AssetAnalysisUpdateFromResult(result, "ready", time.Now())
	update.AnalysisError = ""
	return s.productAssetService.UpdateAssetAnalysis(asset.ID, update)
}

func (s *AssetProcessingService) runTrackedTask(ctx context.Context, taskID string, assetID string, taskType string, run func(context.Context) error) error {
	startedAt := time.Now()
	s.logger.Info("worker task started",
		slog.String("task_type", taskType),
		slog.String("task_id", taskID),
		slog.String("asset_id", assetID),
	)

	if taskID != "" && s.taskService != nil {
		currentTask, err := s.taskService.GetTask(ctx, taskID)
		if err == nil && currentTask.Status == "completed" {
			s.logger.Info("worker task skipped because it is already completed",
				slog.String("task_type", taskType),
				slog.String("task_id", taskID),
				slog.String("asset_id", assetID),
			)
			return nil
		}
		if err := s.taskService.MarkRunning(ctx, taskID); err != nil {
			s.logger.Error("failed to mark task running",
				slog.String("task_type", taskType),
				slog.String("task_id", taskID),
				slog.String("asset_id", assetID),
				slog.String("error", err.Error()),
			)
			return err
		}
	}

	if err := run(ctx); err != nil {
		if taskID != "" && s.taskService != nil {
			if markErr := s.taskService.MarkFailed(context.Background(), taskID, err.Error()); markErr != nil {
				s.logger.Error("failed to mark task failed",
					slog.String("task_type", taskType),
					slog.String("task_id", taskID),
					slog.String("asset_id", assetID),
					slog.String("error", markErr.Error()),
				)
				return markErr
			}
		}
		s.logger.Error("worker task failed",
			slog.String("task_type", taskType),
			slog.String("task_id", taskID),
			slog.String("asset_id", assetID),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			slog.String("error", err.Error()),
		)
		return err
	}

	if taskID != "" && s.taskService != nil {
		if err := s.taskService.MarkCompleted(ctx, taskID); err != nil {
			s.logger.Error("failed to mark task completed",
				slog.String("task_type", taskType),
				slog.String("task_id", taskID),
				slog.String("asset_id", assetID),
				slog.String("error", err.Error()),
			)
			return err
		}
	}

	s.logger.Info("worker task completed",
		slog.String("task_type", taskType),
		slog.String("task_id", taskID),
		slog.String("asset_id", assetID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	return nil
}

func defaultFrameExtractionStrategy(durationMs int) queue.FrameExtractionStrategy {
	return queue.FrameExtractionStrategy{
		Mode:       queue.FrameExtractionModeFixedInterval,
		FrameCount: frameCountForDuration(durationMs),
	}
}

func normalizeFrameExtractionStrategy(durationMs int, strategy queue.FrameExtractionStrategy) queue.FrameExtractionStrategy {
	if strategy.Mode == "" {
		return defaultFrameExtractionStrategy(durationMs)
	}
	switch strategy.Mode {
	case queue.FrameExtractionModeFixedInterval:
		if strategy.FrameCount <= 0 {
			strategy.FrameCount = frameCountForDuration(durationMs)
		}
		return strategy
	case queue.FrameExtractionModeKeyframe:
		if strategy.FrameCount <= 0 {
			strategy.FrameCount = keyframeFallbackFrameCount
		}
		if strategy.KeyframeWindowMs <= 0 {
			strategy.KeyframeWindowMs = keyframeWindowMs
		}
		return strategy
	default:
		return defaultFrameExtractionStrategy(durationMs)
	}
}

func resolveFrameTimestamps(durationMs int, strategy queue.FrameExtractionStrategy) []int {
	switch strategy.Mode {
	case queue.FrameExtractionModeKeyframe:
		return buildFrameTimestamps(durationMs, strategy.FrameCount)
	default:
		return buildFrameTimestamps(durationMs, strategy.FrameCount)
	}
}

func frameCountForDuration(durationMs int) int {
	switch {
	case durationMs <= 0:
		return 1
	case durationMs <= 1500:
		return 1
	case durationMs <= 5000:
		return 3
	case durationMs <= 15000:
		return 5
	default:
		return 7
	}
}

func buildFrameTimestamps(durationMs int, frameCount int) []int {
	if frameCount <= 0 {
		return nil
	}
	if durationMs <= 0 {
		return []int{0}
	}
	if frameCount == 1 {
		return []int{durationMs / 2}
	}

	timestamps := make([]int, 0, frameCount)
	step := durationMs / (frameCount + 1)
	for i := 1; i <= frameCount; i++ {
		timestamps = append(timestamps, i*step)
	}
	return timestamps
}
