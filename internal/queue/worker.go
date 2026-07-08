package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

type TestTaskHandler interface {
	HandleTestTask(ctx context.Context, payload TestTaskPayload) error
	HandleAssetExtractFrames(ctx context.Context, payload AssetExtractFramesPayload) error
	HandleAssetAnalyze(ctx context.Context, payload AssetAnalyzePayload) error
}

func NewServer(redisAddr string, concurrency int) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{Concurrency: concurrency},
	)
}

func NewServeMux(handler TestTaskHandler) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeTestTask, func(ctx context.Context, task *asynq.Task) error {
		var payload TestTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		return handler.HandleTestTask(ctx, payload)
	})
	mux.HandleFunc(TypeAssetExtractFrames, func(ctx context.Context, task *asynq.Task) error {
		var payload AssetExtractFramesPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		return handler.HandleAssetExtractFrames(ctx, payload)
	})
	mux.HandleFunc(TypeAssetAnalyze, func(ctx context.Context, task *asynq.Task) error {
		var payload AssetAnalyzePayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		return handler.HandleAssetAnalyze(ctx, payload)
	})
	return mux
}

func RunFileWorker(ctx context.Context, storageRoot string, handler TestTaskHandler, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	fileQueue := NewFileQueue(storageRoot)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		task, err := fileQueue.Dequeue(ctx)
		if err != nil {
			if !errors.Is(err, ErrFileQueueEmpty) {
				return err
			}
		} else if err := handleFileTask(ctx, task, handler); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func handleFileTask(ctx context.Context, task FileTask, handler TestTaskHandler) error {
	switch task.Type {
	case TypeTestTask:
		var payload TestTaskPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return err
		}
		return handler.HandleTestTask(ctx, payload)
	case TypeAssetExtractFrames:
		var payload AssetExtractFramesPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return err
		}
		return handler.HandleAssetExtractFrames(ctx, payload)
	case TypeAssetAnalyze:
		var payload AssetAnalyzePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return err
		}
		return handler.HandleAssetAnalyze(ctx, payload)
	default:
		return nil
	}
}
