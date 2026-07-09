package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type PostgresTaskStore struct {
	queries *db.Queries
}

func NewPostgresTaskStore(queries *db.Queries) *PostgresTaskStore {
	return &PostgresTaskStore{queries: queries}
}

func (s *PostgresTaskStore) CreateTestTask(ctx context.Context, userID string) (GenerationTask, error) {
	return s.createTask(ctx, userID, "", "test", map[string]any{"kind": "test"})
}

func (s *PostgresTaskStore) CreateAssetExtractFramesTask(ctx context.Context, userID string, productID string, payload queue.AssetExtractFramesPayload) (GenerationTask, error) {
	return s.createTask(ctx, userID, productID, "asset_extract_frames", map[string]any{
		"asset_id":    payload.AssetID,
		"storage_key": payload.StorageKey,
		"duration_ms": payload.DurationMs,
	})
}

func (s *PostgresTaskStore) CreateAssetAnalyzeTask(ctx context.Context, userID string, productID string, payload queue.AssetAnalyzePayload) (GenerationTask, error) {
	return s.createTask(ctx, userID, productID, "asset_analyze", map[string]any{
		"asset_id": payload.AssetID,
	})
}

func (s *PostgresTaskStore) createTask(ctx context.Context, userID string, productID string, taskType string, payloadSummary map[string]any) (GenerationTask, error) {
	snapshot, err := json.Marshal(payloadSummary)
	if err != nil {
		return GenerationTask{}, err
	}

	task, err := s.queries.CreateGenerationTask(ctx, db.CreateGenerationTaskParams{
		ProductID:        nullableUUIDParam(productID),
		CreatedByUserID:  nullableUUIDParam(userID),
		TaskType:         taskType,
		Status:           "queued",
		VariantCount:     1,
		TargetDurationMs: pgtype.Int4{},
		StylePrompt:      pgtype.Text{},
		ConfigSnapshot:   snapshot,
	})
	if err != nil {
		return GenerationTask{}, err
	}
	return generationTaskFromDB(task), nil
}

func (s *PostgresTaskStore) GetTask(ctx context.Context, taskID string) (GenerationTask, error) {
	id, err := uuidParam(taskID)
	if err != nil {
		return GenerationTask{}, err
	}

	row, err := s.queries.GetGenerationTaskByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GenerationTask{}, ErrTaskNotFound
		}
		return GenerationTask{}, err
	}
	return generationTaskFromDB(row), nil
}

func (s *PostgresTaskStore) ListTasks(ctx context.Context) ([]GenerationTask, error) {
	rows, err := s.queries.ListGenerationTasks(ctx, pgtype.Text{})
	if err != nil {
		return nil, err
	}
	tasks := make([]GenerationTask, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, generationTaskFromDB(row))
	}
	return tasks, nil
}

func (s *PostgresTaskStore) MarkRunning(ctx context.Context, taskID string) error {
	return s.updateStatus(ctx, taskID, "running", "")
}

func (s *PostgresTaskStore) MarkCompleted(ctx context.Context, taskID string) error {
	return s.updateStatus(ctx, taskID, "completed", "")
}

func (s *PostgresTaskStore) MarkFailed(ctx context.Context, taskID string, message string) error {
	return s.updateStatus(ctx, taskID, "failed", message)
}

func (s *PostgresTaskStore) updateStatus(ctx context.Context, taskID string, status string, message string) error {
	id, err := uuidParam(taskID)
	if err != nil {
		return err
	}
	return s.queries.UpdateGenerationTaskStatus(ctx, db.UpdateGenerationTaskStatusParams{
		ID:           id,
		Status:       status,
		ErrorMessage: textParam(message),
	})
}

func generationTaskFromDB(row db.GenerationTask) GenerationTask {
	task := GenerationTask{
		ID:              uuidString(row.ID),
		ProductID:       uuidString(row.ProductID),
		CreatedByUserID: uuidString(row.CreatedByUserID),
		TaskType:        row.TaskType,
		Status:          row.Status,
		ErrorMessage:    row.ErrorMessage.String,
		RetryCount:      int(row.RetryCount),
		CreatedAt:       timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:       timeFromTimestamptz(row.UpdatedAt),
	}
	if len(row.ConfigSnapshot) > 0 {
		var payloadSummary map[string]any
		if err := json.Unmarshal(row.ConfigSnapshot, &payloadSummary); err == nil {
			task.PayloadSummary = payloadSummary
		}
	}
	if row.StartedAt.Valid {
		startedAt := row.StartedAt.Time
		task.StartedAt = &startedAt
	}
	if row.FinishedAt.Valid {
		finishedAt := row.FinishedAt.Time
		task.FinishedAt = &finishedAt
	}
	return task
}

func uuidParam(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func nullableUUIDParam(value string) pgtype.UUID {
	id, err := uuidParam(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textParam(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
