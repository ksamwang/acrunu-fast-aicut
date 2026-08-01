package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

const (
	defaultRenderWidth  = 1080
	defaultRenderHeight = 1920
	defaultRenderFPS    = 30
	ffmpegRenderVersion = "ffmpeg-v5-paced-vbr"
)

type generationTimelineRenderer interface {
	RenderTimeline(context.Context, ffmpeg.RenderInput) (ffmpeg.ProbeResult, error)
}

type ffmpegTimelineRenderer struct{}

func (ffmpegTimelineRenderer) RenderTimeline(ctx context.Context, input ffmpeg.RenderInput) (ffmpeg.ProbeResult, error) {
	return ffmpeg.RenderTimeline(ctx, input)
}

type GenerationRenderService struct {
	storageRoot string
	runs        *GenerationRunService
	voiceovers  *VoiceoverService
	assets      *ProductAssetService
	renderer    generationTimelineRenderer
	limiter     chan struct{}
	logger      *slog.Logger
}

type ClipReplacementRenderRequest struct {
	BaseEditPlanUpdatedAt time.Time
	Replacements          []EditPlanClipReplacement
}

func NewGenerationRenderService(
	storageRoot string,
	runs *GenerationRunService,
	voiceovers *VoiceoverService,
	assets *ProductAssetService,
	maxConcurrency int,
	logger *slog.Logger,
) *GenerationRenderService {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GenerationRenderService{
		storageRoot: storageRoot,
		runs:        runs,
		voiceovers:  voiceovers,
		assets:      assets,
		renderer:    ffmpegTimelineRenderer{},
		limiter:     make(chan struct{}, maxConcurrency),
		logger:      logger,
	}
}

func (s *GenerationRenderService) WithRenderer(renderer generationTimelineRenderer) *GenerationRenderService {
	if renderer != nil {
		s.renderer = renderer
	}
	return s
}

func (s *GenerationRenderService) Render(ctx context.Context, runID string) error {
	return s.render(ctx, runID, nil)
}

func (s *GenerationRenderService) RenderClipReplacements(ctx context.Context, runID string, request ClipReplacementRenderRequest) error {
	if request.BaseEditPlanUpdatedAt.IsZero() || len(request.Replacements) == 0 {
		return fmt.Errorf("clip replacement render request is incomplete")
	}
	return s.render(ctx, runID, &request)
}

