package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ScriptGenerationJobModeReplaceAll     = "replace_all"
	ScriptGenerationJobModeReplaceVariant = "replace_variant"

	ScriptGenerationJobStatusQueued     = "queued"
	ScriptGenerationJobStatusGenerating = "generating"
	ScriptGenerationJobStatusCompleted  = "completed"
	ScriptGenerationJobStatusFailed     = "failed"
	ScriptGenerationJobStatusCancelled  = "cancelled"
	ScriptGenerationJobStatusApplied    = "applied"
	ScriptGenerationJobStatusDiscarded  = "discarded"
)

var (
	ErrScriptGenerationJobNotFound = errors.New("script generation job not found")
	ErrScriptGenerationJobActive   = errors.New("script generation job is already active")
	ErrScriptGenerationJobState    = errors.New("script generation job state does not allow this operation")
)

type ScriptGenerationJob struct {
	ID              string                         `json:"id"`
	CreatedByUserID string                         `json:"created_by_user_id"`
	ProductID       string                         `json:"product_id"`
	Mode            string                         `json:"mode"`
	TargetVariantID string                         `json:"target_variant_id,omitempty"`
	BaseRevision    string                         `json:"base_revision"`
	Status          string                         `json:"status"`
	Input           WorkbenchScriptGenerationInput `json:"input"`
	ResultVariants  []GeneratedScriptVariant       `json:"result_variants,omitempty"`
	ErrorMessage    string                         `json:"error_message,omitempty"`
	CreatedAt       time.Time                      `json:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	StartedAt       *time.Time                     `json:"started_at,omitempty"`
	CompletedAt     *time.Time                     `json:"completed_at,omitempty"`
	ResolvedAt      *time.Time                     `json:"resolved_at,omitempty"`
}

type CreateScriptGenerationJobInput struct {
	CreatedByUserID string
	Mode            string
	TargetVariantID string
	BaseRevision    string
	GenerationInput WorkbenchScriptGenerationInput
}

type ScriptGenerationJobService struct {
	pool      *pgxpool.Pool
	generator *ScriptGenerationService

	mu         sync.RWMutex
	jobs       map[string]ScriptGenerationJob
	processing map[string]struct{}
}

func NewScriptGenerationJobService(pool *pgxpool.Pool, generator *ScriptGenerationService) *ScriptGenerationJobService {
	return &ScriptGenerationJobService{
		pool:       pool,
		generator:  generator,
		jobs:       map[string]ScriptGenerationJob{},
		processing: map[string]struct{}{},
	}
}

func (s *ScriptGenerationJobService) Create(ctx context.Context, input CreateScriptGenerationJobInput) (ScriptGenerationJob, error) {
	if s == nil || s.generator == nil || s.generator.productAssetService == nil {
		return ScriptGenerationJob{}, fmt.Errorf("script generation job service is not configured")
	}
	input.CreatedByUserID = strings.TrimSpace(input.CreatedByUserID)
	input.Mode = strings.TrimSpace(input.Mode)
	input.TargetVariantID = strings.TrimSpace(input.TargetVariantID)
	input.BaseRevision = strings.TrimSpace(input.BaseRevision)
	if input.CreatedByUserID == "" {
		return ScriptGenerationJob{}, fmt.Errorf("%w: user is required", ErrScriptGenerationInput)
	}
	if input.Mode != ScriptGenerationJobModeReplaceAll && input.Mode != ScriptGenerationJobModeReplaceVariant {
		return ScriptGenerationJob{}, fmt.Errorf("%w: invalid generation mode", ErrScriptGenerationInput)
	}
	if input.Mode == ScriptGenerationJobModeReplaceVariant {
		if _, err := uuid.Parse(input.TargetVariantID); err != nil {
			return ScriptGenerationJob{}, fmt.Errorf("%w: target variant is required", ErrScriptGenerationInput)
		}
	} else {
		input.TargetVariantID = ""
	}
	normalized, err := normalizeWorkbenchScriptGenerationInput(input.GenerationInput)
	if err != nil {
		return ScriptGenerationJob{}, err
	}
	product, err := s.generator.productAssetService.GetProduct(normalized.ProductID)
	if err != nil || product.Status == "archived" {
		return ScriptGenerationJob{}, ErrProductNotFound
	}
	if _, _, err := s.generator.resolveSellingPoints(normalized); err != nil {
		return ScriptGenerationJob{}, err
	}
	input.GenerationInput = normalized

	if s.pool == nil {
		return s.createMemory(input)
	}
	inputJSON, err := json.Marshal(input.GenerationInput)
	if err != nil {
		return ScriptGenerationJob{}, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO workbench_script_generation_jobs (
			created_by_user_id, product_id, mode, target_variant_id, base_revision, status, input_snapshot
		) VALUES (
			$1::uuid, $2::uuid, $3, NULLIF($4, '')::uuid, $5, 'queued', $6::jsonb
		)
		RETURNING `+scriptGenerationJobColumns,
		input.CreatedByUserID,
		input.GenerationInput.ProductID,
		input.Mode,
		input.TargetVariantID,
		input.BaseRevision,
		inputJSON,
	)
	job, err := scanScriptGenerationJob(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ScriptGenerationJob{}, ErrScriptGenerationJobActive
		}
		return ScriptGenerationJob{}, err
	}
	return job, nil
}

