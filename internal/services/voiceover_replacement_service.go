package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrVoiceoverReplacementNotFound = errors.New("voiceover replacement not found")
	ErrVoiceoverReplacementActive   = errors.New("voiceover replacement is active")
	ErrVoiceoverReplacementNotReady = errors.New("voiceover replacement is not ready")
)

func (s *GenerationRunService) CreateVoiceoverReplacement(
	ctx context.Context,
	runID string,
	taskID string,
	variantID string,
	voiceoverID string,
	userID string,
) (VoiceoverReplacement, error) {
	runID, taskID = normalizeID(runID), normalizeID(taskID)
	variantID, voiceoverID = normalizeID(variantID), normalizeID(voiceoverID)
	if runID == "" || taskID == "" || variantID == "" || voiceoverID == "" {
		return VoiceoverReplacement{}, ErrInvalidVoiceInput
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for id, current := range s.memoryVoiceoverReplacements {
			if current.GenerationRunID != runID || (current.Status != "generating" && current.Status != "ready" && current.Status != "applying") {
				continue
			}
			if current.Status == "applying" {
				return VoiceoverReplacement{}, ErrVoiceoverReplacementActive
			}
			current.Status = "cancelled"
			current.UpdatedAt = time.Now()
			s.memoryVoiceoverReplacements[id] = current
		}
		now := time.Now()
		replacement := VoiceoverReplacement{
			ID: uuid.NewString(), GenerationRunID: runID, GenerationTaskID: taskID,
			ScriptVariantID: variantID, VoiceoverID: voiceoverID, Status: "generating",
			CreatedByUserID: normalizeID(userID), CreatedAt: now, UpdatedAt: now,
		}
		s.memoryVoiceoverReplacements[replacement.ID] = replacement
		return replacement, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return VoiceoverReplacement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var applying bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM voiceover_replacements
			WHERE generation_run_id = $1::uuid AND status = 'applying'
		)`, runID).Scan(&applying); err != nil {
		return VoiceoverReplacement{}, err
	}
	if applying {
		return VoiceoverReplacement{}, ErrVoiceoverReplacementActive
	}
	if _, err := tx.Exec(ctx, `
		UPDATE voiceover_replacements
		SET status = 'cancelled', updated_at = now()
		WHERE generation_run_id = $1::uuid AND status IN ('generating', 'ready')`, runID); err != nil {
		return VoiceoverReplacement{}, err
	}
	var replacementID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO voiceover_replacements (
			generation_run_id, generation_task_id, script_variant_id, voiceover_id, created_by_user_id
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, NULLIF($5, '')::uuid)
		RETURNING id::text`, runID, taskID, variantID, voiceoverID, normalizeID(userID)).Scan(&replacementID); err != nil {
		return VoiceoverReplacement{}, err
	}
	replacement, err := scanVoiceoverReplacement(tx.QueryRow(ctx, voiceoverReplacementColumns+` FROM voiceover_replacements WHERE id = $1::uuid`, replacementID))
	if err != nil {
		return VoiceoverReplacement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return VoiceoverReplacement{}, err
	}
	return s.enrichVoiceoverReplacement(ctx, replacement), nil
}