func (s *GenerationRenderService) render(ctx context.Context, runID string, replacement *ClipReplacementRenderRequest) error {
	if s == nil || s.runs == nil || s.voiceovers == nil || s.assets == nil || s.renderer == nil {
		return fmt.Errorf("generation renderer is not configured")
	}
	select {
	case s.limiter <- struct{}{}:
		defer func() { <-s.limiter }()
	case <-ctx.Done():
		return ctx.Err()
	}

	run, err := s.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if replacement == nil && run.Status == generationRunStatusCompleted && run.OutputStorageKey != "" {
		outputPath, pathErr := safeStoragePath(s.storageRoot, run.OutputStorageKey)
		if pathErr == nil {
			if info, statErr := os.Stat(outputPath); statErr == nil && info.Size() > 0 {
				return nil
			}
		}
	}
	if run.Stage != generationRunStagePlanReady && run.Stage != generationRunStageRendering && run.Stage != generationRunStageFailed {
		return fmt.Errorf("generation run is not ready to render")
	}

	plan, err := s.runs.GetEditPlan(ctx, run.ID)
	if err != nil {
		return err
	}
	if replacement != nil {
		if !plan.UpdatedAt.Equal(replacement.BaseEditPlanUpdatedAt) {
			return ErrEditPlanConflict
		}
		plan, err = MaterializeClipReplacementPlan(run, plan, replacement.Replacements, s.assets)
		if err != nil {
			return err
		}
	}
	if plan.Status != "ready" || len(plan.Clips) == 0 {
		return fmt.Errorf("ready edit plan is required for rendering")
	}
	work, err := s.voiceovers.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		return err
	}
	if work.DurationMs <= 0 || work.AudioStorageKey == "" || len(work.NarrationSegments) == 0 {
		return fmt.Errorf("narration audio and segments are required for rendering")
	}
	if plan.SourceDurationMs > 0 && absInt(plan.SourceDurationMs-work.DurationMs) > 50 {
		return fmt.Errorf("edit plan narration source duration does not match voiceover")
	}
	renderDurationMs := work.DurationMs
	narrationSegments := work.NarrationSegments
	narrationPauses := []ffmpeg.AudioPause{}
	if plan.TimelineDurationMs > 0 {
		renderDurationMs = plan.TimelineDurationMs
	}
	if len(plan.NarrationSegments) > 0 {
		narrationSegments = plan.NarrationSegments
	}
	for _, pause := range plan.NarrationPauses {
		narrationPauses = append(narrationPauses, ffmpeg.AudioPause{AfterMs: pause.AfterMs, DurationMs: pause.DurationMs})
	}
	narrationPath, err := safeStoragePath(s.storageRoot, work.AudioStorageKey)
	if err != nil {
		return err
	}
	if err := requireRegularFile(narrationPath, "narration audio"); err != nil {
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
	renderClips, err := s.buildRenderClips(run, renderDurationMs, clips)
	if err != nil {
		return err
	}
	subtitles := make([]ffmpeg.SubtitleCue, 0, len(narrationSegments))
	for _, segment := range narrationSegments {
		text := SubtitleDisplayText(segment.Text)
		if text == "" {
			continue
		}
		subtitles = append(subtitles, ffmpeg.SubtitleCue{StartMs: segment.StartMs, EndMs: segment.EndMs, Text: text})
	}

	width := renderSnapshotInt(run.ConfigSnapshot, "output_width", defaultRenderWidth, 64, 3840)
	height := renderSnapshotInt(run.ConfigSnapshot, "output_height", defaultRenderHeight, 64, 3840)
	fps := renderSnapshotInt(run.ConfigSnapshot, "output_fps", defaultRenderFPS, 1, 60)
	workDir := filepath.Join(s.storageRoot, "temp", "renders", run.ID, uuid.NewString())
	defer os.RemoveAll(workDir)
	outputFileName := "final.mp4"
	if replacement != nil {
		outputFileName = "final-" + uuid.NewString() + ".mp4"
	}
	outputKey := path.Join("renders", "generations", run.ID, outputFileName)
	finalPath, err := safeStoragePath(s.storageRoot, outputKey)
	if err != nil {
		return err
	}
	temporaryOutput := filepath.Join(workDir, "final.tmp.mp4")

	s.logger.Info("generation render started",
		slog.String("generation_run_id", run.ID),
		slog.Int("clip_count", len(renderClips)),
		slog.Int("subtitle_count", len(subtitles)),
		slog.Int("duration_ms", renderDurationMs),
	)
	probe, err := s.renderer.RenderTimeline(ctx, ffmpeg.RenderInput{
		Clips:           renderClips,
		NarrationPath:   narrationPath,
		NarrationPauses: narrationPauses,
		BGMPath:         bgmPath,
		BGMGainDB:       resolvedBGMGain(bgmConfig),
		BGMFadeInMs:     resolvedBGMFadeIn(bgmConfig),
		BGMFadeOutMs:    resolvedBGMFadeOut(bgmConfig),
		Subtitles:       subtitles,
		SubtitleStyle:   renderSnapshotSubtitleStyle(run.ConfigSnapshot),
		OutputPath:      temporaryOutput,
		WorkDir:         workDir,
		DurationMs:      renderDurationMs,
		Width:           width,
		Height:          height,
		FPS:             fps,
	})
	if err != nil {
		return err
	}
	if probe.Width != width || probe.Height != height {
		return fmt.Errorf("render output dimensions are %dx%d, want %dx%d", probe.Width, probe.Height, width, height)
	}
	if difference := absInt(probe.DurationMs - renderDurationMs); difference > 500 {
		return fmt.Errorf("render output duration differs from narration by %dms", difference)
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("create final render directory: %w", err)
	}
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace existing render output: %w", err)
	}
	if err := os.Rename(temporaryOutput, finalPath); err != nil {
		return fmt.Errorf("publish render output: %w", err)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("stat final render output: %w", err)
	}
	output := GenerationRenderOutput{
		StorageKey:    outputKey,
		MimeType:      "video/mp4",
		DurationMs:    probe.DurationMs,
		Width:         probe.Width,
		Height:        probe.Height,
		FileSizeBytes: info.Size(),
		Renderer:      "ffmpeg",
		RenderVersion: ffmpegRenderVersion,
	}
	if replacement != nil {
		oldOutputKey, commitErr := s.runs.CommitClipReplacementRender(ctx, run.ID, replacement.BaseEditPlanUpdatedAt, plan, output)
		if commitErr != nil {
			_ = os.Remove(finalPath)
			return commitErr
		}
		if oldOutputKey != "" && oldOutputKey != outputKey {
			if oldOutputPath, pathErr := safeStoragePath(s.storageRoot, oldOutputKey); pathErr == nil {
				if removeErr := os.Remove(oldOutputPath); removeErr != nil && !os.IsNotExist(removeErr) {
					s.logger.Warn("remove replaced generation output failed", slog.String("storage_key", oldOutputKey), slog.Any("error", removeErr))
				}
			}
		}
	} else if err := s.runs.MarkRenderCompleted(ctx, run.ID, output); err != nil {
		return err
	}
	s.logger.Info("generation render completed",
		slog.String("generation_run_id", run.ID),
		slog.String("output_storage_key", outputKey),
		slog.Int64("output_size_bytes", info.Size()),
	)
	return nil
}