func (s *ScriptGenerationJobService) createMemory(input CreateScriptGenerationJobInput) (ScriptGenerationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.CreatedByUserID == input.CreatedByUserID && isActiveScriptGenerationJobStatus(job.Status) {
			return ScriptGenerationJob{}, ErrScriptGenerationJobActive
		}
	}
	now := time.Now()
	job := ScriptGenerationJob{
		ID:              uuid.NewString(),
		CreatedByUserID: input.CreatedByUserID,
		ProductID:       input.GenerationInput.ProductID,
		Mode:            input.Mode,
		TargetVariantID: input.TargetVariantID,
		BaseRevision:    input.BaseRevision,
		Status:          ScriptGenerationJobStatusQueued,
		Input:           cloneWorkbenchScriptGenerationInput(input.GenerationInput),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.jobs[job.ID] = job
	return cloneScriptGenerationJob(job), nil
}

func (s *ScriptGenerationJobService) GetForUser(ctx context.Context, jobID string, userID string) (ScriptGenerationJob, error) {
	job, err := s.get(ctx, jobID)
	if err != nil || job.CreatedByUserID != strings.TrimSpace(userID) {
		return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
	}
	return job, nil
}

func (s *ScriptGenerationJobService) GetLatestUnresolvedForUser(ctx context.Context, userID string) (ScriptGenerationJob, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		var latest ScriptGenerationJob
		found := false
		for _, job := range s.jobs {
			if job.CreatedByUserID != userID || !isUnresolvedScriptGenerationJobStatus(job.Status) {
				continue
			}
			if !found || job.CreatedAt.After(latest.CreatedAt) {
				latest, found = job, true
			}
		}
		if !found {
			return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
		}
		return cloneScriptGenerationJob(latest), nil
	}
	job, err := scanScriptGenerationJob(s.pool.QueryRow(ctx, `
		SELECT `+scriptGenerationJobColumns+`
		FROM workbench_script_generation_jobs
		WHERE created_by_user_id = $1::uuid
		  AND status IN ('queued', 'generating', 'completed', 'failed')
		ORDER BY created_at DESC
		LIMIT 1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
	}
	return job, err
}

func (s *ScriptGenerationJobService) Process(ctx context.Context, jobID string) error {
	if !s.beginProcessing(jobID) {
		return nil
	}
	defer s.endProcessing(jobID)

	job, err := s.get(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == ScriptGenerationJobStatusCompleted || job.Status == ScriptGenerationJobStatusApplied || job.Status == ScriptGenerationJobStatusDiscarded || job.Status == ScriptGenerationJobStatusCancelled {
		return nil
	}
	if job.Status != ScriptGenerationJobStatusQueued && job.Status != ScriptGenerationJobStatusGenerating {
		return ErrScriptGenerationJobState
	}
	if job.Status == ScriptGenerationJobStatusQueued {
		if err := s.markGenerating(ctx, job.ID); err != nil {
			return err
		}
	}
	variants, err := s.generator.GenerateWithProgress(ctx, job.Input, func(variant GeneratedScriptVariant) error {
		return s.upsertResultVariant(ctx, job.ID, variant)
	})
	if err != nil {
		if ctx.Err() != nil {
			_ = s.markQueued(context.Background(), job.ID)
			return err
		}
		_ = s.markFailed(context.Background(), job.ID, err.Error())
		return err
	}
	return s.markCompleted(ctx, job.ID, variants)
}

func (s *ScriptGenerationJobService) upsertResultVariant(ctx context.Context, jobID string, variant GeneratedScriptVariant) error {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ErrScriptGenerationJobNotFound
		}
		if job.Status != ScriptGenerationJobStatusGenerating {
			return ErrScriptGenerationJobState
		}
		replaced := false
		for index := range job.ResultVariants {
			if job.ResultVariants[index].Order == variant.Order {
				job.ResultVariants[index] = variant
				replaced = true
				break
			}
		}
		if !replaced {
			job.ResultVariants = append(job.ResultVariants, variant)
		}
		sort.Slice(job.ResultVariants, func(i, j int) bool {
			return job.ResultVariants[i].Order < job.ResultVariants[j].Order
		})
		job.UpdatedAt = time.Now()
		s.jobs[jobID] = job
		return nil
	}

	variantJSON, err := json.Marshal(variant)
	if err != nil {
		return err
	}
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE workbench_script_generation_jobs
		SET result_variants = (
			SELECT COALESCE(jsonb_agg(item ORDER BY (item->>'order')::int), '[]'::jsonb)
			FROM (
				SELECT value AS item
				FROM jsonb_array_elements(result_variants)
				WHERE COALESCE((value->>'order')::int, 0) <> $3
				UNION ALL
				SELECT $2::jsonb
			) AS variants
		), updated_at = now()
		WHERE id = $1::uuid AND status = 'generating'`, jobID, variantJSON, variant.Order)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrScriptGenerationJobState
	}
	return nil
}

