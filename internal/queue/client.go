package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

type Client struct {
	client *asynq.Client
}

func NewClient(redisAddr string) *Client {
	return &Client{
		client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueTestTask(taskID string) error {
	payload, err := json.Marshal(TestTaskPayload{TaskID: taskID})
	if err != nil {
		return err
	}

	_, err = c.client.Enqueue(asynq.NewTask(TypeTestTask, payload))
	return err
}
