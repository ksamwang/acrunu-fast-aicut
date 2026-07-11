package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type Client struct {
	backend string
	client  *asynq.Client
	file    *FileQueue
}

func NewClient(redisAddr string, backend string, storageRoot string) *Client {
	if backend == "file" {
		return &Client{
			backend: backend,
			file:    NewFileQueue(storageRoot),
		}
	}
	return &Client{
		backend: "redis",
		client:  asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Client) EnqueueTestTask(taskID string) error {
	encodedPayload, err := json.Marshal(TestTaskPayload{TaskID: taskID})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeTestTask, encodedPayload, 0)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeTestTask, encodedPayload))
	return err
}

func (c *Client) EnqueueAssetExtractFrames(payload AssetExtractFramesPayload) error {
	if payload.TaskID == "" {
		return fmt.Errorf("task id is required")
	}
	encodedPayload, err := json.Marshal(AssetExtractFramesPayload{
		TaskID:      payload.TaskID,
		AssetID:     payload.AssetID,
		StorageKey:  payload.StorageKey,
		DurationMs:  payload.DurationMs,
		Strategy:    payload.Strategy,
		SkipAnalyze: payload.SkipAnalyze,
	})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeAssetExtractFrames, encodedPayload, 3)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeAssetExtractFrames, encodedPayload), asynq.MaxRetry(3))
	return err
}

func (c *Client) EnqueueAssetAnalyze(taskID string, assetID string) error {
	encodedPayload, err := json.Marshal(AssetAnalyzePayload{TaskID: taskID, AssetID: assetID})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeAssetAnalyze, encodedPayload, 3)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeAssetAnalyze, encodedPayload), asynq.MaxRetry(3))
	return err
}

func (c *Client) EnqueueAssetEmbedding(taskID string, assetID string) error {
	encodedPayload, err := json.Marshal(AssetEmbeddingPayload{TaskID: taskID, AssetID: assetID})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeAssetEmbedding, encodedPayload, 3)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeAssetEmbedding, encodedPayload), asynq.MaxRetry(3))
	return err
}
