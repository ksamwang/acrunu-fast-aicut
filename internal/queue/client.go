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
	payload, err := json.Marshal(TestTaskPayload{TaskID: taskID})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeTestTask, payload)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeTestTask, payload))
	return err
}

func (c *Client) EnqueueAssetExtractFrames(assetID string, storageKey string, durationMs int) error {
	payload, err := json.Marshal(AssetExtractFramesPayload{
		AssetID:    assetID,
		StorageKey: storageKey,
		DurationMs: durationMs,
	})
	if err != nil {
		return err
	}

	if c.backend == "file" {
		return c.file.Enqueue(context.Background(), TypeAssetExtractFrames, payload)
	}
	if c.client == nil {
		return fmt.Errorf("queue client is not initialized")
	}
	_, err = c.client.Enqueue(asynq.NewTask(TypeAssetExtractFrames, payload))
	return err
}
