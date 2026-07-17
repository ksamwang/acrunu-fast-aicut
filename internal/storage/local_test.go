package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStoreDeleteStaysWithinRoot(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStore(root)
	if _, err := store.Save("renders/run/final.mp4", strings.NewReader("video")); err != nil {
		t.Fatalf("save file: %v", err)
	}
	if err := store.Delete("renders/run/final.mp4"); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "renders", "run", "final.mp4")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted file, got %v", err)
	}
	if err := store.Delete(filepath.Join("..", "outside.mp4")); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
