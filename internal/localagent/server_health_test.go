package localagent

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleHealthIdentifiesCompatibleReadyAgent(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg-test-binary")
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-test-binary")
	if err := os.WriteFile(ffmpegPath, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffprobePath, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", ffmpegPath)
	t.Setenv("FFPROBE_PATH", ffprobePath)

	server := New(Options{WorkspaceRoot: t.TempDir(), Processor: stubProcessor{}, AppVersion: "1.2.3"})
	recorder := httptest.NewRecorder()
	server.handleHealth(recorder, httptest.NewRequest("GET", "/healthz", nil))

	if recorder.Code != 200 {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var health struct {
		Status          string `json:"status"`
		App             string `json:"app"`
		Version         string `json:"version"`
		ProtocolVersion int    `json:"protocol_version"`
		FFmpegReady     bool   `json:"ffmpeg_ready"`
		FFprobeReady    bool   `json:"ffprobe_ready"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.Status != "ok" || health.App != AppIdentifier || health.Version != "1.2.3" ||
		health.ProtocolVersion != ProtocolVersion || !health.FFmpegReady || !health.FFprobeReady {
		t.Fatalf("unexpected health response: %+v", health)
	}
}

func TestRunContextStopsWhenAlreadyCancelled(t *testing.T) {
	server := New(Options{Addr: "127.0.0.1:0", WorkspaceRoot: t.TempDir(), Processor: stubProcessor{}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := server.RunContext(ctx); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}
