package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
)

func TestLocalAgentReleaseMetadataAndDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releaseRoot := t.TempDir()
	releaseDir := filepath.Join(releaseRoot, "local-agent", "windows-x64")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := localAgentReleaseManifest{
		Version:         "0.1.0",
		Platform:        "windows-x64",
		ProtocolVersion: 1,
		SHA256:          strings.Repeat("a", 64),
		Filename:        "ACRUNU-Fast-Cut-Local-Agent-Setup-x64.exe",
	}
	manifestBytes, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(releaseDir, "release.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, manifest.Filename), []byte("installer"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(Options{Config: config.Config{StorageRoot: t.TempDir(), ClientReleaseRoot: releaseRoot, QueueBackend: "file"}})
	metadataRequest := httptest.NewRequest(http.MethodGet, "/api/client-releases/local-agent/latest", nil)
	metadataRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(metadataRecorder, metadataRequest)
	if metadataRecorder.Code != http.StatusOK {
		t.Fatalf("expected metadata status 200, got %d: %s", metadataRecorder.Code, metadataRecorder.Body.String())
	}
	if !strings.Contains(metadataRecorder.Body.String(), `"download_url":"/api/client-releases/local-agent/download"`) {
		t.Fatalf("unexpected metadata response: %s", metadataRecorder.Body.String())
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/api/client-releases/local-agent/download", nil)
	downloadRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusOK || downloadRecorder.Body.String() != "installer" {
		t.Fatalf("unexpected download response: status=%d body=%q", downloadRecorder.Code, downloadRecorder.Body.String())
	}
	if disposition := downloadRecorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, manifest.Filename) {
		t.Fatalf("expected installer filename, got %q", disposition)
	}
}

func TestLocalAgentReleaseRejectsUnsafeManifestFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releaseRoot := t.TempDir()
	releaseDir := filepath.Join(releaseRoot, "local-agent", "windows-x64")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"0.1.0","platform":"windows-x64","protocol_version":1,"filename":"../outside.exe"}`
	if err := os.WriteFile(filepath.Join(releaseDir, "release.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(Options{Config: config.Config{StorageRoot: t.TempDir(), ClientReleaseRoot: releaseRoot, QueueBackend: "file"}})
	request := httptest.NewRequest(http.MethodGet, "/api/client-releases/local-agent/latest", nil)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}
