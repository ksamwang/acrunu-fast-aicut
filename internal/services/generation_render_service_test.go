package services

import (
	"path/filepath"
	"testing"
)

func TestSafeStoragePathRejectsTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := safeStoragePath(root, "../outside.mp4"); err == nil {
		t.Fatal("expected storage traversal to be rejected")
	}
	path, err := safeStoragePath(root, "renders/run/final.mp4")
	if err != nil {
		t.Fatalf("safeStoragePath() error = %v", err)
	}
	want := filepath.Join(root, "renders", "run", "final.mp4")
	if path != want {
		t.Fatalf("safeStoragePath() = %q, want %q", path, want)
	}
}

func TestRenderSnapshotIntUsesBoundedValues(t *testing.T) {
	t.Parallel()
	if got := renderSnapshotInt(map[string]any{"output_width": float64(720)}, "output_width", 1080, 64, 3840); got != 720 {
		t.Fatalf("renderSnapshotInt() = %d, want 720", got)
	}
	if got := renderSnapshotInt(map[string]any{"output_width": float64(9000)}, "output_width", 1080, 64, 3840); got != 1080 {
		t.Fatalf("renderSnapshotInt() = %d, want fallback 1080", got)
	}
}

func TestRenderSnapshotSubtitleStyleDecodesResolvedConfig(t *testing.T) {
	t.Parallel()
	style := renderSnapshotSubtitleStyle(map[string]any{
		"subtitle_style": map[string]any{
			"font_family": "Noto Sans CJK SC", "font_weight": float64(700),
			"text_color": "#FFFFFF", "background_color": "#000000", "background_opacity": 0.3,
			"outline_color": "#000000", "outline_width": 0.0, "shadow": false, "max_lines": float64(2),
			"vertical_position": "bottom", "text_align": "center", "vertical_offset_ratio": 0.1,
			"vertical_position_ratio": 0.72,
			"max_width_ratio":         0.88, "font_size_ratio": 0.052, "max_chars_per_line": float64(18),
		},
	})
	if style.FontFamily != "Noto Sans CJK SC" || style.VerticalPositionRatio != 0.72 || style.MaxCharsPerLine != 18 {
		t.Fatalf("unexpected decoded subtitle style %#v", style)
	}
}

func TestRenderSnapshotBGMDecodesResolvedConfig(t *testing.T) {
	t.Parallel()
	config := renderSnapshotBGM(map[string]any{"bgm": map[string]any{
		"track_id": "track-1", "name": "轻快骑行", "storage_key": "bgm/track-1/source.mp3",
		"gain_db": -12.0, "fade_in_ms": 300.0, "fade_out_ms": 500.0,
	}})
	if config == nil || config.TrackID != "track-1" || config.GainDB != -12 || config.FadeOutMs != 500 {
		t.Fatalf("unexpected BGM config %#v", config)
	}
	if config := renderSnapshotBGM(map[string]any{"bgm": map[string]any{"name": "invalid"}}); config != nil {
		t.Fatalf("invalid BGM config = %#v, want nil", config)
	}
}
