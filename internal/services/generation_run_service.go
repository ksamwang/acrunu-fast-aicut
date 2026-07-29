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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	generationRunStatusGenerating = "generating"
	generationRunStatusCompleted  = "completed"
	generationRunStatusFailed     = "failed"

	generationRunStageQueued     = "queued"
	generationRunStageVoicing    = "voicing"
	generationRunStageAligning   = "aligning"
	generationRunStageRetrieving = "retrieving"
	generationRunStagePlanning   = "planning"
	generationRunStagePlanReady  = "plan_ready"
	generationRunStageRendering  = "rendering"
	generationRunStageCompleted  = "completed"
	generationRunStageFailed     = "failed"

	generationRunTaskStageVoiceover = "voiceover"
	generationRunTaskStageEditPlan  = "edit_plan"
	generationRunTaskStageRender    = "render"
)

var (
	ErrGenerationRunNotFound     = errors.New("generation run not found")
	ErrGenerationRunNotRetryable = errors.New("generation run is not retryable")
	ErrGenerationRunActive       = errors.New("generation run is active")
	ErrEditPlanNotFound          = errors.New("edit plan not found")
)

type GenerationRunRetryMode string

const (
	GenerationRunRetryEditPlan  GenerationRunRetryMode = "edit_plan"
	GenerationRunRetryVoiceover GenerationRunRetryMode = "voiceover"
	GenerationRunRetryRender    GenerationRunRetryMode = "render"
)

type GenerationRun struct {
	ID                  string         `json:"id"`
	ProductID           string         `json:"product_id"`
	CreatedByUserID     string         `json:"created_by_user_id,omitempty"`
	CreatedByName       string         `json:"created_by_name,omitempty"`
	VoiceoverTaskID     string         `json:"voiceover_task_id,omitempty"`
	ScriptVariantID     string         `json:"script_variant_id,omitempty"`
	VoiceoverID         string         `json:"voiceover_id,omitempty"`
	Status              string         `json:"status"`
	Stage               string         `json:"stage"`
	Progress            int            `json:"progress"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	ConfigSnapshot      map[string]any `json:"config_snapshot,omitempty"`
	OutputStorageKey    string         `json:"-"`
	OutputMimeType      string         `json:"output_mime_type,omitempty"`
	OutputDurationMs    int            `json:"output_duration_ms,omitempty"`
	OutputWidth         int            `json:"output_width,omitempty"`
	OutputHeight        int            `json:"output_height,omitempty"`
	OutputFileSizeBytes int64          `json:"output_file_size_bytes,omitempty"`
	Renderer            string         `json:"renderer,omitempty"`
	RenderVersion       string         `json:"render_version,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	CompletedAt         *time.Time     `json:"completed_at,omitempty"`
}

type GenerationRenderOutput struct {
	StorageKey    string
	MimeType      string
	DurationMs    int
	Width         int
	Height        int
	FileSizeBytes int64
	Renderer      string
	RenderVersion string
}

type GenerationRunTask struct {
	GenerationRunID  string    `json:"generation_run_id"`
	GenerationTaskID string    `json:"generation_task_id"`
	Stage            string    `json:"stage"`
	CreatedAt        time.Time `json:"created_at"`
}

type EditPlanClip struct {
	ID                 string  `json:"id"`
	VisualBeatID       string  `json:"visual_beat_id"`
	NarrationSegmentID string  `json:"narration_segment_id"`
	AssetID            string  `json:"asset_id"`
	SpeechSegmentID    string  `json:"speech_segment_id,omitempty"`
	SourceInMs         int     `json:"source_in_ms"`
	SourceOutMs        int     `json:"source_out_ms"`
	StartMs            int     `json:"start_ms"`
	EndMs              int     `json:"end_ms"`
	TimelineDurationMs int     `json:"timeline_duration_ms"`
	Label              string  `json:"label"`
	VisualGoal         string  `json:"visual_goal"`
	SourceType         string  `json:"source_type"`
	UseOriginalAudio   bool    `json:"use_original_audio"`
	AudioGainDB        float64 `json:"audio_gain_db"`
}

type EditPlan struct {
	ID                 string             `json:"id"`
	GenerationRunID    string             `json:"generation_run_id"`
	ScriptVariantID    string             `json:"script_variant_id"`
	VoiceoverID        string             `json:"voiceover_id"`
	Status             string             `json:"status"`
	CandidateSnapshot  json.RawMessage    `json:"candidate_snapshot,omitempty"`
	PlanJSON           json.RawMessage    `json:"plan_json,omitempty"`
	LLMProvider        string             `json:"llm_provider,omitempty"`
	LLMModel           string             `json:"llm_model,omitempty"`
	PromptVersion      string             `json:"prompt_version,omitempty"`
	ErrorMessage       string             `json:"error_message,omitempty"`
	SourceDurationMs   int                `json:"source_duration_ms,omitempty"`
	TimelineDurationMs int                `json:"timeline_duration_ms,omitempty"`
	NarrationSegments  []NarrationSegment `json:"narration_segments,omitempty"`
	NarrationPauses    []NarrationPause   `json:"narration_pauses,omitempty"`
	VisualBeats        []VisualBeat       `json:"visual_beats"`
	Clips              []EditPlanClip     `json:"clips"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

type NarrationPause struct {
	AfterMs    int `json:"after_ms"`
	DurationMs int `json:"duration_ms"`
}

type CreateGenerationRunInput struct {
	ProductID       string
	CreatedByUserID string
	CreatedByName   string
	ConfigSnapshot  map[string]any
}

type generationWorkLoader interface {
	GetVoiceoverWork(context.Context, string) (VoiceoverWork, error)
}

type GenerationRunService struct {
	pool       *pgxpool.Pool
	voiceovers generationWorkLoader

	mu          sync.RWMutex
	memoryRuns  map[string]GenerationRun
	memoryTasks map[string]GenerationRunTask
	memoryPlans map[string]EditPlan
}

func NewGenerationRunService(voiceovers generationWorkLoader) *GenerationRunService {
	return newGenerationRunService(nil, voiceovers)
}

func NewGenerationRunServiceWithPool(pool *pgxpool.Pool, voiceovers generationWorkLoader) *GenerationRunService {
	if pool == nil {
		return NewGenerationRunService(voiceovers)
	}
	return newGenerationRunService(pool, voiceovers)
}

func newGenerationRunService(pool *pgxpool.Pool, voiceovers generationWorkLoader) *GenerationRunService {
	return &GenerationRunService{
		pool:        pool,
		voiceovers:  voiceovers,
		memoryRuns:  map[string]GenerationRun{},
		memoryTasks: map[string]GenerationRunTask{},
		memoryPlans: map[string]EditPlan{},
	}
}

func (s *GenerationRunService) Create(ctx context.Context, input CreateGenerationRunInput) (GenerationRun, error) {
	input.ProductID = normalizeID(input.ProductID)
	if input.ProductID == "" {
		return GenerationRun{}, fmt.Errorf("product id is required")
	}
	input.CreatedByUserID = normalizeID(input.CreatedByUserID)
	input.CreatedByName = strings.TrimSpace(input.CreatedByName)
	if s.pool == nil {
		now := time.Now()
		run := GenerationRun{
			ID:              uuid.NewString(),
			ProductID:       input.ProductID,
			CreatedByUserID: input.CreatedByUserID,
			CreatedByName:   input.CreatedByName,
			Status:          generationRunStatusGenerating,
			Stage:           generationRunStageQueued,
			Progress:        4,
			ConfigSnapshot:  cloneRunObject(input.ConfigSnapshot),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		s.mu.Lock()
		s.memoryRuns[run.ID] = run
		s.mu.Unlock()
		return cloneGenerationRun(run), nil
	}

	snapshot, err := generationRunSnapshotJSON(input.ConfigSnapshot)
	if err != nil {
		return GenerationRun{}, err
	}
	var runID string
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO generation_runs (product_id, created_by_user_id, created_by_name_snapshot, status, stage, progress, config_snapshot)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, 'generating', 'queued', 4, $4::jsonb)
		RETURNING id::text`, input.ProductID, input.CreatedByUserID, input.CreatedByName, snapshot).Scan(&runID); err != nil {
		return GenerationRun{}, err
	}
	return s.Get(ctx, runID)
}

