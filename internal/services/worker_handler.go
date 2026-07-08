package services

import (
	"context"

	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

type WorkerHandler struct {
	taskService            *TaskService
	assetProcessingService *AssetProcessingService
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
