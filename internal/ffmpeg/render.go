package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const subtitleFileName = "subtitles.ass"

type RenderClip struct {
	InputPath        string
	SourceInMs       int
	SourceOutMs      int
	UseOriginalAudio bool
	HasAudio         bool
	AudioGainDB      float64
}

type SubtitleCue struct {
	StartMs int
	EndMs   int
	Text    string
}

type RenderInput struct {
	Clips         []RenderClip
	NarrationPath string
	Subtitles     []SubtitleCue
	OutputPath    string
	WorkDir       string
	DurationMs    int
	Width         int
	Height        int
	FPS           int
}

func RenderTimeline(ctx context.Context, input RenderInput) (ProbeResult, error) {
	if err := validateRenderInput(input); err != nil {
		return ProbeResult{}, err
	}
	if err := os.MkdirAll(input.WorkDir, 0755); err != nil {
		return ProbeResult{}, fmt.Errorf("create render work directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(input.OutputPath), 0755); err != nil {
		return ProbeResult{}, fmt.Errorf("create render output directory: %w", err)
	}
	if len(input.Subtitles) > 0 {
		content := buildASS(input.Subtitles, input.Width, input.Height)
		if err := os.WriteFile(filepath.Join(input.WorkDir, subtitleFileName), []byte(content), 0644); err != nil {
			return ProbeResult{}, fmt.Errorf("write render subtitles: %w", err)
		}
	}
	args, err := renderTimelineArgs(input)
	if err != nil {
		return ProbeResult{}, err
	}
	cmd := exec.CommandContext(ctx, ffmpegPath(), args...)
	cmd.Dir = input.WorkDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ProbeResult{}, fmt.Errorf("ffmpeg timeline render failed: %w: %s", err, tailText(output, 8192))
	}
	info, err := os.Stat(input.OutputPath)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("render output is missing: %w", err)
	}
	if info.Size() == 0 {
		return ProbeResult{}, fmt.Errorf("render output is empty")
	}
	result, err := Probe(ctx, input.OutputPath)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe render output: %w", err)
	}
	if result.Width <= 0 || result.Height <= 0 || result.DurationMs <= 0 || !result.HasAudio {
		return ProbeResult{}, fmt.Errorf("render output has invalid media streams")
	}
	return result, nil
}