func (s *ScriptGenerationJobService) PendingJobIDs(ctx context.Context) ([]string, error) {
	if s.pool == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		ids := make([]string, 0)
		for _, job := range s.jobs {
			if isActiveScriptGenerationJobStatus(job.Status) {
				ids = append(ids, job.ID)
			}
		}
		return ids, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text
		FROM workbench_script_generation_jobs
		WHERE status IN ('queued', 'generating')
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *ScriptGenerationJobService) beginProcessing(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.processing[jobID]; exists {
		return false
	}
	s.processing[jobID] = struct{}{}
	return true
}

func (s *ScriptGenerationJobService) endProcessing(jobID string) {
	s.mu.Lock()
	delete(s.processing, jobID)
	s.mu.Unlock()
}

func (s *ScriptGenerationJobService) CancelForUser(ctx context.Context, jobID string, userID string) (ScriptGenerationJob, error) {
	return s.transitionForUser(ctx, jobID, userID, []string{ScriptGenerationJobStatusQueued, ScriptGenerationJobStatusGenerating}, ScriptGenerationJobStatusCancelled)
}

func (s *ScriptGenerationJobService) MarkFailed(ctx context.Context, jobID string, cause error) error {
	if cause == nil {
		return nil
	}
	job, err := s.get(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == ScriptGenerationJobStatusQueued {
		if err := s.markGenerating(ctx, jobID); err != nil {
			return err
		}
	}
	return s.markFailed(ctx, jobID, cause.Error())
}

func (s *ScriptGenerationJobService) ResolveForUser(ctx context.Context, jobID string, userID string, resolution string) (ScriptGenerationJob, error) {
	resolution = strings.TrimSpace(resolution)
	if resolution == ScriptGenerationJobStatusApplied {
		return s.transitionForUser(ctx, jobID, userID, []string{ScriptGenerationJobStatusCompleted}, resolution)
	}
	if resolution == ScriptGenerationJobStatusDiscarded {
		return s.transitionForUser(ctx, jobID, userID, []string{ScriptGenerationJobStatusCompleted, ScriptGenerationJobStatusFailed, ScriptGenerationJobStatusCancelled}, resolution)
	}
	return ScriptGenerationJob{}, ErrScriptGenerationJobState
}

func (s *ScriptGenerationJobService) get(ctx context.Context, jobID string) (ScriptGenerationJob, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
		}
		return cloneScriptGenerationJob(job), nil
	}
	job, err := scanScriptGenerationJob(s.pool.QueryRow(ctx, `
		SELECT `+scriptGenerationJobColumns+`
		FROM workbench_script_generation_jobs
		WHERE id = $1::uuid`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
	}
	return job, err
}

