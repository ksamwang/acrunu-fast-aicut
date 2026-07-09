package localagent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type stubProcessor struct{}

func (stubProcessor) Cut(_ context.Context, sourcePath string, outputPath string, _ int, _ int) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

func (stubProcessor) Probe(_ context.Context, _ string) (ffmpeg.ProbeResult, error) {
	return ffmpeg.ProbeResult{
		DurationMs: 6000,
		Width:      1080,
		Height:     1920,
		FPS:        30,
		Codec:      "h264",
		HasAudio:   true,
		AudioCodec: "aac",
	}, nil
}

func (stubProcessor) ExtractFrames(_ context.Context, _ string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	frames := make([]ffmpeg.ExtractedFrame, 0, len(timestampsMs))
	for index, ts := range timestampsMs {
		outputPath := filepath.Join(outputDir, "frame-"+string(rune('0'+index))+".jpg")
		if err := os.WriteFile(outputPath, []byte("frame"), 0644); err != nil {
			return nil, err
		}
		frames = append(frames, ffmpeg.ExtractedFrame{
			FrameIndex:  index,
			TimestampMs: ts,
			OutputPath:  outputPath,
		})
	}
	return frames, nil
}

func (stubProcessor) Analyze(_ context.Context, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	return modelgateway.AnalyzeAssetResult{
		UsabilityStatus:  "usable",
		SceneDescription: "demo scene",
		ShotSize:         "medium_close_up",
		CameraMovement:   "static",
		Subjects:         []string{"person"},
		SceneTags:        []string{"indoor"},
		ModelResult: map[string]any{
			"frame_count": len(input.FrameSnapshots),
		},
	}, nil
}

func TestWorkspaceImportSavePrepareAndClear(t *testing.T) {
	root := t.TempDir()
	workspace, err := NewWorkspace(root, stubProcessor{})
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}

	header, cleanup := newMultipartHeader(t, "sample.mp4", []byte("video"))
	defer cleanup()

	imported, err := workspace.ImportFiles(context.Background(), []*multipart.FileHeader{header})
	if err != nil {
		t.Fatalf("ImportFiles() error = %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("expected 1 imported item, got %d", len(imported))
	}
	item := imported[0]
	if item.Status != workspaceStatusPending {
		t.Fatalf("expected pending status, got %s", item.Status)
	}

	saved, err := workspace.SaveItem(item.ID, WorkspaceSaveInput{
		AssetName:   "测试素材",
		SourceType:  "talking_head",
		SourceInMs:  0,
		SourceOutMs: 5000,
		Transcript:  "大家好",
	})
	if err != nil {
		t.Fatalf("SaveItem() error = %v", err)
	}
	if saved.Status != workspaceStatusSaved {
		t.Fatalf("expected saved status, got %s", saved.Status)
	}

	prepared, err := workspace.PrepareItem(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("PrepareItem() error = %v", err)
	}
	if prepared.Status != workspaceStatusReadyToSubmit {
		t.Fatalf("expected ready_to_submit status, got %s", prepared.Status)
	}
	if len(prepared.FrameSnapshots) != 3 {
		t.Fatalf("expected 3 frame snapshots, got %d", len(prepared.FrameSnapshots))
	}
	if prepared.Analysis == nil || prepared.Analysis.SceneDescription == "" {
		t.Fatalf("expected analysis result to be populated")
	}

	if err := workspace.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if len(workspace.ListItems()) != 0 {
		t.Fatalf("expected workspace to be empty after clear")
	}
}

func TestWorkspaceClearRemovesSubmittedLocalRecords(t *testing.T) {
	root := t.TempDir()
	workspace, err := NewWorkspace(root, stubProcessor{})
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}

	header, cleanup := newMultipartHeader(t, "sample.mp4", []byte("video"))
	defer cleanup()

	imported, err := workspace.ImportFiles(context.Background(), []*multipart.FileHeader{header})
	if err != nil {
		t.Fatalf("ImportFiles() error = %v", err)
	}
	item := imported[0]

	if _, err := workspace.SaveItem(item.ID, WorkspaceSaveInput{
		AssetName:   "test asset",
		SourceType:  "talking_head",
		SourceInMs:  0,
		SourceOutMs: 5000,
		Transcript:  "[00:00:00:00]-[00:00:02:00] hello",
	}); err != nil {
		t.Fatalf("SaveItem() error = %v", err)
	}
	if _, err := workspace.PrepareItem(context.Background(), item.ID); err != nil {
		t.Fatalf("PrepareItem() error = %v", err)
	}

	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if token := r.Header.Get("X-Upload-Token"); token != "upload-token" {
			t.Fatalf("expected upload token header, got %q", token)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.FormValue("submission_mode"); got != "preprocessed" {
			t.Fatalf("expected preprocessed submission, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"asset": map[string]any{
					"id": "asset-submitted-1",
				},
			},
		})
	}))
	defer uploadServer.Close()

	submitted, err := workspace.SubmitItem(context.Background(), item.ID, WorkspaceSubmitInput{
		ProductID:   "product-1",
		UploadURL:   uploadServer.URL,
		UploadToken: "upload-token",
	})
	if err != nil {
		t.Fatalf("SubmitItem() error = %v", err)
	}
	if submitted.Status != workspaceStatusSubmitted {
		t.Fatalf("expected submitted status, got %s", submitted.Status)
	}

	if err := workspace.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if len(workspace.ListItems()) != 0 {
		t.Fatalf("expected submitted local records to be removed by clear")
	}
}

func newMultipartHeader(t *testing.T, fileName string, contents []byte) (*multipart.FileHeader, func()) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(contents); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), writer.Boundary())
	form, err := reader.ReadForm(int64(len(body.Bytes())))
	if err != nil {
		t.Fatalf("ReadForm() error = %v", err)
	}

	headers := form.File["files"]
	if len(headers) != 1 {
		t.Fatalf("expected 1 multipart file header, got %d", len(headers))
	}

	return headers[0], func() {
		for _, files := range form.File {
			for _, fileHeader := range files {
				file, err := fileHeader.Open()
				if err == nil {
					_, _ = io.Copy(io.Discard, file)
					_ = file.Close()
				}
			}
		}
	}
}