func renderSnapshotSubtitleStyle(snapshot map[string]any) ffmpeg.SubtitleStyle {
	style := ResolvedSubtitleStyle{}
	if raw, ok := snapshot["subtitle_style"]; ok {
		if payload, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(payload, &style)
		}
	}
	return ffmpeg.SubtitleStyle{
		FontFamily: style.FontFamily, FontWeight: style.FontWeight, TextColor: style.TextColor,
		BackgroundColor: style.BackgroundColor, BackgroundOpacity: style.BackgroundOpacity,
		OutlineColor: style.OutlineColor, OutlineWidth: style.OutlineWidth, Shadow: style.Shadow,
		MaxLines: style.MaxLines, VerticalPosition: style.VerticalPosition, TextAlign: style.TextAlign,
		VerticalOffsetRatio: style.VerticalOffset, VerticalPositionRatio: style.VerticalPositionRatio,
		MaxWidthRatio: style.MaxWidthRatio,
		FontSizeRatio: style.FontSizeRatio, MaxCharsPerLine: style.MaxCharsPerLine,
	}
}

func renderSnapshotBGM(snapshot map[string]any) *ResolvedBGMConfig {
	if snapshot == nil {
		return nil
	}
	raw, ok := snapshot["bgm"]
	if !ok {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	config := ResolvedBGMConfig{}
	if err := json.Unmarshal(payload, &config); err != nil || strings.TrimSpace(config.StorageKey) == "" {
		return nil
	}
	if config.GainDB < -30 || config.GainDB > 0 {
		config.GainDB = -12
	}
	if config.FadeInMs < 0 {
		config.FadeInMs = 0
	}
	if config.FadeOutMs < 0 {
		config.FadeOutMs = 0
	}
	return &config
}

func resolvedBGMGain(config *ResolvedBGMConfig) float64 {
	if config == nil {
		return 0
	}
	return config.GainDB
}

func resolvedBGMFadeIn(config *ResolvedBGMConfig) int {
	if config == nil {
		return 0
	}
	return config.FadeInMs
}

func resolvedBGMFadeOut(config *ResolvedBGMConfig) int {
	if config == nil {
		return 0
	}
	return config.FadeOutMs
}

func (s *GenerationRenderService) buildRenderClips(run GenerationRun, durationMs int, clips []EditPlanClip) ([]ffmpeg.RenderClip, error) {
	expectedStart := 0
	result := make([]ffmpeg.RenderClip, 0, len(clips))
	for index, clip := range clips {
		if clip.StartMs != expectedStart || clip.EndMs <= clip.StartMs || clip.EndMs-clip.StartMs != clip.SourceOutMs-clip.SourceInMs {
			return nil, fmt.Errorf("edit plan clip %d does not form a continuous timeline", index+1)
		}
		asset, ok := s.assets.GetAsset(clip.AssetID)
		if !ok || asset.ProductID != run.ProductID || strings.TrimSpace(asset.StorageKey) == "" {
			return nil, fmt.Errorf("edit plan clip %d references an unavailable asset", index+1)
		}
		if asset.DurationMs > 0 && clip.SourceOutMs > asset.DurationMs {
			return nil, fmt.Errorf("edit plan clip %d exceeds asset duration", index+1)
		}
		inputPath, err := safeStoragePath(s.storageRoot, asset.StorageKey)
		if err != nil {
			return nil, err
		}
		if err := requireRegularFile(inputPath, fmt.Sprintf("asset %s", asset.ID)); err != nil {
			return nil, err
		}
		result = append(result, ffmpeg.RenderClip{
			InputPath:        inputPath,
			SourceInMs:       clip.SourceInMs,
			SourceOutMs:      clip.SourceOutMs,
			UseOriginalAudio: clip.UseOriginalAudio,
			HasAudio:         asset.HasAudio,
			AudioGainDB:      clip.AudioGainDB,
		})
		expectedStart = clip.EndMs
	}
	if expectedStart != durationMs {
		return nil, fmt.Errorf("edit plan ends at %dms, want %dms", expectedStart, durationMs)
	}
	return result, nil
}

func safeStoragePath(root string, storageKey string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve storage root: %w", err)
	}
	storageKey = filepath.Clean(filepath.FromSlash(strings.TrimSpace(storageKey)))
	if storageKey == "." || filepath.IsAbs(storageKey) {
		return "", fmt.Errorf("invalid storage key")
	}
	fullPath := filepath.Join(root, storageKey)
	relative, err := filepath.Rel(root, fullPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key escapes storage root")
	}
	return fullPath, nil
}

func requireRegularFile(filePath string, label string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("%s is unavailable: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("%s is not a readable media file", label)
	}
	return nil
}

func renderSnapshotInt(snapshot map[string]any, key string, fallback int, minimum int, maximum int) int {
	value := fallback
	if raw, ok := snapshot[key]; ok {
		switch typed := raw.(type) {
		case int:
			value = typed
		case float64:
			value = int(typed)
		}
	}
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
