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
)

var (
	ErrGenerationRunNotFound = errors.New("generation run not found")
	ErrEditPlanNotFound      = errors.New("edit plan not found")
)

type GenerationRun struct {
	ID              string         `json:"id"`
	ProductID       string         `json:"product_id"`
	CreatedByUserID string         `json:"created_by_user_id,omitempty"`
	VoiceoverTaskID string         `json:"voiceover_task_id,omitempty"`
	ScriptVariantID string         `json:"script_variant_id,omitempty"`
	VoiceoverID     string         `json:"voiceover_id,omitempty"`
	Status          string         `json:"status"`
	Stage           string         `json:"stage"`
	Progress        int            `json:"progress"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	ConfigSnapshot  map[string]any `json:"config_snapshot,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
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
	ID                string          `json:"id"`
	GenerationRunID   string          `json:"generation_run_id"`
	ScriptVariantID   string          `json:"script_variant_id"`
	VoiceoverID       string          `json:"voiceover_id"`
	Status            string          `json:"status"`
	CandidateSnapshot json.RawMessage `json:"candidate_snapshot,omitempty"`
	PlanJSON          json.RawMessage `json:"plan_json,omitempty"`
	LLMProvider       string          `json:"llm_provider,omitempty"`
	LLMModel          string          `json:"llm_model,omitempty"`
	PromptVersion     string          `json:"prompt_version,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	VisualBeats       []VisualBeat    `json:"visual_beats"`
	Clips             []EditPlanClip  `json:"clips"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateGenerationRunInput struct {
	ProductID       string
	CreatedByUserID string
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
	if s.pool == nil {
		now := time.Now()
		run := GenerationRun{
			ID:              uuid.NewString(),
			ProductID:       input.ProductID,
			CreatedByUserID: input.CreatedByUserID,
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
		INSERT INTO generation_runs (product_id, created_by_user_id, status, stage, progress, config_snapshot)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, 'generating', 'queued', 4, $3::jsonb)
		RETURNING id::text`, input.ProductID, input.CreatedByUserID, snapshot).Scan(&runID); err != nil {
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

func (s *GenerationRunService) SaveEditPlan(ctx context.Context, plan EditPlan) (EditPlan, error) {
	if err := validateEditPlanForStorage(plan); err != nil {
		return EditPlan{}, err
	}
	if len(plan.CandidateSnapshot) == 0 {
		plan.CandidateSnapshot = mustRunJSON(map[string]any{
			"visual_beats":   plan.VisualBeats,
			"candidate_sets": []CandidateSet{},
			"clips":          plan.Clips,
		})
	}
	if len(plan.PlanJSON) == 0 {
		plan.PlanJSON = mustRunJSON(map[string]any{
			"visual_beats":   plan.VisualBeats,
			"candidate_sets": []CandidateSet{},
			"clips":          plan.Clips,
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
				id, edit_plan_id, beat_index, narration_segment_id,
				start_ms, end_ms, label, selling_point, visual_goal, source_type
			) VALUES (
				$1::uuid, $2::uuid, $3, $4::uuid,
				$5, $6, $7, $8, $9, $10
			)`,
			beat.ID,
			stored.ID,
			index,
			beat.NarrationSegmentID,
			beat.StartMs,
			beat.EndMs,
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
	visualBeatRows, err := s.pool.Query(ctx, `
		SELECT id::text, narration_segment_id::text, start_ms, end_ms,
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
			&beat.StartMs,
			&beat.EndMs,
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
	if run.CompletedAt != nil {
		completedAt := *run.CompletedAt
		work.CompletedAt = &completedAt
	} else {
		work.CompletedAt = nil
	}
	plan, err := s.GetEditPlan(ctx, run.ID)
	if err == nil {
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
	SELECT id::text, product_id::text, COALESCE(created_by_user_id::text, ''),
		COALESCE(voiceover_task_id::text, ''), COALESCE(script_variant_id::text, ''), COALESCE(voiceover_id::text, ''),
		status, stage, progress, COALESCE(error_message, ''), config_snapshot, created_at, updated_at, completed_at`

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
		&run.VoiceoverTaskID,
		&run.ScriptVariantID,
		&run.VoiceoverID,
		&run.Status,
		&run.Stage,
		&run.Progress,
		&run.ErrorMessage,
		&snapshot,
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
	if stage == generationRunTaskStageVoiceover || stage == generationRunTaskStageEditPlan {
		return nil
	}
	return fmt.Errorf("invalid generation run task stage %q", stage)
}

func validateEditPlanForStorage(plan EditPlan) error {
	if normalizeID(plan.GenerationRunID) == "" || normalizeID(plan.ScriptVariantID) == "" || normalizeID(plan.VoiceoverID) == "" {
		return fmt.Errorf("edit plan run, script variant, and voiceover are required")
	}
	if plan.Status != "ready" && plan.Status != "planning" && plan.Status != "queued" && plan.Status != "failed" {
		return fmt.Errorf("invalid edit plan status %q", plan.Status)
	}
	if plan.Status == "ready" && (len(plan.VisualBeats) == 0 || len(plan.Clips) != len(plan.VisualBeats)) {
		return fmt.Errorf("ready edit plan must contain one clip for every visual beat")
	}
	if len(plan.Clips) > 0 && len(plan.VisualBeats) == 0 {
		return fmt.Errorf("clip segments require visual beats")
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
		if _, exists := visualBeats[beat.ID]; exists {
			return fmt.Errorf("visual beat %q is repeated", beat.ID)
		}
		visualBeats[beat.ID] = beat
		expectedStartMs = beat.EndMs
	}
	for index, clip := range plan.Clips {
		if normalizeID(clip.VisualBeatID) == "" || normalizeID(clip.NarrationSegmentID) == "" || normalizeID(clip.AssetID) == "" {
			return fmt.Errorf("clip %d references are required", index+1)
		}
		beat, exists := visualBeats[clip.VisualBeatID]
		if !exists || beat.NarrationSegmentID != clip.NarrationSegmentID {
			return fmt.Errorf("clip %d does not match its visual beat", index+1)
		}
		if clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
			return fmt.Errorf("clip %d source range is invalid", index+1)
		}
		if clip.StartMs < 0 || clip.EndMs <= clip.StartMs || clip.TimelineDurationMs != clip.EndMs-clip.StartMs {
			return fmt.Errorf("clip %d timeline range is invalid", index+1)
		}
		if clip.StartMs != beat.StartMs || clip.EndMs != beat.EndMs || clip.SourceOutMs-clip.SourceInMs != beat.EndMs-beat.StartMs {
			return fmt.Errorf("clip %d duration does not match its visual beat", index+1)
		}
		if clip.SourceType != "visual_only" && clip.SourceType != "talking_head" {
			return fmt.Errorf("clip %d source type is invalid", index+1)
		}
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
	return plan
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
