package services

import (
	"context"
	"fmt"
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
	EnqueueAssetAnalyze(assetID string) error
}

type AssetProcessingService struct {
	storageRoot          string
	localStore           *storage.LocalStore
	productAssetService  *ProductAssetService
	queueClient          assetAnalysisTaskEnqueuer
	analyzer             modelgateway.AssetAnalyzer
}

func NewAssetProcessingService(
	storageRoot string,
	productAssetService *ProductAssetService,
	queueClient assetAnalysisTaskEnqueuer,
	analyzer modelgateway.AssetAnalyzer,
) *AssetProcessingService {
	if analyzer == nil {
		analyzer = modelgateway.NewMockAssetAnalyzer()
	}

	return &AssetProcessingService{
		storageRoot:         storageRoot,
		localStore:          storage.NewLocalStore(storageRoot),
		productAssetService: productAssetService,
		queueClient:         queueClient,
		analyzer:            analyzer,
	}
}

func (s *AssetProcessingService) HandleAssetExtractFrames(ctx context.Context, payload queue.AssetExtractFramesPayload) error {
	inputPath := s.localStore.FullPath(payload.StorageKey)
	outputDir := filepath.Join(s.storageRoot, "frames", payload.AssetID)
	timestamps := buildFrameTimestamps(payload.DurationMs, 3)

	frames, err := ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestamps)
	if err != nil {
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
		return s.queueClient.EnqueueAssetAnalyze(payload.AssetID)
	}
	return nil
}

func (s *AssetProcessingService) HandleAssetAnalyze(ctx context.Context, payload queue.AssetAnalyzePayload) error {
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
		return nil
	}

	update := AssetAnalysisUpdateFromResult(result, "ready", time.Now())
	update.AnalysisError = ""
	return s.productAssetService.UpdateAssetAnalysis(asset.ID, update)
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