func (s *ScriptGenerationJobService) markGenerating(ctx context.Context, jobID string) error {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ErrScriptGenerationJobNotFound
		}
		if job.Status != ScriptGenerationJobStatusQueued {
			return ErrScriptGenerationJobState
		}
		now := time.Now()
		job.Status = ScriptGenerationJobStatusGenerating
		job.StartedAt = &now
		job.UpdatedAt = now
		s.jobs[jobID] = job
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE workbench_script_generation_jobs
		SET status = 'generating', started_at = COALESCE(started_at, now()), updated_at = now(), error_message = NULL
		WHERE id = $1::uuid AND status = 'queued'`, jobID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrScriptGenerationJobState
	}
	return nil
}

func (s *ScriptGenerationJobService) markQueued(ctx context.Context, jobID string) error {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ErrScriptGenerationJobNotFound
		}
		if job.Status != ScriptGenerationJobStatusGenerating {
			return nil
		}
		job.Status = ScriptGenerationJobStatusQueued
		job.StartedAt = nil
		job.UpdatedAt = time.Now()
		s.jobs[jobID] = job
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE workbench_script_generation_jobs
		SET status = 'queued', started_at = NULL, updated_at = now()
		WHERE id = $1::uuid AND status = 'generating'`, jobID)
	return err
}

func (s *ScriptGenerationJobService) markCompleted(ctx context.Context, jobID string, variants []GeneratedScriptVariant) error {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ErrScriptGenerationJobNotFound
		}
		if job.Status == ScriptGenerationJobStatusCancelled {
			return nil
		}
		if job.Status != ScriptGenerationJobStatusGenerating {
			return ErrScriptGenerationJobState
		}
		now := time.Now()
		job.Status = ScriptGenerationJobStatusCompleted
		job.ResultVariants = append([]GeneratedScriptVariant(nil), variants...)
		job.CompletedAt = &now
		job.UpdatedAt = now
		s.jobs[jobID] = job
		return nil
	}
	resultJSON, err := json.Marshal(variants)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE workbench_script_generation_jobs
		SET status = 'completed', result_variants = $2::jsonb, error_message = NULL,
			completed_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status = 'generating'`, jobID, resultJSON)
	return err
}

func (s *ScriptGenerationJobService) markFailed(ctx context.Context, jobID string, message string) error {
	message = strings.TrimSpace(message)
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok {
			return ErrScriptGenerationJobNotFound
		}
		if job.Status == ScriptGenerationJobStatusCancelled {
			return nil
		}
		now := time.Now()
		job.Status = ScriptGenerationJobStatusFailed
		job.ErrorMessage = message
		job.CompletedAt = &now
		job.UpdatedAt = now
		s.jobs[jobID] = job
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE workbench_script_generation_jobs
		SET status = 'failed', error_message = $2, completed_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status = 'generating'`, jobID, message)
	return err
}

