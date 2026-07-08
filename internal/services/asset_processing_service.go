package services

import (
	"context"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
	"github.com/ksamwang/acrunu-fast-aicut/internal/storage"
)

type AssetProcessingService struct {
	storageRoot string
	localStore  *storage.LocalStore
	queries     *db.Queries
}

func NewAssetProcessingService(storageRoot string, queries *db.Queries) *AssetProcessingService {
	return &AssetProcessingService{
		storageRoot: storageRoot,
		localStore:  storage.NewLocalStore(storageRoot),
		queries:     queries,
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
	if s.queries == nil {
		return nil
	}

	assetID := assetNullableUUIDParam(payload.AssetID)
	if err := s.queries.DeleteAssetFrameSnapshotsByAsset(ctx, assetID); err != nil {
		return err
	}

	for _, frame := range frames {
		storageKey := filepath.ToSlash(filepath.Join("frames", payload.AssetID, filepath.Base(frame.OutputPath)))
		if _, err := s.queries.UpsertAssetFrameSnapshot(ctx, db.UpsertAssetFrameSnapshotParams{
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

	return nil
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
