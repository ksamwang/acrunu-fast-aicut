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

type TaskService struct {
	mu       sync.RWMutex
	filePath string
	tasks    map[string]GenerationTask
}

func NewTaskService(storageRoot string) *TaskService {
	service := &TaskService{
		filePath: filepath.Join(storageRoot, "temp", "tasks.json"),
		tasks:    map[string]GenerationTask{},
	}
	_ = service.load()
	return service
}

func (s *TaskService) CreateTestTask(userID string) GenerationTask {
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
	_ = s.saveLocked()
	return task
}

func (s *TaskService) ListTasks() []GenerationTask {
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
	return tasks
}

func (s *TaskService) MarkRunning(taskID string) error {
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

func (s *TaskService) MarkCompleted(taskID string) error {
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

func (s *TaskService) MarkFailed(taskID string, message string) error {
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

func (s *TaskService) HandleTestTask(ctx context.Context, payload queue.TestTaskPayload) error {
	if err := s.MarkRunning(payload.TaskID); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		_ = s.MarkFailed(payload.TaskID, ctx.Err().Error())
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}
	return s.MarkCompleted(payload.TaskID)
}

func (s *TaskService) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *TaskService) loadLocked() error {
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

func (s *TaskService) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, content, 0644)
}
