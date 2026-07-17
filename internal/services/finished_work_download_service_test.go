package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinishedWorkDownloadCreatesUniqueOneTimeBatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID: "voiceover-task-1", ProductID: "product-1", ProductName: "束裤带", Title: `夜骑:安全/展示`, Status: "completed",
	}}
	runs := NewGenerationRunService(loader)
	run, err := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1", CreatedByUserID: "user-1", CreatedByName: "王璐"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.LinkTask(context.Background(), run.ID, "voiceover-task-1", generationRunTaskStageVoiceover); err != nil {
		t.Fatal(err)
	}
	if err := runs.AttachVoiceoverArtifacts(context.Background(), run.ID, "voiceover-task-1", "script-1", "voiceover-1"); err != nil {
		t.Fatal(err)
	}
	storageKey := filepath.ToSlash(filepath.Join("renders", run.ID, "final.mp4"))
	fullPath := filepath.Join(root, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runs.MarkRenderCompleted(context.Background(), run.ID, GenerationRenderOutput{StorageKey: storageKey, MimeType: "video/mp4", DurationMs: 1000, Width: 1080, Height: 1920, FileSizeBytes: 5, Renderer: "ffmpeg", RenderVersion: "test"}); err != nil {
		t.Fatal(err)
	}

	service := NewFinishedWorkDownloadService(runs, root)
	batch, err := service.Create(context.Background(), []string{run.ID, run.ID})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if batch.FileCount != 1 || batch.Token == "" || !strings.HasSuffix(batch.FileName, ".zip") {
		t.Fatalf("unexpected batch %#v", batch)
	}
	_, files, err := service.Consume(batch.Token)
	if err != nil || len(files) != 1 {
		t.Fatalf("Consume() = %#v, %v", files, err)
	}
	if strings.ContainsAny(files[0].FileName, `:"/\|?*`) || !strings.HasSuffix(files[0].FileName, ".mp4") {
		t.Fatalf("unsafe download file name %q", files[0].FileName)
	}
	if _, _, err := service.Consume(batch.Token); !errors.Is(err, ErrFinishedWorkDownloadToken) {
		t.Fatalf("second Consume() error = %v", err)
	}
}

func TestFinishedWorkDownloadRejectsIncompleteRun(t *testing.T) {
	t.Parallel()
	runs := NewGenerationRunService(nil)
	run, err := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewFinishedWorkDownloadService(runs, t.TempDir())
	if _, err := service.Create(context.Background(), []string{run.ID}); !errors.Is(err, ErrFinishedWorkDownloadUnavailable) {
		t.Fatalf("Create() error = %v", err)
	}
}
