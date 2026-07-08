package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

var ErrTaskNotFound = errors.New("task not found")

type GenerationTask struct {
	ID              string     `json:"id"`
	ProductID       string     `json:"product_id,omitempty"`
	CreatedByUserID string     `json:"created_by_user_id,omitempty"`
	TaskType        string     `json:"task_type"`
	Status          string     `json:"status"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	RetryCount      int        `json:"retry_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type TaskStore interface {
	CreateTestTask(ctx context.Context, userID string) (GenerationTask, error)
	ListTasks(ctx context.Context) ([]GenerationTask, error)
	MarkRunning(ctx context.Context, taskID string) error
	MarkCompleted(ctx context.Context, taskID string) error
	MarkFailed(ctx context.Context, taskID string, message string) error
}

type TaskService struct {
	store TaskStore
}

type fileTaskStore struct {
	mu       sync.RWMutex
	filePath string
	tasks    map[string]GenerationTask
}

func NewTaskService(storageRoot string) *TaskService {
	store := &fileTaskStore{
		filePath: filepath.Join(storageRoot, "temp", "tasks.json"),
		tasks:    map[string]GenerationTask{},
	}
	_ = store.load()
	return NewTaskServiceWithStore(store)
}

func NewTaskServiceWithStore(store TaskStore) *TaskService {
	return &TaskService{store: store}
}

func (s *TaskService) CreateTestTask(ctx context.Context, userID string) (GenerationTask, error) {
	return s.store.CreateTestTask(ctx, userID)
}

func (s *TaskService) ListTasks(ctx context.Context) ([]GenerationTask, error) {
	return s.store.ListTasks(ctx)
}

func (s *TaskService) MarkRunning(ctx context.Context, taskID string) error {
	return s.store.MarkRunning(ctx, taskID)
}

func (s *TaskService) MarkCompleted(ctx context.Context, taskID string) error {
	return s.store.MarkCompleted(ctx, taskID)
}

func (s *TaskService) MarkFailed(ctx context.Context, taskID string, message string) error {
	return s.store.MarkFailed(ctx, taskID, message)
}

func (s *TaskService) HandleTestTask(ctx context.Context, payload queue.TestTaskPayload) error {
	if err := s.MarkRunning(ctx, payload.TaskID); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		_ = s.MarkFailed(context.Background(), payload.TaskID, ctx.Err().Error())
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}
	return s.MarkCompleted(ctx, payload.TaskID)
}

func (s *fileTaskStore) CreateTestTask(_ context.Context, userID string) (GenerationTask, error) {
	now := time.Now()
	task := GenerationTask{
		ID:              uuid.NewString(),
		CreatedByUserID: userID,
		TaskType:        "test",
		Status:          "queued",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return task, s.saveLocked()
}

func (s *fileTaskStore) ListTasks(_ context.Context) ([]GenerationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()

	tasks := make([]GenerationTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	return tasks, nil
}

func (s *fileTaskStore) MarkRunning(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	now := time.Now()
	task.Status = "running"
	task.StartedAt = &now
	task.UpdatedAt = now
	s.tasks[taskID] = task
	return s.saveLocked()
}

func (s *fileTaskStore) MarkCompleted(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	now := time.Now()
	task.Status = "completed"
	task.FinishedAt = &now
	task.UpdatedAt = now
	s.tasks[taskID] = task
	return s.saveLocked()
}

func (s *fileTaskStore) MarkFailed(_ context.Context, taskID string, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	now := time.Now()
	task.Status = "failed"
	task.ErrorMessage = message
	task.RetryCount++
	task.FinishedAt = &now
	task.UpdatedAt = now
	s.tasks[taskID] = task
	return s.saveLocked()
}

func (s *fileTaskStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *fileTaskStore) loadLocked() error {
	content, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(content) == 0 {
		return nil
	}
	return json.Unmarshal(content, &s.tasks)
}

func (s *fileTaskStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, content, 0644)
}