func (s *GenerationRunService) Get(ctx context.Context, runID string) (GenerationRun, error) {
	runID = normalizeID(runID)
	if runID == "" {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		run, ok := s.memoryRuns[runID]
		s.mu.RUnlock()
		if !ok {
			return GenerationRun{}, ErrGenerationRunNotFound
		}
		return cloneGenerationRun(run), nil
	}
	run, err := scanGenerationRun(s.pool.QueryRow(ctx, generationRunColumns+` FROM generation_runs WHERE id = $1::uuid`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	return run, err
}

func (s *GenerationRunService) Delete(ctx context.Context, runID string) (GenerationRun, error) {
	runID = normalizeID(runID)
	if runID == "" {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			return GenerationRun{}, ErrGenerationRunNotFound
		}
		if run.Status == generationRunStatusGenerating {
			return GenerationRun{}, ErrGenerationRunActive
		}
		delete(s.memoryRuns, runID)
		delete(s.memoryPlans, runID)
		for taskID, link := range s.memoryTasks {
			if link.GenerationRunID == runID {
				delete(s.memoryTasks, taskID)
			}
		}
		return cloneGenerationRun(run), nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GenerationRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	run, err := scanGenerationRun(tx.QueryRow(ctx, generationRunColumns+` FROM generation_runs WHERE id = $1::uuid FOR UPDATE`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	if err != nil {
		return GenerationRun{}, err
	}
	if run.Status == generationRunStatusGenerating {
		return GenerationRun{}, ErrGenerationRunActive
	}
	if _, err := tx.Exec(ctx, `DELETE FROM generation_runs WHERE id = $1::uuid`, runID); err != nil {
		return GenerationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GenerationRun{}, err
	}
	return run, nil
}

func (s *GenerationRunService) List(ctx context.Context) ([]GenerationRun, error) {
	if s.pool == nil {
		s.mu.RLock()
		items := make([]GenerationRun, 0, len(s.memoryRuns))
		for _, run := range s.memoryRuns {
			items = append(items, cloneGenerationRun(run))
		}
		s.mu.RUnlock()
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		return items, nil
	}
	rows, err := s.pool.Query(ctx, generationRunColumns+` FROM generation_runs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GenerationRun{}
	for rows.Next() {
		run, err := scanGenerationRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func (s *GenerationRunService) ListByStage(ctx context.Context, stage string) ([]GenerationRun, error) {
	if err := validateGenerationRunStage(stage); err != nil {
		return nil, err
	}
	if s.pool == nil {
		items, err := s.List(ctx)
		if err != nil {
			return nil, err
		}
		filtered := make([]GenerationRun, 0, len(items))
		for _, item := range items {
			if item.Stage == stage {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}

	rows, err := s.pool.Query(ctx, generationRunColumns+` FROM generation_runs WHERE stage = $1 ORDER BY created_at ASC`, stage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GenerationRun{}
	for rows.Next() {
		run, err := scanGenerationRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func (s *GenerationRunService) LinkTask(ctx context.Context, runID string, taskID string, stage string) error {
	if err := validateGenerationRunTaskStage(stage); err != nil {
		return err
	}
	runID = normalizeID(runID)
	taskID = normalizeID(taskID)
	if runID == "" || taskID == "" {
		return fmt.Errorf("run id and task id are required")
	}
	if s.pool == nil {
		if _, err := s.Get(ctx, runID); err != nil {
			return err
		}
		entry := GenerationRunTask{GenerationRunID: runID, GenerationTaskID: taskID, Stage: stage, CreatedAt: time.Now()}
		s.mu.Lock()
		s.memoryTasks[taskID] = entry
		s.mu.Unlock()
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO generation_run_tasks (generation_run_id, generation_task_id, stage)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (generation_task_id) DO UPDATE SET
			generation_run_id = EXCLUDED.generation_run_id,
			stage = EXCLUDED.stage`, runID, taskID, stage)
	return err
}

func (s *GenerationRunService) FindByTask(ctx context.Context, taskID string) (GenerationRun, error) {
	taskID = normalizeID(taskID)
	if taskID == "" {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		entry, ok := s.memoryTasks[taskID]
		run, runOK := s.memoryRuns[entry.GenerationRunID]
		s.mu.RUnlock()
		if !ok || !runOK {
			return GenerationRun{}, ErrGenerationRunNotFound
		}
		return cloneGenerationRun(run), nil
	}
	run, err := scanGenerationRun(s.pool.QueryRow(ctx, generationRunColumns+`
		FROM generation_runs runs
		JOIN generation_run_tasks links ON links.generation_run_id = runs.id
		WHERE links.generation_task_id = $1::uuid`, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	return run, err
}

func (s *GenerationRunService) FindTaskByStage(ctx context.Context, runID string, stage string) (GenerationRunTask, bool, error) {
	if err := validateGenerationRunTaskStage(stage); err != nil {
		return GenerationRunTask{}, false, err
	}
	runID = normalizeID(runID)
	if runID == "" {
		return GenerationRunTask{}, false, ErrGenerationRunNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, entry := range s.memoryTasks {
			if entry.GenerationRunID == runID && entry.Stage == stage {
				return entry, true, nil
			}
		}
		return GenerationRunTask{}, false, nil
	}
	var entry GenerationRunTask
	err := s.pool.QueryRow(ctx, `
		SELECT generation_run_id::text, generation_task_id::text, stage, created_at
		FROM generation_run_tasks
		WHERE generation_run_id = $1::uuid AND stage = $2
		ORDER BY created_at ASC
		LIMIT 1`, runID, stage).Scan(&entry.GenerationRunID, &entry.GenerationTaskID, &entry.Stage, &entry.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return GenerationRunTask{}, false, nil
	}
	return entry, err == nil, err
}

func (s *GenerationRunService) AttachVoiceoverArtifacts(ctx context.Context, runID string, taskID string, scriptVariantID string, voiceoverID string) error {
	runID = normalizeID(runID)
	taskID = normalizeID(taskID)
	scriptVariantID = normalizeID(scriptVariantID)
	voiceoverID = normalizeID(voiceoverID)
	if runID == "" || taskID == "" || scriptVariantID == "" || voiceoverID == "" {
		return fmt.Errorf("generation run voiceover artifacts are required")
	}
	if s.pool == nil {
		s.mu.Lock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			s.mu.Unlock()
			return ErrGenerationRunNotFound
		}
		run.VoiceoverTaskID = taskID
		run.ScriptVariantID = scriptVariantID
		run.VoiceoverID = voiceoverID
		run.Stage = generationRunStageVoicing
		run.Progress = 8
		run.UpdatedAt = time.Now()
		s.memoryRuns[runID] = run
		s.mu.Unlock()
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE generation_runs
		SET voiceover_task_id = $2::uuid,
			script_variant_id = $3::uuid,
			voiceover_id = $4::uuid,
			stage = 'voicing',
			progress = 8,
			error_message = NULL,
			updated_at = now()
		WHERE id = $1::uuid`, runID, taskID, scriptVariantID, voiceoverID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrGenerationRunNotFound
	}
	return nil
}

func (s *GenerationRunService) UpdateStage(ctx context.Context, runID string, stage string, progress int) error {
	if err := validateGenerationRunStage(stage); err != nil {
		return err
	}
	if progress < 0 || progress > 100 {
		return fmt.Errorf("generation run progress must be between 0 and 100")
	}
	runID = normalizeID(runID)
	if runID == "" {
		return ErrGenerationRunNotFound
	}
	status := generationRunStatusGenerating
	if stage == generationRunStageCompleted {
		status = generationRunStatusCompleted
		progress = 100
	}
	if stage == generationRunStageFailed {
		status = generationRunStatusFailed
		progress = 100
	}
	if s.pool == nil {
		s.mu.Lock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			s.mu.Unlock()
			return ErrGenerationRunNotFound
		}
		run.Status = status
		run.Stage = stage
		run.Progress = progress
		run.ErrorMessage = ""
		run.UpdatedAt = time.Now()
		if status == generationRunStatusCompleted {
			completedAt := run.UpdatedAt
			run.CompletedAt = &completedAt
		}
		s.memoryRuns[runID] = run
		s.mu.Unlock()
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE generation_runs
		SET status = $2,
			stage = $3,
			progress = $4,
			error_message = NULL,
			completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $1::uuid`, runID, status, stage, progress)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrGenerationRunNotFound
	}
	return nil
}

func (s *GenerationRunService) MarkFailed(ctx context.Context, runID string, cause error) error {
	if cause == nil {
		return nil
	}
	runID = normalizeID(runID)
	if runID == "" {
		return ErrGenerationRunNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			s.mu.Unlock()
			return ErrGenerationRunNotFound
		}
		run.Status = generationRunStatusFailed
		run.Stage = generationRunStageFailed
		run.Progress = 100
		run.ErrorMessage = cause.Error()
		run.UpdatedAt = time.Now()
		s.memoryRuns[runID] = run
		s.mu.Unlock()
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE generation_runs
		SET status = 'failed', stage = 'failed', progress = 100,
			error_message = $2, updated_at = now()
		WHERE id = $1::uuid`, runID, cause.Error())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrGenerationRunNotFound
	}
	return nil
}

func (s *GenerationRunService) MarkRenderCompleted(ctx context.Context, runID string, output GenerationRenderOutput) error {
	runID = normalizeID(runID)
	output.StorageKey = strings.TrimSpace(output.StorageKey)
	output.MimeType = strings.TrimSpace(output.MimeType)
	output.Renderer = strings.TrimSpace(output.Renderer)
	output.RenderVersion = strings.TrimSpace(output.RenderVersion)
	if runID == "" {
		return ErrGenerationRunNotFound
	}
	if output.StorageKey == "" || output.MimeType == "" || output.DurationMs <= 0 || output.Width <= 0 || output.Height <= 0 || output.FileSizeBytes <= 0 || output.Renderer == "" || output.RenderVersion == "" {
		return fmt.Errorf("generation render output is incomplete")
	}
	if s.pool == nil {
		s.mu.Lock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			s.mu.Unlock()
			return ErrGenerationRunNotFound
		}
		now := time.Now()
		run.Status = generationRunStatusCompleted
		run.Stage = generationRunStageCompleted
		run.Progress = 100
		run.ErrorMessage = ""
		run.OutputStorageKey = output.StorageKey
		run.OutputMimeType = output.MimeType
		run.OutputDurationMs = output.DurationMs
		run.OutputWidth = output.Width
		run.OutputHeight = output.Height
		run.OutputFileSizeBytes = output.FileSizeBytes
		run.Renderer = output.Renderer
		run.RenderVersion = output.RenderVersion
		run.CompletedAt = &now
		run.UpdatedAt = now
		s.memoryRuns[runID] = run
		s.mu.Unlock()
		return nil
	}

	command, err := s.pool.Exec(ctx, `
		UPDATE generation_runs
		SET status = 'completed', stage = 'completed', progress = 100,
			error_message = NULL,
			output_storage_key = $2,
			output_mime_type = $3,
			output_duration_ms = $4,
			output_width = $5,
			output_height = $6,
			output_file_size_bytes = $7,
			renderer = $8,
			render_version = $9,
			completed_at = now(), updated_at = now()
		WHERE id = $1::uuid`,
		runID,
		output.StorageKey,
		output.MimeType,
		output.DurationMs,
		output.Width,
		output.Height,
		output.FileSizeBytes,
		output.Renderer,
		output.RenderVersion,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrGenerationRunNotFound
	}
	return nil
}

// PrepareRetry keeps the existing run as the user-visible finished-work item,
// while discarding stale work links and the previous partial edit plan.
func (s *GenerationRunService) PrepareRetry(ctx context.Context, runID string, mode GenerationRunRetryMode) (GenerationRun, error) {
	if err := validateGenerationRunRetryMode(mode); err != nil {
		return GenerationRun{}, err
	}
	runID = normalizeID(runID)
	if runID == "" {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	stage, progress := retryStage(mode)
	if s.pool == nil {
		s.mu.Lock()
		run, ok := s.memoryRuns[runID]
		if !ok {
			s.mu.Unlock()
			return GenerationRun{}, ErrGenerationRunNotFound
		}
		if !generationRunCanRetry(run.Status, mode) {
			s.mu.Unlock()
			return GenerationRun{}, ErrGenerationRunNotRetryable
		}
		for taskID, link := range s.memoryTasks {
			if link.GenerationRunID != runID {
				continue
			}
			if mode == GenerationRunRetryVoiceover ||
				(mode == GenerationRunRetryEditPlan && (link.Stage == generationRunTaskStageEditPlan || link.Stage == generationRunTaskStageRender)) ||
				(mode == GenerationRunRetryRender && link.Stage == generationRunTaskStageRender) {
				delete(s.memoryTasks, taskID)
			}
		}
		if mode != GenerationRunRetryRender {
			delete(s.memoryPlans, runID)
		}
		run.Status = generationRunStatusGenerating
		run.Stage = stage
		run.Progress = progress
		run.ErrorMessage = ""
		run.CompletedAt = nil
		run.OutputStorageKey = ""
		run.OutputMimeType = ""
		run.OutputDurationMs = 0
		run.OutputWidth = 0
		run.OutputHeight = 0
		run.OutputFileSizeBytes = 0
		run.Renderer = ""
		run.RenderVersion = ""
		run.UpdatedAt = time.Now()
		s.memoryRuns[runID] = run
		s.mu.Unlock()
		return cloneGenerationRun(run), nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GenerationRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	run, err := scanGenerationRun(tx.QueryRow(ctx, generationRunColumns+` FROM generation_runs WHERE id = $1::uuid FOR UPDATE`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return GenerationRun{}, ErrGenerationRunNotFound
	}
	if err != nil {
		return GenerationRun{}, err
	}
	if !generationRunCanRetry(run.Status, mode) {
		return GenerationRun{}, ErrGenerationRunNotRetryable
	}
	if mode == GenerationRunRetryVoiceover {
		_, err = tx.Exec(ctx, `DELETE FROM generation_run_tasks WHERE generation_run_id = $1::uuid`, runID)
	} else if mode == GenerationRunRetryEditPlan {
		_, err = tx.Exec(ctx, `
			DELETE FROM generation_run_tasks
			WHERE generation_run_id = $1::uuid AND stage IN ($2, $3)`, runID, generationRunTaskStageEditPlan, generationRunTaskStageRender)
	} else {
		_, err = tx.Exec(ctx, `
			DELETE FROM generation_run_tasks
			WHERE generation_run_id = $1::uuid AND stage = $2`, runID, generationRunTaskStageRender)
	}
	if err != nil {
		return GenerationRun{}, err
	}
	if mode != GenerationRunRetryRender {
		if _, err := tx.Exec(ctx, `DELETE FROM edit_plans WHERE generation_run_id = $1::uuid`, runID); err != nil {
			return GenerationRun{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE generation_runs
		SET status = 'generating', stage = $2, progress = $3,
			error_message = NULL, completed_at = NULL,
			output_storage_key = NULL, output_mime_type = NULL,
			output_duration_ms = NULL, output_width = NULL, output_height = NULL,
			output_file_size_bytes = NULL, renderer = NULL, render_version = NULL,
			updated_at = now()
		WHERE id = $1::uuid`, runID, stage, progress); err != nil {
		return GenerationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GenerationRun{}, err
	}
	return s.Get(ctx, runID)
}

func generationRunCanRetry(status string, mode GenerationRunRetryMode) bool {
	return status == generationRunStatusFailed || (status == generationRunStatusCompleted && mode == GenerationRunRetryVoiceover)
}

func (s *GenerationRunService) SaveEditPlan(ctx context.Context, plan EditPlan) (EditPlan, error) {
	for index := range plan.VisualBeats {
		plan.VisualBeats[index].DurationClass = normalizeVisualBeatDurationClass(plan.VisualBeats[index].DurationClass)
	}
	if err := validateEditPlanForStorage(plan); err != nil {
		return EditPlan{}, err
	}
	if len(plan.CandidateSnapshot) == 0 {
		plan.CandidateSnapshot = mustRunJSON(map[string]any{
			"source_duration_ms":   plan.SourceDurationMs,
			"timeline_duration_ms": plan.TimelineDurationMs,
			"narration_segments":   plan.NarrationSegments,
			"narration_pauses":     plan.NarrationPauses,
			"visual_beats":         plan.VisualBeats,
			"candidate_sets":       []CandidateSet{},
			"clips":                plan.Clips,
		})
	}
	if len(plan.PlanJSON) == 0 {
		plan.PlanJSON = mustRunJSON(map[string]any{
			"source_duration_ms":   plan.SourceDurationMs,
			"timeline_duration_ms": plan.TimelineDurationMs,
			"narration_segments":   plan.NarrationSegments,
			"narration_pauses":     plan.NarrationPauses,
			"visual_beats":         plan.VisualBeats,
			"candidate_sets":       []CandidateSet{},
			"clips":                plan.Clips,
		})
	}
	if !json.Valid(plan.CandidateSnapshot) || !json.Valid(plan.PlanJSON) {
		return EditPlan{}, fmt.Errorf("edit plan JSON is invalid")
	}
	if s.pool == nil {
		now := time.Now()
		s.mu.Lock()
		if _, ok := s.memoryRuns[plan.GenerationRunID]; !ok {
			s.mu.Unlock()
			return EditPlan{}, ErrGenerationRunNotFound
		}
		if existing, ok := s.memoryPlans[plan.GenerationRunID]; ok {
			plan.ID = existing.ID
			plan.CreatedAt = existing.CreatedAt
		} else {
			plan.ID = uuid.NewString()
			plan.CreatedAt = now
		}
		plan.UpdatedAt = now
		plan.VisualBeats = cloneVisualBeats(plan.VisualBeats)
		for index := range plan.VisualBeats {
			if plan.VisualBeats[index].ID == "" {
				plan.VisualBeats[index].ID = uuid.NewString()
			}
		}
		plan.Clips = cloneEditPlanClips(plan.Clips)
		for index := range plan.Clips {
			if plan.Clips[index].ID == "" {
				plan.Clips[index].ID = uuid.NewString()
			}
		}
		s.memoryPlans[plan.GenerationRunID] = cloneEditPlan(plan)
		s.mu.Unlock()
		return cloneEditPlan(plan), nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return EditPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		INSERT INTO edit_plans (
			generation_run_id, script_variant_id, voiceover_id, status,
			candidate_snapshot, plan_json, llm_provider, llm_model, prompt_version, error_message
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::jsonb, $6::jsonb, $7, $8, $9, NULLIF($10, ''))
		ON CONFLICT (generation_run_id) DO UPDATE SET
			script_variant_id = EXCLUDED.script_variant_id,
			voiceover_id = EXCLUDED.voiceover_id,
			status = EXCLUDED.status,
			candidate_snapshot = EXCLUDED.candidate_snapshot,
			plan_json = EXCLUDED.plan_json,
			llm_provider = EXCLUDED.llm_provider,
			llm_model = EXCLUDED.llm_model,
			prompt_version = EXCLUDED.prompt_version,
			error_message = EXCLUDED.error_message,
			updated_at = now()
		RETURNING id::text, generation_run_id::text, script_variant_id::text, voiceover_id::text,
			status, candidate_snapshot, plan_json, llm_provider, llm_model, prompt_version,
			COALESCE(error_message, ''), created_at, updated_at`,
		plan.GenerationRunID,
		plan.ScriptVariantID,
		plan.VoiceoverID,
		plan.Status,
		[]byte(plan.CandidateSnapshot),
		[]byte(plan.PlanJSON),
		plan.LLMProvider,
		plan.LLMModel,
		plan.PromptVersion,
		plan.ErrorMessage,
	)
	stored, err := scanEditPlan(row)
	if err != nil {
		return EditPlan{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM clip_segments WHERE edit_plan_id = $1::uuid`, stored.ID); err != nil {
		return EditPlan{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM visual_beats WHERE edit_plan_id = $1::uuid`, stored.ID); err != nil {
		return EditPlan{}, err
	}
	for index, beat := range plan.VisualBeats {
		if beat.ID == "" {
			beat.ID = uuid.NewString()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO visual_beats (
				id, edit_plan_id, beat_index, narration_segment_id, narrative_beat_id,
				start_ms, end_ms, duration_class, label, selling_point, visual_goal, source_type
			) VALUES (
				$1::uuid, $2::uuid, $3, $4::uuid, $5,
				$6, $7, $8, $9, $10, $11, $12
			)`,
			beat.ID,
			stored.ID,
			index,
			beat.NarrationSegmentID,
			beat.NarrativeBeatID,
			beat.StartMs,
			beat.EndMs,
			beat.DurationClass,
			beat.Label,
			beat.SellingPoint,
			beat.VisualGoal,
			beat.SourceType,
		); err != nil {
			return EditPlan{}, err
		}
		plan.VisualBeats[index] = beat
	}
	for index, clip := range plan.Clips {
		if clip.ID == "" {
			clip.ID = uuid.NewString()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO clip_segments (
				id, edit_plan_id, segment_index, visual_beat_id, narration_segment_id, asset_id, speech_segment_id,
				source_in_ms, source_out_ms, timeline_in_ms, timeline_duration_ms,
				source_type, label, visual_goal, use_original_audio, audio_gain_db
			) VALUES (
				$1::uuid, $2::uuid, $3, $4::uuid, $5::uuid, $6::uuid, NULLIF($7, '')::uuid,
				$8, $9, $10, $11, $12, $13, $14, $15, $16
			)`,
			clip.ID,
			stored.ID,
			index,
			clip.VisualBeatID,
			clip.NarrationSegmentID,
			clip.AssetID,
			clip.SpeechSegmentID,
			clip.SourceInMs,
			clip.SourceOutMs,
			clip.StartMs,
			clip.TimelineDurationMs,
			clip.SourceType,
			clip.Label,
			clip.VisualGoal,
			clip.UseOriginalAudio,
			clip.AudioGainDB,
		); err != nil {
			return EditPlan{}, err
		}
		plan.Clips[index] = clip
	}
	if err := tx.Commit(ctx); err != nil {
		return EditPlan{}, err
	}
	stored.VisualBeats = cloneVisualBeats(plan.VisualBeats)
	stored.Clips = cloneEditPlanClips(plan.Clips)
	stored.SourceDurationMs = plan.SourceDurationMs
	stored.TimelineDurationMs = plan.TimelineDurationMs
	stored.NarrationSegments = cloneNarrationSegments(plan.NarrationSegments)
	stored.NarrationPauses = cloneNarrationPauses(plan.NarrationPauses)
	return stored, nil
}

func (s *GenerationRunService) GetEditPlan(ctx context.Context, runID string) (EditPlan, error) {
	runID = normalizeID(runID)
	if runID == "" {
		return EditPlan{}, ErrEditPlanNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		plan, ok := s.memoryPlans[runID]
		s.mu.RUnlock()
		if !ok {
			return EditPlan{}, ErrEditPlanNotFound
		}
		return cloneEditPlan(plan), nil
	}
	plan, err := scanEditPlan(s.pool.QueryRow(ctx, `
		SELECT id::text, generation_run_id::text, script_variant_id::text, voiceover_id::text,
			status, candidate_snapshot, plan_json, llm_provider, llm_model, prompt_version,
			COALESCE(error_message, ''), created_at, updated_at
		FROM edit_plans
		WHERE generation_run_id = $1::uuid`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return EditPlan{}, ErrEditPlanNotFound
	}
	if err != nil {
		return EditPlan{}, err
	}
	decodeEditPlanTimeline(&plan)
	visualBeatRows, err := s.pool.Query(ctx, `
		SELECT id::text, narration_segment_id::text, narrative_beat_id, start_ms, end_ms, duration_class,
			label, selling_point, visual_goal, source_type
		FROM visual_beats
		WHERE edit_plan_id = $1::uuid
		ORDER BY beat_index ASC`, plan.ID)
	if err != nil {
		return EditPlan{}, err
	}
	defer visualBeatRows.Close()
	for visualBeatRows.Next() {
		var beat VisualBeat
		if err := visualBeatRows.Scan(
			&beat.ID,
			&beat.NarrationSegmentID,
			&beat.NarrativeBeatID,
			&beat.StartMs,
			&beat.EndMs,
			&beat.DurationClass,
			&beat.Label,
			&beat.SellingPoint,
			&beat.VisualGoal,
			&beat.SourceType,
		); err != nil {
			return EditPlan{}, err
		}
		plan.VisualBeats = append(plan.VisualBeats, beat)
	}
	if err := visualBeatRows.Err(); err != nil {
		return EditPlan{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, visual_beat_id::text, narration_segment_id::text, asset_id::text, COALESCE(speech_segment_id::text, ''),
			source_in_ms, source_out_ms, timeline_in_ms, timeline_duration_ms,
			label, visual_goal, source_type, use_original_audio, audio_gain_db
		FROM clip_segments
		WHERE edit_plan_id = $1::uuid
		ORDER BY segment_index ASC`, plan.ID)
	if err != nil {
		return EditPlan{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var clip EditPlanClip
		var audioGainDB pgtype.Numeric
		if err := rows.Scan(
			&clip.ID,
			&clip.VisualBeatID,
			&clip.NarrationSegmentID,
			&clip.AssetID,
			&clip.SpeechSegmentID,
			&clip.SourceInMs,
			&clip.SourceOutMs,
			&clip.StartMs,
			&clip.TimelineDurationMs,
			&clip.Label,
			&clip.VisualGoal,
			&clip.SourceType,
			&clip.UseOriginalAudio,
			&audioGainDB,
		); err != nil {
			return EditPlan{}, err
		}
		clip.AudioGainDB = numericValue(audioGainDB)
		clip.EndMs = clip.StartMs + clip.TimelineDurationMs
		plan.Clips = append(plan.Clips, clip)
	}
	if err := rows.Err(); err != nil {
		return EditPlan{}, err
	}
	return plan, nil
}

func (s *GenerationRunService) ListWorks(ctx context.Context) ([]VoiceoverWork, error) {
	runs, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	works := make([]VoiceoverWork, 0, len(runs))
	for _, run := range runs {
		work, err := s.workFromRun(ctx, run)
		if errors.Is(err, ErrVoiceoverWorkNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	return works, nil
}

func (s *GenerationRunService) GetWork(ctx context.Context, runID string) (VoiceoverWork, error) {
	run, err := s.Get(ctx, runID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	return s.workFromRun(ctx, run)
}

func (s *GenerationRunService) workFromRun(ctx context.Context, run GenerationRun) (VoiceoverWork, error) {
	if s.voiceovers == nil || run.VoiceoverTaskID == "" {
		return VoiceoverWork{}, ErrVoiceoverWorkNotFound
	}
	work, err := s.voiceovers.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	work.ID = run.ID
	work.RunID = run.ID
	work.Status = run.Status
	work.StageLabel = generationRunStageLabel(run.Stage)
	work.Progress = run.Progress
	work.ErrorMessage = run.ErrorMessage
	work.CreatedByUserID = run.CreatedByUserID
	work.CreatedByName = run.CreatedByName
	if run.OutputStorageKey != "" {
		work.VideoURL = publicStorageURL(run.OutputStorageKey)
	}
	work.OutputMimeType = run.OutputMimeType
	work.OutputWidth = run.OutputWidth
	work.OutputHeight = run.OutputHeight
	work.OutputFileSizeBytes = run.OutputFileSizeBytes
	if bgm := renderSnapshotBGM(run.ConfigSnapshot); bgm != nil {
		work.BGM = &VoiceoverWorkBGM{TrackID: bgm.TrackID, Name: bgm.Name, GainDB: bgm.GainDB}
	} else {
		work.BGM = nil
	}
	if run.OutputDurationMs > 0 {
		work.DurationMs = run.OutputDurationMs
	}
	if run.CompletedAt != nil {
		completedAt := *run.CompletedAt
		work.CompletedAt = &completedAt
	} else {
		work.CompletedAt = nil
	}
	plan, err := s.GetEditPlan(ctx, run.ID)
	if err == nil {
		if len(plan.NarrationSegments) > 0 {
			work.NarrationSegments = cloneNarrationSegments(plan.NarrationSegments)
		}
		if plan.TimelineDurationMs > 0 {
			work.DurationMs = plan.TimelineDurationMs
		}
		work.VisualBeats = cloneVisualBeats(plan.VisualBeats)
		work.EditPlan = editPlanWorkClips(plan.Clips)
	} else if !errors.Is(err, ErrEditPlanNotFound) {
		return VoiceoverWork{}, err
	}
	return work, nil
}

func editPlanWorkClips(clips []EditPlanClip) []VoiceoverEditPlanClip {
	items := make([]VoiceoverEditPlanClip, 0, len(clips))
	for _, clip := range clips {
		items = append(items, VoiceoverEditPlanClip{
			ID:                 clip.ID,
			VisualBeatID:       clip.VisualBeatID,
			NarrationSegmentID: clip.NarrationSegmentID,
			AssetID:            clip.AssetID,
			SpeechSegmentID:    clip.SpeechSegmentID,
			StartMs:            clip.StartMs,
			EndMs:              clip.EndMs,
			SourceInMs:         clip.SourceInMs,
			SourceOutMs:        clip.SourceOutMs,
			Label:              clip.Label,
			VisualGoal:         clip.VisualGoal,
			SourceType:         clip.SourceType,
			UseOriginalAudio:   clip.UseOriginalAudio,
		})
	}
	return items
}

const generationRunColumns = `
	SELECT id::text, product_id::text, COALESCE(created_by_user_id::text, ''), COALESCE(created_by_name_snapshot, ''),
		COALESCE(voiceover_task_id::text, ''), COALESCE(script_variant_id::text, ''), COALESCE(voiceover_id::text, ''),
		status, stage, progress, COALESCE(error_message, ''), config_snapshot,
		COALESCE(output_storage_key, ''), COALESCE(output_mime_type, ''), COALESCE(output_duration_ms, 0),
		COALESCE(output_width, 0), COALESCE(output_height, 0), COALESCE(output_file_size_bytes, 0),
		COALESCE(renderer, ''), COALESCE(render_version, ''),
		created_at, updated_at, completed_at`

type generationRunScanner interface {
	Scan(...any) error
}

func scanGenerationRun(row generationRunScanner) (GenerationRun, error) {
	var run GenerationRun
	var snapshot []byte
	var completedAt pgtype.Timestamptz
	if err := row.Scan(
		&run.ID,
		&run.ProductID,
		&run.CreatedByUserID,
		&run.CreatedByName,
		&run.VoiceoverTaskID,
		&run.ScriptVariantID,
		&run.VoiceoverID,
		&run.Status,
		&run.Stage,
		&run.Progress,
		&run.ErrorMessage,
		&snapshot,
		&run.OutputStorageKey,
		&run.OutputMimeType,
		&run.OutputDurationMs,
		&run.OutputWidth,
		&run.OutputHeight,
		&run.OutputFileSizeBytes,
		&run.Renderer,
		&run.RenderVersion,
		&run.CreatedAt,
		&run.UpdatedAt,
		&completedAt,
	); err != nil {
		return GenerationRun{}, err
	}
	run.ConfigSnapshot = map[string]any{}
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &run.ConfigSnapshot)
	}
	if completedAt.Valid {
		value := completedAt.Time
		run.CompletedAt = &value
	}
	return run, nil
}

type editPlanScanner interface {
	Scan(...any) error
}

func scanEditPlan(row editPlanScanner) (EditPlan, error) {
	var plan EditPlan
	if err := row.Scan(
		&plan.ID,
		&plan.GenerationRunID,
		&plan.ScriptVariantID,
		&plan.VoiceoverID,
		&plan.Status,
		&plan.CandidateSnapshot,
		&plan.PlanJSON,
		&plan.LLMProvider,
		&plan.LLMModel,
		&plan.PromptVersion,
		&plan.ErrorMessage,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	); err != nil {
		return EditPlan{}, err
	}
	return plan, nil
}

func validateGenerationRunStage(stage string) error {
	switch stage {
	case generationRunStageQueued,
		generationRunStageVoicing,
		generationRunStageAligning,
		generationRunStageRetrieving,
		generationRunStagePlanning,
		generationRunStagePlanReady,
		generationRunStageRendering,
		generationRunStageCompleted,
		generationRunStageFailed:
		return nil
	default:
		return fmt.Errorf("invalid generation run stage %q", stage)
	}
}

func validateGenerationRunTaskStage(stage string) error {
	if stage == generationRunTaskStageVoiceover || stage == generationRunTaskStageEditPlan || stage == generationRunTaskStageRender {
		return nil
	}
	return fmt.Errorf("invalid generation run task stage %q", stage)
}

func validateGenerationRunRetryMode(mode GenerationRunRetryMode) error {
	if mode == GenerationRunRetryEditPlan || mode == GenerationRunRetryVoiceover || mode == GenerationRunRetryRender {
		return nil
	}
	return fmt.Errorf("invalid generation run retry mode %q", mode)
}

func retryStage(mode GenerationRunRetryMode) (string, int) {
	if mode == GenerationRunRetryVoiceover {
		return generationRunStageVoicing, 8
	}
	if mode == GenerationRunRetryRender {
		return generationRunStagePlanReady, 88
	}
	return generationRunStageRetrieving, 76
}

func validateEditPlanForStorage(plan EditPlan) error {
	if normalizeID(plan.GenerationRunID) == "" || normalizeID(plan.ScriptVariantID) == "" || normalizeID(plan.VoiceoverID) == "" {
		return fmt.Errorf("edit plan run, script variant, and voiceover are required")
	}
	if plan.Status != "ready" && plan.Status != "planning" && plan.Status != "queued" && plan.Status != "failed" {
		return fmt.Errorf("invalid edit plan status %q", plan.Status)
	}
	if plan.Status == "ready" && (len(plan.VisualBeats) == 0 || len(plan.Clips) == 0) {
		return fmt.Errorf("ready edit plan must contain visual beats and clips")
	}
	if len(plan.Clips) > 0 && len(plan.VisualBeats) == 0 {
		return fmt.Errorf("clip segments require visual beats")
	}
	if err := validateEditPlanNarrationTimeline(plan); err != nil {
		return err
	}
	visualBeats := map[string]VisualBeat{}
	expectedStartMs := 0
	for index, beat := range plan.VisualBeats {
		if normalizeID(beat.ID) == "" || normalizeID(beat.NarrationSegmentID) == "" || strings.TrimSpace(beat.Label) == "" || strings.TrimSpace(beat.VisualGoal) == "" {
			return fmt.Errorf("visual beat %d is incomplete", index+1)
		}
		if beat.StartMs != expectedStartMs || beat.EndMs <= beat.StartMs {
			return fmt.Errorf("visual beat %d timeline range is invalid", index+1)
		}
		if beat.SourceType != "visual_only" && beat.SourceType != "talking_head" && beat.SourceType != "mixed" {
			return fmt.Errorf("visual beat %d source type is invalid", index+1)
		}
		if !isVisualBeatDurationClass(normalizeVisualBeatDurationClass(beat.DurationClass)) {
			return fmt.Errorf("visual beat %d duration class is invalid", index+1)
		}
		if !isVisualBeatDurationValid(normalizeVisualBeatDurationClass(beat.DurationClass), beat.EndMs-beat.StartMs) {
			return fmt.Errorf("visual beat %d duration does not match its duration class", index+1)
		}
		if _, exists := visualBeats[beat.ID]; exists {
			return fmt.Errorf("visual beat %q is repeated", beat.ID)
		}
		visualBeats[beat.ID] = beat
		expectedStartMs = beat.EndMs
	}
	clipCounts := make(map[string]int, len(plan.VisualBeats))
	longestClipByVisualBeat := make(map[string]int, len(plan.VisualBeats))
	usedAssetIDs := make(map[string]int, len(plan.Clips))
	expectedClipStartMs := 0
	for index, clip := range plan.Clips {
		assetID := normalizeID(clip.AssetID)
		if normalizeID(clip.VisualBeatID) == "" || normalizeID(clip.NarrationSegmentID) == "" || assetID == "" {
			return fmt.Errorf("clip %d references are required", index+1)
		}
		beat, exists := visualBeats[clip.VisualBeatID]
		if !exists || beat.NarrationSegmentID != clip.NarrationSegmentID {
			return fmt.Errorf("clip %d does not match its visual beat", index+1)
		}
		if clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
			return fmt.Errorf("clip %d source range is invalid", index+1)
		}
		if clip.StartMs != expectedClipStartMs || clip.EndMs <= clip.StartMs || clip.TimelineDurationMs != clip.EndMs-clip.StartMs {
			return fmt.Errorf("clip %d timeline range is invalid", index+1)
		}
		if clip.StartMs < beat.StartMs || clip.EndMs > beat.EndMs {
			return fmt.Errorf("clip %d crosses its visual beat boundary", index+1)
		}
		if clip.SourceOutMs-clip.SourceInMs != clip.TimelineDurationMs {
			return fmt.Errorf("clip %d source duration does not match its timeline duration", index+1)
		}
		if previousIndex, exists := usedAssetIDs[assetID]; exists {
			return fmt.Errorf("clip %d reuses asset %q already selected by clip %d", index+1, assetID, previousIndex+1)
		}
		usedAssetIDs[assetID] = index
		if clip.TimelineDurationMs < modelgateway.MinimumEditPlanClipDurationMs {
			return fmt.Errorf("clip %d is shorter than %dms", index+1, modelgateway.MinimumEditPlanClipDurationMs)
		}
		if clip.TimelineDurationMs > modelgateway.MaximumEditPlanClipDurationMs {
			return fmt.Errorf("clip %d is longer than %dms", index+1, modelgateway.MaximumEditPlanClipDurationMs)
		}
		if clip.SourceType != "visual_only" && clip.SourceType != "talking_head" {
			return fmt.Errorf("clip %d source type is invalid", index+1)
		}
		clipCounts[clip.VisualBeatID]++
		if clip.TimelineDurationMs > longestClipByVisualBeat[clip.VisualBeatID] {
			longestClipByVisualBeat[clip.VisualBeatID] = clip.TimelineDurationMs
		}
		if clipCounts[clip.VisualBeatID] > modelgateway.MaximumEditPlanClipsPerBeat {
			return fmt.Errorf("visual beat %q has too many clips", clip.VisualBeatID)
		}
		expectedClipStartMs = clip.EndMs
	}
	if plan.Status == "ready" {
		if expectedClipStartMs != expectedStartMs {
			return fmt.Errorf("clip segments do not cover the visual timeline")
		}
		for beatID := range visualBeats {
			if clipCounts[beatID] == 0 {
				return fmt.Errorf("visual beat %q has no clips", beatID)
			}
			beat := visualBeats[beatID]
			if beat.DurationClass == VisualBeatDurationAction && longestClipByVisualBeat[beatID] < 2800 {
				return fmt.Errorf("action visual beat %q has no complete action clip", beatID)
			}
		}
	}
	return nil
}

func validateEditPlanNarrationTimeline(plan EditPlan) error {
	if plan.SourceDurationMs == 0 && plan.TimelineDurationMs == 0 && len(plan.NarrationSegments) == 0 && len(plan.NarrationPauses) == 0 {
		return nil
	}
	if plan.SourceDurationMs <= 0 || plan.TimelineDurationMs < plan.SourceDurationMs || len(plan.NarrationSegments) == 0 {
		return fmt.Errorf("edit plan narration timeline is incomplete")
	}
	previousEndMs := 0
	for index, segment := range plan.NarrationSegments {
		if normalizeID(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" || segment.StartMs < previousEndMs || segment.EndMs <= segment.StartMs || segment.EndMs > plan.TimelineDurationMs {
			return fmt.Errorf("edit plan narration segment %d is invalid", index+1)
		}
		previousEndMs = segment.EndMs
	}
	pauseDurationMs := 0
	previousPauseAfterMs := -1
	for index, pause := range plan.NarrationPauses {
		if pause.AfterMs <= previousPauseAfterMs || pause.AfterMs <= 0 || pause.AfterMs > plan.SourceDurationMs || pause.DurationMs <= 0 {
			return fmt.Errorf("edit plan narration pause %d is invalid", index+1)
		}
		previousPauseAfterMs = pause.AfterMs
		pauseDurationMs += pause.DurationMs
	}
	if plan.SourceDurationMs+pauseDurationMs != plan.TimelineDurationMs {
		return fmt.Errorf("edit plan narration pauses do not match timeline duration")
	}
	if len(plan.VisualBeats) > 0 && plan.VisualBeats[len(plan.VisualBeats)-1].EndMs != plan.TimelineDurationMs {
		return fmt.Errorf("visual beats do not match narration timeline duration")
	}
	return nil
}

func generationRunStageLabel(stage string) string {
	switch stage {
	case generationRunStageQueued:
		return "等待生成"
	case generationRunStageVoicing:
		return "生成旁白"
	case generationRunStageAligning:
		return "识别旁白"
	case generationRunStageRetrieving:
		return "召回素材"
	case generationRunStagePlanning:
		return "生成编排"
	case generationRunStagePlanReady:
		return "编排完成，等待渲染"
	case generationRunStageRendering:
		return "正在渲染"
	case generationRunStageCompleted:
		return "已完成"
	case generationRunStageFailed:
		return "生成失败"
	default:
		return "生成中"
	}
}

func cloneGenerationRun(run GenerationRun) GenerationRun {
	run.ConfigSnapshot = cloneRunObject(run.ConfigSnapshot)
	if run.CompletedAt != nil {
		value := *run.CompletedAt
		run.CompletedAt = &value
	}
	return run
}

func cloneEditPlan(plan EditPlan) EditPlan {
	plan.CandidateSnapshot = append(json.RawMessage(nil), plan.CandidateSnapshot...)
	plan.PlanJSON = append(json.RawMessage(nil), plan.PlanJSON...)
	plan.VisualBeats = cloneVisualBeats(plan.VisualBeats)
	plan.Clips = cloneEditPlanClips(plan.Clips)
	plan.NarrationSegments = cloneNarrationSegments(plan.NarrationSegments)
	plan.NarrationPauses = cloneNarrationPauses(plan.NarrationPauses)
	return plan
}

func cloneNarrationSegments(segments []NarrationSegment) []NarrationSegment {
	result := append([]NarrationSegment(nil), segments...)
	for index := range result {
		if result[index].SynthesisUnitIndex != nil {
			unitIndex := *result[index].SynthesisUnitIndex
			result[index].SynthesisUnitIndex = &unitIndex
		}
	}
	return result
}

func cloneNarrationPauses(pauses []NarrationPause) []NarrationPause {
	return append([]NarrationPause(nil), pauses...)
}

func decodeEditPlanTimeline(plan *EditPlan) {
	if plan == nil || len(plan.PlanJSON) == 0 {
		return
	}
	var payload struct {
		SourceDurationMs   int                `json:"source_duration_ms"`
		TimelineDurationMs int                `json:"timeline_duration_ms"`
		NarrationSegments  []NarrationSegment `json:"narration_segments"`
		NarrationPauses    []NarrationPause   `json:"narration_pauses"`
	}
	if err := json.Unmarshal(plan.PlanJSON, &payload); err != nil {
		return
	}
	plan.SourceDurationMs = payload.SourceDurationMs
	plan.TimelineDurationMs = payload.TimelineDurationMs
	plan.NarrationSegments = cloneNarrationSegments(payload.NarrationSegments)
	plan.NarrationPauses = cloneNarrationPauses(payload.NarrationPauses)
}

func cloneEditPlanClips(clips []EditPlanClip) []EditPlanClip {
	return append([]EditPlanClip(nil), clips...)
}

func cloneVisualBeats(beats []VisualBeat) []VisualBeat {
	return append([]VisualBeat(nil), beats...)
}

func cloneRunObject(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	result := map[string]any{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func firstRunObject(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func generationRunSnapshotJSON(value map[string]any) (string, error) {
	encoded, err := json.Marshal(firstRunObject(value))
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func mustRunJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func normalizeID(value string) string {
	return strings.TrimSpace(value)
}