func (s *ScriptGenerationJobService) transitionForUser(ctx context.Context, jobID string, userID string, allowed []string, status string) (ScriptGenerationJob, error) {
	jobID = strings.TrimSpace(jobID)
	userID = strings.TrimSpace(userID)
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[jobID]
		if !ok || job.CreatedByUserID != userID {
			return ScriptGenerationJob{}, ErrScriptGenerationJobNotFound
		}
		if !containsString(allowed, job.Status) {
			return ScriptGenerationJob{}, ErrScriptGenerationJobState
		}
		now := time.Now()
		job.Status = status
		job.UpdatedAt = now
		if status == ScriptGenerationJobStatusCancelled {
			job.CompletedAt = &now
		} else {
			job.ResolvedAt = &now
		}
		s.jobs[jobID] = job
		return cloneScriptGenerationJob(job), nil
	}
	statusValues := make([]string, len(allowed))
	copy(statusValues, allowed)
	job, err := scanScriptGenerationJob(s.pool.QueryRow(ctx, `
		UPDATE workbench_script_generation_jobs
		SET status = $3,
			completed_at = CASE WHEN $3 = 'cancelled' THEN COALESCE(completed_at, now()) ELSE completed_at END,
			resolved_at = CASE WHEN $3 IN ('applied', 'discarded') THEN now() ELSE resolved_at END,
			updated_at = now()
		WHERE id = $1::uuid AND created_by_user_id = $2::uuid AND status = ANY($4::text[])
		RETURNING `+scriptGenerationJobColumns, jobID, userID, status, statusValues))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.GetForUser(ctx, jobID, userID); getErr != nil {
			return ScriptGenerationJob{}, getErr
		}
		return ScriptGenerationJob{}, ErrScriptGenerationJobState
	}
	return job, err
}

const scriptGenerationJobColumns = `
	id::text, created_by_user_id::text, product_id::text, mode,
	COALESCE(target_variant_id::text, ''), base_revision, status,
	input_snapshot, result_variants, COALESCE(error_message, ''),
	created_at, updated_at, started_at, completed_at, resolved_at`

type scriptGenerationJobScanner interface {
	Scan(...any) error
}

func scanScriptGenerationJob(scanner scriptGenerationJobScanner) (ScriptGenerationJob, error) {
	var job ScriptGenerationJob
	var inputJSON []byte
	var resultJSON []byte
	var startedAt pgtype.Timestamptz
	var completedAt pgtype.Timestamptz
	var resolvedAt pgtype.Timestamptz
	if err := scanner.Scan(
		&job.ID,
		&job.CreatedByUserID,
		&job.ProductID,
		&job.Mode,
		&job.TargetVariantID,
		&job.BaseRevision,
		&job.Status,
		&inputJSON,
		&resultJSON,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
		&startedAt,
		&completedAt,
		&resolvedAt,
	); err != nil {
		return ScriptGenerationJob{}, err
	}
	if err := json.Unmarshal(inputJSON, &job.Input); err != nil {
		return ScriptGenerationJob{}, err
	}
	if len(resultJSON) > 0 {
		if err := json.Unmarshal(resultJSON, &job.ResultVariants); err != nil {
			return ScriptGenerationJob{}, err
		}
	}
	job.StartedAt = timestamptzPointer(startedAt)
	job.CompletedAt = timestamptzPointer(completedAt)
	job.ResolvedAt = timestamptzPointer(resolvedAt)
	return job, nil
}

func timestamptzPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func isActiveScriptGenerationJobStatus(status string) bool {
	return status == ScriptGenerationJobStatusQueued || status == ScriptGenerationJobStatusGenerating
}

func isUnresolvedScriptGenerationJobStatus(status string) bool {
	return isActiveScriptGenerationJobStatus(status) || status == ScriptGenerationJobStatusCompleted || status == ScriptGenerationJobStatusFailed
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneWorkbenchScriptGenerationInput(input WorkbenchScriptGenerationInput) WorkbenchScriptGenerationInput {
	input.SellingPointIDs = append([]string(nil), input.SellingPointIDs...)
	input.CustomSellingPoints = append([]string(nil), input.CustomSellingPoints...)
	if input.Temperature != nil {
		value := *input.Temperature
		input.Temperature = &value
	}
	return input
}

func cloneScriptGenerationJob(job ScriptGenerationJob) ScriptGenerationJob {
	job.Input = cloneWorkbenchScriptGenerationInput(job.Input)
	job.ResultVariants = append([]GeneratedScriptVariant(nil), job.ResultVariants...)
	for index := range job.ResultVariants {
		job.ResultVariants[index].Beats = append([]GeneratedScriptBeat(nil), job.ResultVariants[index].Beats...)
	}
	if job.StartedAt != nil {
		value := *job.StartedAt
		job.StartedAt = &value
	}
	if job.CompletedAt != nil {
		value := *job.CompletedAt
		job.CompletedAt = &value
	}
	if job.ResolvedAt != nil {
		value := *job.ResolvedAt
		job.ResolvedAt = &value
	}
	return job
}