func (s *GenerationRunService) GetVoiceoverReplacement(ctx context.Context, replacementID string) (VoiceoverReplacement, error) {
	replacementID = normalizeID(replacementID)
	if replacementID == "" {
		return VoiceoverReplacement{}, ErrVoiceoverReplacementNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		replacement, ok := s.memoryVoiceoverReplacements[replacementID]
		s.mu.RUnlock()
		if !ok {
			return VoiceoverReplacement{}, ErrVoiceoverReplacementNotFound
		}
		return s.enrichVoiceoverReplacement(ctx, replacement), nil
	}
	replacement, err := scanVoiceoverReplacement(s.pool.QueryRow(ctx, voiceoverReplacementColumns+` FROM voiceover_replacements WHERE id = $1::uuid`, replacementID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VoiceoverReplacement{}, ErrVoiceoverReplacementNotFound
	}
	if err != nil {
		return VoiceoverReplacement{}, err
	}
	return s.enrichVoiceoverReplacement(ctx, replacement), nil
}

func (s *GenerationRunService) GetCurrentVoiceoverReplacement(ctx context.Context, runID string) (VoiceoverReplacement, error) {
	runID = normalizeID(runID)
	if s.pool == nil {
		s.mu.RLock()
		var latest VoiceoverReplacement
		for _, replacement := range s.memoryVoiceoverReplacements {
			if replacement.GenerationRunID == runID && replacement.CreatedAt.After(latest.CreatedAt) {
				latest = replacement
			}
		}
		s.mu.RUnlock()
		if latest.ID == "" {
			return VoiceoverReplacement{}, ErrVoiceoverReplacementNotFound
		}
		return s.enrichVoiceoverReplacement(ctx, latest), nil
	}
	replacement, err := scanVoiceoverReplacement(s.pool.QueryRow(ctx, voiceoverReplacementColumns+`
		FROM voiceover_replacements WHERE generation_run_id = $1::uuid ORDER BY created_at DESC LIMIT 1`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VoiceoverReplacement{}, ErrVoiceoverReplacementNotFound
	}
	if err != nil {
		return VoiceoverReplacement{}, err
	}
	return s.enrichVoiceoverReplacement(ctx, replacement), nil
}

func (s *GenerationRunService) MarkVoiceoverReplacementReady(ctx context.Context, replacementID string) error {
	err := s.setVoiceoverReplacementStatus(ctx, replacementID, "ready", "", "")
	if !errors.Is(err, ErrVoiceoverReplacementNotReady) {
		return err
	}
	replacement, getErr := s.GetVoiceoverReplacement(ctx, replacementID)
	if getErr == nil && replacement.Status == "cancelled" {
		return nil
	}
	return err
}

func (s *GenerationRunService) MarkVoiceoverReplacementFailed(ctx context.Context, replacementID string, cause error) error {
	if cause == nil {
		return nil
	}
	return s.setVoiceoverReplacementStatus(ctx, replacementID, "failed", cause.Error(), "")
}

func (s *GenerationRunService) MarkVoiceoverReplacementApplying(ctx context.Context, replacementID string, renderTaskID string) error {
	return s.setVoiceoverReplacementStatus(ctx, replacementID, "applying", "", renderTaskID)
}

func (s *GenerationRunService) CancelVoiceoverReplacement(ctx context.Context, replacementID string) error {
	replacement, err := s.GetVoiceoverReplacement(ctx, replacementID)
	if err != nil {
		return err
	}
	if replacement.Status == "applying" {
		return ErrVoiceoverReplacementActive
	}
	if replacement.Status == "cancelled" {
		return nil
	}
	return s.setVoiceoverReplacementStatus(ctx, replacementID, "cancelled", "", "")
}

func (s *GenerationRunService) setVoiceoverReplacementStatus(ctx context.Context, replacementID string, status string, errorMessage string, renderTaskID string) error {
	replacementID = normalizeID(replacementID)
	if replacementID == "" {
		return ErrVoiceoverReplacementNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		replacement, ok := s.memoryVoiceoverReplacements[replacementID]
		if !ok {
			return ErrVoiceoverReplacementNotFound
		}
		if !voiceoverReplacementTransitionAllowed(replacement.Status, status) {
			return ErrVoiceoverReplacementNotReady
		}
		replacement.Status = status
		replacement.ErrorMessage = errorMessage
		if renderTaskID != "" {
			replacement.RenderTaskID = normalizeID(renderTaskID)
		}
		replacement.UpdatedAt = time.Now()
		s.memoryVoiceoverReplacements[replacementID] = replacement
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE voiceover_replacements
		SET status = $2, error_message = NULLIF($3, ''),
			render_task_id = COALESCE(NULLIF($4, '')::uuid, render_task_id), updated_at = now()
		WHERE id = $1::uuid AND (
			($2 = 'ready' AND status = 'generating') OR
			($2 = 'applying' AND status = 'ready') OR
			($2 = 'failed' AND status IN ('generating', 'ready', 'applying')) OR
			($2 = 'cancelled' AND status IN ('generating', 'ready', 'failed'))
		)`, replacementID, status, errorMessage, normalizeID(renderTaskID))
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM voiceover_replacements WHERE id = $1::uuid)`, replacementID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrVoiceoverReplacementNotFound
		}
		return ErrVoiceoverReplacementNotReady
	}
	return nil
}

func voiceoverReplacementTransitionAllowed(current string, next string) bool {
	switch next {
	case "ready":
		return current == "generating"
	case "applying":
		return current == "ready"
	case "failed":
		return current == "generating" || current == "ready" || current == "applying"
	case "cancelled":
		return current == "generating" || current == "ready" || current == "failed"
	default:
		return false
	}
}

func (s *GenerationRunService) enrichVoiceoverReplacement(ctx context.Context, replacement VoiceoverReplacement) VoiceoverReplacement {
	if s.voiceovers == nil || replacement.GenerationTaskID == "" {
		return replacement
	}
	work, err := s.voiceovers.GetVoiceoverWork(ctx, replacement.GenerationTaskID)
	if err != nil {
		return replacement
	}
	replacement.AudioURL = work.AudioURL
	replacement.DurationMs = work.DurationMs
	if replacement.Status == "generating" && work.Status == "failed" {
		replacement.Status = "failed"
		replacement.ErrorMessage = work.ErrorMessage
	}
	return replacement
}

const voiceoverReplacementColumns = `
	SELECT id::text, generation_run_id::text, generation_task_id::text,
		script_variant_id::text, voiceover_id::text, COALESCE(render_task_id::text, ''),
		status, COALESCE(error_message, ''), COALESCE(created_by_user_id::text, ''),
		created_at, updated_at, applied_at`

func scanVoiceoverReplacement(row generationRunScanner) (VoiceoverReplacement, error) {
	var replacement VoiceoverReplacement
	var appliedAt pgtype.Timestamptz
	if err := row.Scan(
		&replacement.ID, &replacement.GenerationRunID, &replacement.GenerationTaskID,
		&replacement.ScriptVariantID, &replacement.VoiceoverID, &replacement.RenderTaskID,
		&replacement.Status, &replacement.ErrorMessage, &replacement.CreatedByUserID,
		&replacement.CreatedAt, &replacement.UpdatedAt, &appliedAt,
	); err != nil {
		return VoiceoverReplacement{}, err
	}
	if appliedAt.Valid {
		value := appliedAt.Time
		replacement.AppliedAt = &value
	}
	return replacement, nil
}

func (s *GenerationRunService) CommitVoiceoverReplacementRender(
	ctx context.Context,
	replacementID string,
	basePlanUpdatedAt time.Time,
	plan EditPlan,
	output GenerationRenderOutput,
) (string, error) {
	replacement, err := s.GetVoiceoverReplacement(ctx, replacementID)
	if err != nil {
		return "", err
	}
	if replacement.Status != "applying" || plan.GenerationRunID != replacement.GenerationRunID || basePlanUpdatedAt.IsZero() {
		return "", ErrVoiceoverReplacementNotReady
	}
	if err := validateEditPlanForStorage(plan); err != nil {
		return "", err
	}
	if output.StorageKey == "" || output.MimeType == "" || output.DurationMs <= 0 || output.Width <= 0 || output.Height <= 0 || output.FileSizeBytes <= 0 || output.Renderer == "" || output.RenderVersion == "" {
		return "", errors.New("replacement render output is incomplete")
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		run, runOK := s.memoryRuns[replacement.GenerationRunID]
		currentPlan, planOK := s.memoryPlans[replacement.GenerationRunID]
		if !runOK || !planOK || !currentPlan.UpdatedAt.Equal(basePlanUpdatedAt) {
			return "", ErrEditPlanConflict
		}
		oldOutput := run.OutputStorageKey
		now := time.Now()
		plan.ID, plan.CreatedAt, plan.UpdatedAt = currentPlan.ID, currentPlan.CreatedAt, now
		s.memoryPlans[run.ID] = cloneEditPlan(plan)
		run.VoiceoverTaskID = replacement.GenerationTaskID
		run.ScriptVariantID = replacement.ScriptVariantID
		run.VoiceoverID = replacement.VoiceoverID
		run.OutputStorageKey, run.OutputMimeType = output.StorageKey, output.MimeType
		run.OutputDurationMs, run.OutputWidth, run.OutputHeight = output.DurationMs, output.Width, output.Height
		run.OutputFileSizeBytes, run.Renderer, run.RenderVersion = output.FileSizeBytes, output.Renderer, output.RenderVersion
		run.Status, run.Stage, run.Progress, run.ErrorMessage = generationRunStatusCompleted, generationRunStageCompleted, 100, ""
		run.CompletedAt, run.UpdatedAt = &now, now
		s.memoryRuns[run.ID] = run
		for taskID, link := range s.memoryTasks {
			if link.GenerationRunID == run.ID && (link.Stage == generationRunTaskStageVoiceover || link.Stage == generationRunTaskStageRender) {
				delete(s.memoryTasks, taskID)
			}
		}
		s.memoryTasks[replacement.GenerationTaskID] = GenerationRunTask{GenerationRunID: run.ID, GenerationTaskID: replacement.GenerationTaskID, Stage: generationRunTaskStageVoiceover, CreatedAt: now}
		if replacement.RenderTaskID != "" {
			s.memoryTasks[replacement.RenderTaskID] = GenerationRunTask{GenerationRunID: run.ID, GenerationTaskID: replacement.RenderTaskID, Stage: generationRunTaskStageRender, CreatedAt: now}
		}
		replacement.Status, replacement.AppliedAt, replacement.UpdatedAt = "applied", &now, now
		s.memoryVoiceoverReplacements[replacement.ID] = replacement
		return oldOutput, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentStatus, runID, oldOutput, editPlanID string
	if err := tx.QueryRow(ctx, `
		SELECT replacements.status, replacements.generation_run_id::text,
			COALESCE(runs.output_storage_key, ''), plans.id::text
		FROM voiceover_replacements replacements
		JOIN generation_runs runs ON runs.id = replacements.generation_run_id
		JOIN edit_plans plans ON plans.generation_run_id = runs.id
		WHERE replacements.id = $1::uuid
		FOR UPDATE OF replacements, runs, plans`, replacement.ID).Scan(&currentStatus, &runID, &oldOutput, &editPlanID); err != nil {
		return "", err
	}
	if currentStatus != "applying" || runID != plan.GenerationRunID {
		return "", ErrVoiceoverReplacementNotReady
	}
	var currentUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM edit_plans WHERE id = $1::uuid`, editPlanID).Scan(&currentUpdatedAt); err != nil {
		return "", err
	}
	if !currentUpdatedAt.Equal(basePlanUpdatedAt) {
		return "", ErrEditPlanConflict
	}
	if _, err := tx.Exec(ctx, `
		UPDATE edit_plans SET script_variant_id = $2::uuid, voiceover_id = $3::uuid,
			status = 'ready', candidate_snapshot = $4::jsonb, plan_json = $5::jsonb,
			error_message = NULL, updated_at = now()
		WHERE id = $1::uuid`, editPlanID, plan.ScriptVariantID, plan.VoiceoverID, []byte(plan.CandidateSnapshot), []byte(plan.PlanJSON)); err != nil {
		return "", err
	}
	for _, beat := range plan.VisualBeats {
		command, err := tx.Exec(ctx, `
			UPDATE visual_beats SET narration_segment_id = $3::uuid, start_ms = $4, end_ms = $5,
				duration_class = $6, updated_at = now()
			WHERE id = $1::uuid AND edit_plan_id = $2::uuid`, beat.ID, editPlanID, beat.NarrationSegmentID, beat.StartMs, beat.EndMs, beat.DurationClass)
		if err != nil || command.RowsAffected() != 1 {
			if err != nil {
				return "", err
			}
			return "", ErrEditPlanConflict
		}
	}
	for _, clip := range plan.Clips {
		command, err := tx.Exec(ctx, `
			UPDATE clip_segments SET narration_segment_id = $3::uuid,
				source_in_ms = $4, source_out_ms = $5, timeline_in_ms = $6,
				timeline_duration_ms = $7, updated_at = now()
			WHERE id = $1::uuid AND edit_plan_id = $2::uuid`, clip.ID, editPlanID, clip.NarrationSegmentID,
			clip.SourceInMs, clip.SourceOutMs, clip.StartMs, clip.TimelineDurationMs)
		if err != nil || command.RowsAffected() != 1 {
			if err != nil {
				return "", err
			}
			return "", ErrEditPlanConflict
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE generation_runs SET voiceover_task_id = $2::uuid, script_variant_id = $3::uuid,
			voiceover_id = $4::uuid, status = 'completed', stage = 'completed', progress = 100,
			error_message = NULL, output_storage_key = $5, output_mime_type = $6,
			output_duration_ms = $7, output_width = $8, output_height = $9,
			output_file_size_bytes = $10, renderer = $11, render_version = $12,
			completed_at = now(), updated_at = now()
		WHERE id = $1::uuid`, runID, replacement.GenerationTaskID, replacement.ScriptVariantID,
		replacement.VoiceoverID, output.StorageKey, output.MimeType, output.DurationMs,
		output.Width, output.Height, output.FileSizeBytes, output.Renderer, output.RenderVersion); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM generation_run_tasks
		WHERE generation_run_id = $1::uuid AND stage IN ('voiceover', 'render')`, runID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO generation_run_tasks (generation_run_id, generation_task_id, stage)
		VALUES ($1::uuid, $2::uuid, 'voiceover')`, runID, replacement.GenerationTaskID); err != nil {
		return "", err
	}
	if replacement.RenderTaskID != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO generation_run_tasks (generation_run_id, generation_task_id, stage)
			VALUES ($1::uuid, $2::uuid, 'render')`, runID, replacement.RenderTaskID); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE voiceover_replacements SET status = 'applied', error_message = NULL,
			applied_at = now(), updated_at = now() WHERE id = $1::uuid`, replacement.ID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return oldOutput, nil
}
