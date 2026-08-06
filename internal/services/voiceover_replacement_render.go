package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

func (s *GenerationRenderService) RenderVoiceoverReplacement(ctx context.Context, replacementID string) error {
	if s == nil || s.runs == nil || s.voiceovers == nil || s.assets == nil || s.renderer == nil {
		return fmt.Errorf("generation renderer is not configured")
	}
	select {
	case s.limiter <- struct{}{}:
		defer func() { <-s.limiter }()
	case <-ctx.Done():
		return ctx.Err()
	}
	replacement, err := s.runs.GetVoiceoverReplacement(ctx, replacementID)
	if err != nil {
		return err
	}
	if replacement.Status != "applying" {
		return ErrVoiceoverReplacementNotReady
	}
	run, err := s.runs.Get(ctx, replacement.GenerationRunID)
	if err != nil {
		return err
	}
	if run.Status != generationRunStatusCompleted || run.OutputStorageKey == "" {
		return fmt.Errorf("finished work changed before applying replacement voiceover")
	}
	basePlan, err := s.runs.GetEditPlan(ctx, run.ID)
	if err != nil {
		return err
	}
	oldWork, err := s.voiceovers.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		return err
	}
	newWork, err := s.voiceovers.GetVoiceoverWork(ctx, replacement.GenerationTaskID)
	if err != nil {
		return err
	}
	if newWork.Status != "completed" || newWork.AudioStorageKey == "" || len(newWork.NarrationSegments) == 0 {
		return fmt.Errorf("replacement voiceover is incomplete")
	}
	plan, err := RetimeEditPlanForVoiceover(basePlan, oldWork, newWork, replacement.ScriptVariantID, replacement.VoiceoverID, s.assets)
	if err != nil {
		return err
	}

	narrationPath, err := safeStoragePath(s.storageRoot, newWork.AudioStorageKey)
	if err != nil {
		return err
	}
	if err := requireRegularFile(narrationPath, "replacement narration audio"); err != nil {
		return err
	}
	bgmConfig := renderSnapshotBGM(run.ConfigSnapshot)
	bgmPath := ""
	if bgmConfig != nil {
		bgmPath, err = safeStoragePath(s.storageRoot, bgmConfig.StorageKey)
		if err != nil {
			return err
		}
		if err := requireRegularFile(bgmPath, "background music"); err != nil {
			return err
		}
	}
	clips := append([]EditPlanClip(nil), plan.Clips...)
	sort.SliceStable(clips, func(i, j int) bool { return clips[i].StartMs < clips[j].StartMs })
	renderClips, err := s.buildRenderClips(run, plan.TimelineDurationMs, clips)
	if err != nil {
		return err
	}
	subtitles := make([]ffmpeg.SubtitleCue, 0, len(plan.NarrationSegments))
	for _, segment := range plan.NarrationSegments {
		if text := SubtitleDisplayText(segment.Text); text != "" {
			subtitles = append(subtitles, ffmpeg.SubtitleCue{StartMs: segment.StartMs, EndMs: segment.EndMs, Text: text})
		}
	}
	pauses := make([]ffmpeg.AudioPause, 0, len(plan.NarrationPauses))
	for _, pause := range plan.NarrationPauses {
		pauses = append(pauses, ffmpeg.AudioPause{AfterMs: pause.AfterMs, DurationMs: pause.DurationMs})
	}

	width := renderSnapshotInt(run.ConfigSnapshot, "output_width", defaultRenderWidth, 64, 3840)
	height := renderSnapshotInt(run.ConfigSnapshot, "output_height", defaultRenderHeight, 64, 3840)
	fps := renderSnapshotInt(run.ConfigSnapshot, "output_fps", defaultRenderFPS, 1, 60)
	workDir := filepath.Join(s.storageRoot, "temp", "renders", run.ID, uuid.NewString())
	defer os.RemoveAll(workDir)
	outputKey := path.Join("renders", "generations", run.ID, "final-voice-"+uuid.NewString()+".mp4")
	finalPath, err := safeStoragePath(s.storageRoot, outputKey)
	if err != nil {
		return err
	}
	temporaryOutput := filepath.Join(workDir, "final.tmp.mp4")
	s.logger.Info("voiceover replacement render started",
		slog.String("generation_run_id", run.ID), slog.String("replacement_id", replacement.ID),
		slog.Int("duration_ms", plan.TimelineDurationMs))
	probe, err := s.renderer.RenderTimeline(ctx, ffmpeg.RenderInput{
		Clips: renderClips, NarrationPath: narrationPath, NarrationPauses: pauses,
		BGMPath: bgmPath, BGMGainDB: resolvedBGMGain(bgmConfig),
		BGMFadeInMs: resolvedBGMFadeIn(bgmConfig), BGMFadeOutMs: resolvedBGMFadeOut(bgmConfig),
		Subtitles: subtitles, SubtitleStyle: renderSnapshotSubtitleStyle(run.ConfigSnapshot),
		OutputPath: temporaryOutput, WorkDir: workDir, DurationMs: plan.TimelineDurationMs,
		Width: width, Height: height, FPS: fps,
	})
	if err != nil {
		return err
	}
	if probe.Width != width || probe.Height != height || absInt(probe.DurationMs-plan.TimelineDurationMs) > 500 {
		return fmt.Errorf("replacement render output does not match requested dimensions or duration")
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return err
	}
	if err := os.Rename(temporaryOutput, finalPath); err != nil {
		return fmt.Errorf("publish replacement render output: %w", err)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return err
	}
	output := GenerationRenderOutput{
		StorageKey: outputKey, MimeType: "video/mp4", DurationMs: probe.DurationMs,
		Width: probe.Width, Height: probe.Height, FileSizeBytes: info.Size(),
		Renderer: "ffmpeg", RenderVersion: ffmpegRenderVersion,
	}
	oldOutputKey, err := s.runs.CommitVoiceoverReplacementRender(ctx, replacement.ID, basePlan.UpdatedAt, plan, output)
	if err != nil {
		_ = os.Remove(finalPath)
		return err
	}
	if oldOutputKey != "" && oldOutputKey != outputKey {
		if oldPath, pathErr := safeStoragePath(s.storageRoot, oldOutputKey); pathErr == nil {
			if removeErr := os.Remove(oldPath); removeErr != nil && !os.IsNotExist(removeErr) {
				s.logger.Warn("remove replaced generation output failed", slog.String("storage_key", oldOutputKey), slog.Any("error", removeErr))
			}
		}
	}
	return nil
}