func renderTimelineArgs(input RenderInput) ([]string, error) {
	if err := validateRenderInput(input); err != nil {
		return nil, err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	for _, clip := range input.Clips {
		args = append(args,
			"-ss", millisecondsAsSeconds(clip.SourceInMs),
			"-t", millisecondsAsSeconds(clip.SourceOutMs-clip.SourceInMs),
			"-i", clip.InputPath,
		)
	}
	args = append(args, "-i", input.NarrationPath)

	filters := make([]string, 0, len(input.Clips)*2+5)
	videoInputs := strings.Builder{}
	originalAudioInputs := strings.Builder{}
	hasOriginalAudio := false
	for _, clip := range input.Clips {
		if clip.UseOriginalAudio && clip.HasAudio {
			hasOriginalAudio = true
			break
		}
	}
	for index, clip := range input.Clips {
		duration := millisecondsAsSeconds(clip.SourceOutMs - clip.SourceInMs)
		filters = append(filters, fmt.Sprintf(
			"[%d:v:0]trim=start=0:duration=%s,setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=%d,format=yuv420p[v%d]",
			index, duration, input.Width, input.Height, input.Width, input.Height, input.FPS, index,
		))
		fmt.Fprintf(&videoInputs, "[v%d]", index)

		if hasOriginalAudio {
			if clip.UseOriginalAudio && clip.HasAudio {
				filters = append(filters, fmt.Sprintf(
					"[%d:a:0]atrim=start=0:duration=%s,asetpts=PTS-STARTPTS,aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo,volume=%.3fdB[oa%d]",
					index, duration, clip.AudioGainDB, index,
				))
			} else {
				filters = append(filters, fmt.Sprintf(
					"anullsrc=channel_layout=stereo:sample_rate=48000,atrim=duration=%s,asetpts=PTS-STARTPTS[oa%d]",
					duration, index,
				))
			}
			fmt.Fprintf(&originalAudioInputs, "[oa%d]", index)
		}
	}
	filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=1:a=0[video]", videoInputs.String(), len(input.Clips)))
	videoOutput := "[video]"
	if len(input.Subtitles) > 0 {
		filters = append(filters, fmt.Sprintf("[video]subtitles=filename=%s[video_with_subtitles]", subtitleFileName))
		videoOutput = "[video_with_subtitles]"
	}

	narrationIndex := len(input.Clips)
	totalDuration := millisecondsAsSeconds(input.DurationMs)
	filters = append(filters, fmt.Sprintf(
		"[%d:a:0]aresample=48000,aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo,apad,atrim=duration=%s,asetpts=PTS-STARTPTS[narration]",
		narrationIndex, totalDuration,
	))
	audioOutput := "[narration]"
	if hasOriginalAudio {
		filters = append(filters,
			fmt.Sprintf("%sconcat=n=%d:v=0:a=1[original_audio]", originalAudioInputs.String(), len(input.Clips)),
			"[narration][original_audio]amix=inputs=2:duration=first:dropout_transition=0:normalize=0[audio]",
		)
		audioOutput = "[audio]"
	}

	args = append(args,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", videoOutput,
		"-map", audioOutput,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "20",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ar", "48000",
		"-ac", "2",
		"-t", totalDuration,
		"-movflags", "+faststart",
		input.OutputPath,
	)
	return args, nil
}

func validateRenderInput(input RenderInput) error {
	if len(input.Clips) == 0 {
		return fmt.Errorf("render clips are required")
	}
	if strings.TrimSpace(input.NarrationPath) == "" || strings.TrimSpace(input.OutputPath) == "" || strings.TrimSpace(input.WorkDir) == "" {
		return fmt.Errorf("render narration, output, and work paths are required")
	}
	if input.DurationMs <= 0 || input.Width <= 0 || input.Height <= 0 || input.FPS <= 0 {
		return fmt.Errorf("render duration, dimensions, and fps must be positive")
	}
	for index, clip := range input.Clips {
		if strings.TrimSpace(clip.InputPath) == "" || clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
			return fmt.Errorf("render clip %d is invalid", index+1)
		}
	}
	for index, cue := range input.Subtitles {
		if cue.StartMs < 0 || cue.EndMs <= cue.StartMs || cue.EndMs > input.DurationMs || strings.TrimSpace(cue.Text) == "" {
			return fmt.Errorf("render subtitle %d is invalid", index+1)
		}
	}
	return nil
}

func buildASS(cues []SubtitleCue, width int, height int) string {
	fontSize := width * 54 / 1000
	if fontSize < 32 {
		fontSize = 32
	}
	marginV := height * 9 / 100
	var content strings.Builder
	fmt.Fprintf(&content, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\nScaledBorderAndShadow: yes\n\n", width, height)
	content.WriteString("[V4+ Styles]\n")
	content.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&content, "Style: Default,Noto Sans CJK SC,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&HB3000000,-1,0,0,0,100,100,0,0,3,0,0,2,56,56,%d,1\n\n", fontSize, marginV)
	content.WriteString("[Events]\n")
	content.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, cue := range cues {
		text := escapeASSText(strings.TrimSpace(cue.Text))
		if text == "" {
			continue
		}
		fmt.Fprintf(&content, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", formatASSTimestamp(cue.StartMs), formatASSTimestamp(cue.EndMs), text)
	}
	return content.String()
}

func formatASSTimestamp(milliseconds int) string {
	centiseconds := milliseconds / 10
	hours := centiseconds / 360000
	centiseconds %= 360000
	minutes := centiseconds / 6000
	centiseconds %= 6000
	seconds := centiseconds / 100
	centiseconds %= 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds)
}

func escapeASSText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "{", "\\{")
	return strings.ReplaceAll(text, "}", "\\}")
}

func millisecondsAsSeconds(milliseconds int) string {
	return fmt.Sprintf("%.3f", float64(milliseconds)/1000)
}

func tailText(value []byte, limit int) string {
	if len(value) <= limit {
		return string(value)
	}
	return string(value[len(value)-limit:])
}
