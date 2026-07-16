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
