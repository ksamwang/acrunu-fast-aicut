package ffmpeg

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTimelineArgsBuildsContinuousVideoAndNarration(t *testing.T) {
	t.Parallel()
	input := RenderInput{
		Clips: []RenderClip{
			{InputPath: "first.mp4", SourceInMs: 100, SourceOutMs: 1100},
			{InputPath: "second.mp4", SourceInMs: 200, SourceOutMs: 1700},
		},
		NarrationPath: "voice.wav",
		Subtitles:     []SubtitleCue{{StartMs: 0, EndMs: 1000, Text: "第一句"}},
		OutputPath:    "output.mp4",
		WorkDir:       "work",
		DurationMs:    2500,
		Width:         1080,
		Height:        1920,
		FPS:           30,
	}
	args, err := renderTimelineArgs(input)
	if err != nil {
		t.Fatalf("renderTimelineArgs() error = %v", err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"concat=n=2:v=1:a=0[video]",
		"subtitles=filename=subtitles.ass",
		"[2:a:0]aresample=48000",
		"-map [video_with_subtitles]",
		"-map [narration]",
		"-t 2.500",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("render args missing %q:\n%s", expected, joined)
		}
	}
}

func TestBuildASSUsesCleanTextAndTransparentBackground(t *testing.T) {
	t.Parallel()
	content := buildASS([]SubtitleCue{{StartMs: 0, EndMs: 1230, Text: "骑行更安全"}}, 1080, 1920, SubtitleStyle{})
	if !strings.Contains(content, "&HB3000000") || !strings.Contains(content, "Dialogue: 0,0:00:00.00,0:00:01.23") || !strings.Contains(content, "骑行更安全") {
		t.Fatalf("unexpected ASS content:\n%s", content)
	}
}

func TestBuildASSKeepsShadowVisibleWithoutSubtitleBackground(t *testing.T) {
	t.Parallel()
	content := buildASS([]SubtitleCue{{StartMs: 0, EndMs: 1000, Text: "骑行更安全"}}, 1080, 1920, SubtitleStyle{
		FontFamily: "Noto Sans CJK SC", FontWeight: 700, TextColor: "#FFFFFF",
		BackgroundColor: "#000000", BackgroundOpacity: 0, OutlineColor: "#000000",
		OutlineWidth: 0, Shadow: true, MaxLines: 2, VerticalPosition: "center", TextAlign: "center",
		VerticalPositionRatio: 0.8, MaxWidthRatio: 0.84, FontSizeRatio: 0.06, MaxCharsPerLine: 16,
	})
	if !strings.Contains(content, "&H47000000") || !strings.Contains(content, ",1,0.00,2,5,") {
		t.Fatalf("shadow style is not visible without a background:\n%s", content)
	}
}

func TestBuildASSUsesConfiguredAlignmentColorAndWrapping(t *testing.T) {
	t.Parallel()
	content := buildASS([]SubtitleCue{{StartMs: 0, EndMs: 2000, Text: "一二三四五六七八九十甲乙丙丁戊己庚辛壬癸"}}, 1080, 1440, SubtitleStyle{
		FontFamily: "Noto Sans CJK SC", FontWeight: 400, TextColor: "#FFCC00",
		BackgroundColor: "#102030", BackgroundOpacity: 0.5, OutlineColor: "#FFFFFF",
		OutlineWidth: 1.5, MaxLines: 2, VerticalPosition: "top", TextAlign: "left",
		VerticalOffsetRatio: 0.1, VerticalPositionRatio: 0.2,
		MaxWidthRatio: 0.8, FontSizeRatio: 0.05, MaxCharsPerLine: 10,
	})
	for _, expected := range []string{
		"Noto Sans CJK SC,54,&H0000CCFF",
		"&H80302010",
		",3,1.50,0,4,108,108,144,1",
		`{\an4\pos(108,288)}`,
		`一二三四五六七八九十\N甲乙丙丁戊己庚辛壬癸`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("configured ASS missing %q:\n%s", expected, content)
		}
	}
}

func TestRenderTimelineIntegration(t *testing.T) {
	ffmpegBinary, err := exec.LookPath(ffmpegPath())
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	root := t.TempDir()
	first := filepath.Join(root, "first.mp4")
	second := filepath.Join(root, "second.mp4")
	narration := filepath.Join(root, "voice.wav")
	for _, fixture := range []struct {
		path string
		args []string
	}{
		{first, []string{"-y", "-f", "lavfi", "-i", "color=c=red:s=320x568:r=30:d=0.7", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p"}},
		{second, []string{"-y", "-f", "lavfi", "-i", "color=c=blue:s=320x568:r=30:d=0.7", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p"}},
		{narration, []string{"-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1.2", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le"}},
	} {
		args := append(fixture.args, fixture.path)
		if output, err := exec.Command(ffmpegBinary, args...).CombinedOutput(); err != nil {
			t.Fatalf("create fixture %s: %v: %s", fixture.path, err, output)
		}
	}
	outputPath := filepath.Join(root, "output.mp4")
	result, err := RenderTimeline(context.Background(), RenderInput{
		Clips: []RenderClip{
			{InputPath: first, SourceInMs: 0, SourceOutMs: 600},
			{InputPath: second, SourceInMs: 0, SourceOutMs: 600},
		},
		NarrationPath: narration,
		Subtitles:     []SubtitleCue{{StartMs: 0, EndMs: 600, Text: "骑行更安全"}},
		OutputPath:    outputPath,
		WorkDir:       filepath.Join(root, "work"),
		DurationMs:    1200,
		Width:         360,
		Height:        640,
		FPS:           30,
	})
	if err != nil {
		t.Fatalf("RenderTimeline() error = %v", err)
	}
	if result.Width != 360 || result.Height != 640 || !result.HasAudio || result.DurationMs < 1100 || result.DurationMs > 1300 {
		t.Fatalf("unexpected rendered media %#v", result)
	}
}
