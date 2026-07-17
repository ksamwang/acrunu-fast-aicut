package ffmpeg

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

type SubtitleStyle struct {
	FontFamily            string
	FontWeight            int
	TextColor             string
	BackgroundColor       string
	BackgroundOpacity     float64
	OutlineColor          string
	OutlineWidth          float64
	Shadow                bool
	MaxLines              int
	VerticalPosition      string
	TextAlign             string
	VerticalOffsetRatio   float64
	VerticalPositionRatio float64
	MaxWidthRatio         float64
	FontSizeRatio         float64
	MaxCharsPerLine       int
}

type RenderInput struct {
	Clips         []RenderClip
	NarrationPath string
	Subtitles     []SubtitleCue
	SubtitleStyle SubtitleStyle
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
		content := buildASS(input.Subtitles, input.Width, input.Height, input.SubtitleStyle)
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

func buildASS(cues []SubtitleCue, width int, height int, rawStyle SubtitleStyle) string {
	style := normalizeSubtitleStyle(rawStyle)
	fontSize := int(math.Round(float64(width) * style.FontSizeRatio))
	if fontSize < 16 {
		fontSize = 16
	}
	marginV := int(math.Round(float64(height) * style.VerticalOffsetRatio))
	marginH := int(math.Round(float64(width) * (1 - style.MaxWidthRatio) / 2))
	if marginH < 0 {
		marginH = 0
	}
	borderStyle := 1
	if style.BackgroundOpacity > 0 {
		borderStyle = 3
	}
	bold := 0
	if style.FontWeight >= 600 {
		bold = -1
	}
	shadow := 0
	if style.Shadow {
		shadow = 2
	}
	alignment := subtitleASSAlignment("center", style.TextAlign)
	positionX := subtitleASSPositionX(style.TextAlign, width, marginH)
	positionY := int(math.Round(float64(height) * style.VerticalPositionRatio))
	var content strings.Builder
	fmt.Fprintf(&content, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\nScaledBorderAndShadow: yes\n\n", width, height)
	content.WriteString("[V4+ Styles]\n")
	content.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&content, "Style: Default,%s,%d,%s,%s,%s,%s,%d,0,0,0,100,100,0,0,%d,%.2f,%d,%d,%d,%d,%d,1\n\n",
		style.FontFamily,
		fontSize,
		assColor(style.TextColor, 1),
		assColor(style.TextColor, 1),
		assColor(style.OutlineColor, 1),
		assColor(style.BackgroundColor, style.BackgroundOpacity),
		bold,
		borderStyle,
		style.OutlineWidth,
		shadow,
		alignment,
		marginH,
		marginH,
		marginV,
	)
	content.WriteString("[Events]\n")
	content.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, cue := range cues {
		text := wrapASSSubtitle(strings.TrimSpace(cue.Text), style.MaxLines, style.MaxCharsPerLine)
		if text == "" {
			continue
		}
		fmt.Fprintf(&content, "Dialogue: 0,%s,%s,Default,,0,0,0,,{\\an%d\\pos(%d,%d)}%s\n", formatASSTimestamp(cue.StartMs), formatASSTimestamp(cue.EndMs), alignment, positionX, positionY, text)
	}
	return content.String()
}

func normalizeSubtitleStyle(style SubtitleStyle) SubtitleStyle {
	if strings.TrimSpace(style.FontFamily) == "" {
		style = SubtitleStyle{
			FontFamily: "Noto Sans CJK SC", FontWeight: 700, TextColor: "#FFFFFF",
			BackgroundColor: "#000000", BackgroundOpacity: 0.3, OutlineColor: "#000000",
			OutlineWidth: 0, MaxLines: 2, VerticalPosition: "bottom", TextAlign: "center",
			VerticalOffsetRatio: 0.14, VerticalPositionRatio: 0.82,
			MaxWidthRatio: 0.84, FontSizeRatio: 0.054, MaxCharsPerLine: 16,
		}
	}
	if strings.TrimSpace(style.FontFamily) == "" || strings.ContainsAny(style.FontFamily, ",\r\n") {
		style.FontFamily = "Noto Sans CJK SC"
	}
	if style.FontWeight < 100 || style.FontWeight > 900 {
		style.FontWeight = 700
	}
	if !validASSHexColor(style.TextColor) {
		style.TextColor = "#FFFFFF"
	}
	if !validASSHexColor(style.BackgroundColor) {
		style.BackgroundColor = "#000000"
	}
	if !validASSHexColor(style.OutlineColor) {
		style.OutlineColor = "#000000"
	}
	if style.BackgroundOpacity < 0 || style.BackgroundOpacity > 1 {
		style.BackgroundOpacity = 0.3
	}
	if style.OutlineWidth < 0 || style.OutlineWidth > 8 {
		style.OutlineWidth = 0
	}
	if style.MaxLines < 1 || style.MaxLines > 3 {
		style.MaxLines = 2
	}
	if style.VerticalPosition != "top" && style.VerticalPosition != "center" && style.VerticalPosition != "bottom" {
		style.VerticalPosition = "bottom"
	}
	if style.TextAlign != "left" && style.TextAlign != "center" && style.TextAlign != "right" {
		style.TextAlign = "center"
	}
	if style.VerticalOffsetRatio < 0 || style.VerticalOffsetRatio > 0.4 {
		style.VerticalOffsetRatio = 0.14
	}
	if style.VerticalPositionRatio < 0.05 || style.VerticalPositionRatio > 0.95 {
		style.VerticalPositionRatio = legacySubtitlePositionRatio(style)
	}
	if style.MaxWidthRatio < 0.3 || style.MaxWidthRatio > 0.96 {
		style.MaxWidthRatio = 0.84
	}
	if style.FontSizeRatio < 0.02 || style.FontSizeRatio > 0.12 {
		style.FontSizeRatio = 0.054
	}
	if style.MaxCharsPerLine < 4 || style.MaxCharsPerLine > 40 {
		style.MaxCharsPerLine = 16
	}
	return style
}

func legacySubtitlePositionRatio(style SubtitleStyle) float64 {
	position := 0.5
	if style.VerticalPosition == "top" {
		position = style.VerticalOffsetRatio + 0.05
	} else if style.VerticalPosition == "bottom" {
		position = 1 - style.VerticalOffsetRatio - 0.05
	}
	return math.Max(0.05, math.Min(0.95, position))
}

func subtitleASSPositionX(horizontal string, width int, marginH int) int {
	if horizontal == "left" {
		return marginH
	}
	if horizontal == "right" {
		return width - marginH
	}
	return width / 2
}

func subtitleASSAlignment(vertical string, horizontal string) int {
	base := 1
	if vertical == "center" {
		base = 4
	} else if vertical == "top" {
		base = 7
	}
	if horizontal == "center" {
		return base + 1
	}
	if horizontal == "right" {
		return base + 2
	}
	return base
}

func assColor(value string, opacity float64) string {
	if !validASSHexColor(value) {
		value = "#000000"
	}
	red, _ := strconv.ParseUint(value[1:3], 16, 8)
	green, _ := strconv.ParseUint(value[3:5], 16, 8)
	blue, _ := strconv.ParseUint(value[5:7], 16, 8)
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}
	alpha := int(math.Round((1 - opacity) * 255))
	return fmt.Sprintf("&H%02X%02X%02X%02X", alpha, blue, green, red)
}

func validASSHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}

func wrapASSSubtitle(text string, maxLines int, maxCharsPerLine int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	lineCount := (len(runes) + maxCharsPerLine - 1) / maxCharsPerLine
	if lineCount < 1 {
		lineCount = 1
	}
	if lineCount > maxLines {
		lineCount = maxLines
	}
	charsPerLine := (len(runes) + lineCount - 1) / lineCount
	lines := make([]string, 0, lineCount)
	for start := 0; start < len(runes); start += charsPerLine {
		end := start + charsPerLine
		if end > len(runes) {
			end = len(runes)
		}
		line := strings.TrimSpace(string(runes[start:end]))
		if line != "" {
			lines = append(lines, escapeASSText(line))
		}
	}
	return strings.Join(lines, `\N`)
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
