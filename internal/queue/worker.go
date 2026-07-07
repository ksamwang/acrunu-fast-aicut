package queue

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
)

type TestTaskHandler interface {
	HandleTestTask(ctx context.Context, payload TestTaskPayload) error
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
	return mux
}
