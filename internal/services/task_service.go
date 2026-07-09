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
	ID              string         `json:"id"`
	ProductID       string         `json:"product_id,omitempty"`
	CreatedByUserID string         `json:"created_by_user_id,omitempty"`
	TaskType        string         `json:"task_type"`
	Status          string         `json:"status"`
	PayloadSummary  map[string]any `json:"payload_summary,omitempty"`
	AssetID         string         `json:"asset_id,omitempty"`
	DurationMs      int64          `json:"duration_ms,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	RetryCount      int            `json:"retry_count"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
}

type TaskFilters struct {
	TaskType string
	Status   string
}

type TaskStore interface {
	CreateTestTask(ctx context.Context, userID string) (GenerationTask, error)
	CreateAssetExtractFramesTask(ctx context.Context, userID string, productID string, payload queue.AssetExtractFramesPayload) (GenerationTask, error)
	CreateAssetAnalyzeTask(ctx context.Context, userID string, productID string, payload queue.AssetAnalyzePayload) (GenerationTask, error)
	GetTask(ctx context.Context, taskID string) (GenerationTask, error)
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
	task, err := s.store.CreateTestTask(ctx, userID)
	if err != nil {
		return GenerationTask{}, err
	}
	return finalizeTask(task), nil
}

func (s *TaskService) CreateAssetExtractFramesTask(ctx context.Context, userID string, productID string, payload queue.AssetExtractFramesPayload) (GenerationTask, error) {
	task, err := s.store.CreateAssetExtractFramesTask(ctx, userID, productID, payload)
	if err != nil {
		return GenerationTask{}, err
	}
	return finalizeTask(task), nil
}

func (s *TaskService) CreateAssetAnalyzeTask(ctx context.Context, userID string, productID string, payload queue.AssetAnalyzePayload) (GenerationTask, error) {
	task, err := s.store.CreateAssetAnalyzeTask(ctx, userID, productID, payload)
	if err != nil {
		return GenerationTask{}, err
	}
	return finalizeTask(task), nil
}

func (s *TaskService) GetTask(ctx context.Context, taskID string) (GenerationTask, error) {
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return GenerationTask{}, err
	}
	return finalizeTask(task), nil
}

func (s *TaskService) ListTasks(ctx context.Context, filters TaskFilters) ([]GenerationTask, error) {
	tasks, err := s.store.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]GenerationTask, 0, len(tasks))
	for i := range tasks {
		task := finalizeTask(tasks[i])
		if filters.TaskType != "" && task.TaskType != filters.TaskType {
			continue
		}
		if filters.Status != "" && task.Status != filters.Status {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
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
	return s.createTask(userID, "", "test", map[string]any{"kind": "test"})
}

func (s *fileTaskStore) CreateAssetExtractFramesTask(_ context.Context, userID string, productID string, payload queue.AssetExtractFramesPayload) (GenerationTask, error) {
	summary := map[string]any{
		"asset_id":    payload.AssetID,
		"storage_key": payload.StorageKey,
		"duration_ms": payload.DurationMs,
	}
	if payload.Strategy.Mode != "" {
		summary["strategy_mode"] = payload.Strategy.Mode
	}
	if payload.Strategy.FrameCount > 0 {
		summary["frame_count"] = payload.Strategy.FrameCount
	}
	if payload.Strategy.KeyframeWindowMs > 0 {
		summary["keyframe_window_ms"] = payload.Strategy.KeyframeWindowMs
	}
	return s.createTask(userID, productID, "asset_extract_frames", summary)
}

func (s *fileTaskStore) CreateAssetAnalyzeTask(_ context.Context, userID string, productID string, payload queue.AssetAnalyzePayload) (GenerationTask, error) {
	return s.createTask(userID, productID, "asset_analyze", map[string]any{
		"asset_id": payload.AssetID,
	})
}

func (s *fileTaskStore) createTask(userID string, productID string, taskType string, payloadSummary map[string]any) (GenerationTask, error) {
	now := time.Now()
	task := GenerationTask{
		ID:              uuid.NewString(),
		ProductID:       productID,
		CreatedByUserID: userID,
		TaskType:        taskType,
		Status:          "queued",
		PayloadSummary:  payloadSummary,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return task, s.saveLocked()
}

func (s *fileTaskStore) GetTask(_ context.Context, taskID string) (GenerationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()

	task, ok := s.tasks[taskID]
	if !ok {
		return GenerationTask{}, ErrTaskNotFound
	}
	return task, nil
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

func finalizeTask(task GenerationTask) GenerationTask {
	if task.AssetID == "" && task.PayloadSummary != nil {
		if assetID, ok := task.PayloadSummary["asset_id"].(string); ok {
			task.AssetID = assetID
		}
	}

	if task.StartedAt != nil {
		end := time.Now()
		if task.FinishedAt != nil {
			end = *task.FinishedAt
		}
		if duration := end.Sub(*task.StartedAt); duration > 0 {
			task.DurationMs = duration.Milliseconds()
		}
	}

	return task
}
