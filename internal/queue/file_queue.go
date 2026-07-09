package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrFileQueueEmpty = errors.New("file queue empty")

type FileTask struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Attempt   int             `json:"attempt"`
	MaxRetry  int             `json:"max_retry"`
	CreatedAt time.Time       `json:"created_at"`
}

type FileQueue struct {
	filePath string
}

func NewFileQueue(storageRoot string) *FileQueue {
	return &FileQueue{filePath: filepath.Join(storageRoot, "temp", "queue.json")}
}

func (q *FileQueue) Enqueue(_ context.Context, taskType string, payload []byte, maxRetry int) error {
	tasks, err := q.read()
	if err != nil {
		return err
	}
	if maxRetry < 0 {
		maxRetry = 0
	}
	tasks = append(tasks, FileTask{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:      taskType,
		Payload:   append([]byte(nil), payload...),
		Attempt:   0,
		MaxRetry:  maxRetry,
		CreatedAt: time.Now(),
	})
	return q.write(tasks)
}

func (q *FileQueue) Dequeue(_ context.Context) (FileTask, error) {
	tasks, err := q.read()
	if err != nil {
		return FileTask{}, err
	}
	if len(tasks) == 0 {
		return FileTask{}, ErrFileQueueEmpty
	}
	task := tasks[0]
	return task, q.write(tasks[1:])
}

func (q *FileQueue) Requeue(_ context.Context, task FileTask) error {
	tasks, err := q.read()
	if err != nil {
		return err
	}
	task.Attempt++
	tasks = append(tasks, task)
	return q.write(tasks)
}

func (q *FileQueue) read() ([]FileTask, error) {
	content, err := os.ReadFile(q.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(content) == 0 {
		return nil, nil
	}
	var tasks []FileTask
	if err := json.Unmarshal(content, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (q *FileQueue) write(tasks []FileTask) error {
	if err := os.MkdirAll(filepath.Dir(q.filePath), 0755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.filePath, content, 0644)
}
