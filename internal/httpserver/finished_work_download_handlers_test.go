package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type finishedDownloadWorkLoader struct{ work services.VoiceoverWork }

func (loader finishedDownloadWorkLoader) GetVoiceoverWork(context.Context, string) (services.VoiceoverWork, error) {
	return loader.work, nil
}

func TestFinishedWorkDownloadHandlersStreamZip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	root := t.TempDir()
	runs := services.NewGenerationRunService(finishedDownloadWorkLoader{work: services.VoiceoverWork{
		ID: "voiceover-task-1", ProductID: "product-1", ProductName: "束裤带", Title: "夜骑安全", Status: "completed",
	}})
	run, err := runs.Create(ctx, services.CreateGenerationRunInput{ProductID: "product-1", CreatedByUserID: "user-1", CreatedByName: "王璐"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.LinkTask(ctx, run.ID, "voiceover-task-1", "voiceover"); err != nil {
		t.Fatal(err)
	}
	if err := runs.AttachVoiceoverArtifacts(ctx, run.ID, "voiceover-task-1", "script-1", "voiceover-1"); err != nil {
		t.Fatal(err)
	}
	storageKey := filepath.ToSlash(filepath.Join("renders", run.ID, "final.mp4"))
	fullPath := filepath.Join(root, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("video-payload"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runs.MarkRenderCompleted(ctx, run.ID, services.GenerationRenderOutput{StorageKey: storageKey, MimeType: "video/mp4", DurationMs: 1000, Width: 1080, Height: 1920, FileSizeBytes: 13, Renderer: "ffmpeg", RenderVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	server := New(Options{Config: config.Config{StorageRoot: root, QueueBackend: "file"}, GenerationRunService: runs})

	payload, _ := json.Marshal(map[string]any{"work_ids": []string{run.ID}})
	createRequest := httptest.NewRequest(http.MethodPost, "/api/workbench/works/download-batches", bytes.NewReader(payload))
	createRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create download: %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var response struct {
		Data struct {
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, response.Data.DownloadURL, nil)
	downloadRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusOK || downloadRecorder.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download zip: %d headers=%v body=%s", downloadRecorder.Code, downloadRecorder.Header(), downloadRecorder.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(downloadRecorder.Body.Bytes()), int64(downloadRecorder.Body.Len()))
	if err != nil || len(reader.File) != 1 {
		t.Fatalf("read zip: files=%d err=%v", len(reader.File), err)
	}
	entry, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(entry)
	_ = entry.Close()
	if err != nil || string(content) != "video-payload" {
		t.Fatalf("unexpected zip payload %q err=%v", content, err)
	}
	secondRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(secondRecorder, httptest.NewRequest(http.MethodGet, response.Data.DownloadURL, nil))
	if secondRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected one-time URL, got %d", secondRecorder.Code)
	}
}
