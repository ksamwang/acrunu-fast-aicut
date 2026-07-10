package localagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/httpserver"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type stubProcessor struct{}

func (stubProcessor) Cut(_ context.Context, sourcePath string, outputPath string, _ int, _ int, _ ffmpeg.CutOptions) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

func (stubProcessor) InterpretFPS(_ context.Context, sourcePath string, outputPath string, _ float64, _ float64, _ int) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, append([]byte("interpreted:"), data...), 0644)
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
		SceneDescription:  "demo scene",
		ShotSize:          "medium_close_up",
		CameraMovement:    "static",
		VisualTags:        []string{"person", "indoor"},
		QualityTags:       []string{"clear"},
		VisibleProduct:    true,
		ProductPosition:   "center",
		SceneContext:      "indoor",
		ActionDescription: "person shows product",
		PeoplePresence:    true,
		FaceVisible:       true,
		LightingCondition: "normal",
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

	saved, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
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
	expectedTimestamps := []int{0, 3000, 6000}
	for index, snapshot := range prepared.FrameSnapshots {
		if snapshot.TimestampMs != expectedTimestamps[index] {
			t.Fatalf("expected frame %d timestamp %d, got %d", index, expectedTimestamps[index], snapshot.TimestampMs)
		}
	}
	if prepared.Analysis != nil {
		t.Fatalf("expected prepare not to run vlm analysis")
	}

	if err := workspace.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if len(workspace.ListItems()) != 0 {
		t.Fatalf("expected workspace to be empty after clear")
	}
}

func TestWorkspacePreviewFramesUsesCurrentSourceRange(t *testing.T) {
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

	previewed, err := workspace.PreviewFrames(context.Background(), item.ID, WorkspacePreviewFramesInput{
		SourceInMs:  1000,
		SourceOutMs: 5000,
	})
	if err != nil {
		t.Fatalf("PreviewFrames() error = %v", err)
	}
	if len(previewed.PreviewFrames) != 3 {
		t.Fatalf("expected 3 preview frames, got %d", len(previewed.PreviewFrames))
	}
	expectedTimestamps := []int{1000, 3000, 5000}
	for index, snapshot := range previewed.PreviewFrames {
		if snapshot.TimestampMs != expectedTimestamps[index] {
			t.Fatalf("expected preview frame %d timestamp %d, got %d", index, expectedTimestamps[index], snapshot.TimestampMs)
		}
		if snapshot.ImagePath == "" {
			t.Fatalf("expected preview frame %d image path", index)
		}
	}
	if len(previewed.FrameSnapshots) != 0 {
		t.Fatalf("expected preview frames not to populate clean shot frame snapshots")
	}
}

func TestWorkspaceSaveInterpretFPS(t *testing.T) {
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

	saved, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
		AssetName:    "slow shot",
		SourceType:   "visual_only",
		SourceInMs:   0,
		SourceOutMs:  5000,
		InterpretFPS: true,
		PlaybackFPS:  25,
	})
	if err != nil {
		t.Fatalf("SaveItem() error = %v", err)
	}
	if !saved.InterpretFPS {
		t.Fatalf("expected interpret fps enabled")
	}
	if saved.PlaybackFPS != 25 {
		t.Fatalf("expected playback fps 25, got %v", saved.PlaybackFPS)
	}
	if saved.SpeedRatio != 25.0/30.0 {
		t.Fatalf("expected speed ratio %v, got %v", 25.0/30.0, saved.SpeedRatio)
	}
	if saved.OriginalSourcePath == "" || saved.SourcePath == saved.OriginalSourcePath {
		t.Fatalf("expected interpret fps to switch to a working source, got source=%q original=%q", saved.SourcePath, saved.OriginalSourcePath)
	}

	prepared, err := workspace.PrepareItem(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("PrepareItem() after interpret fps error = %v", err)
	}
	if prepared.Status != workspaceStatusReadyToSubmit {
		t.Fatalf("expected ready_to_submit status, got %s", prepared.Status)
	}
}

func TestWorkspaceRejectsInvalidInterpretFPS(t *testing.T) {
	tests := []struct {
		name  string
		input WorkspaceSaveInput
	}{
		{
			name: "talking head is not supported",
			input: WorkspaceSaveInput{
				SourceType:   "talking_head",
				SourceInMs:   0,
				SourceOutMs:  5000,
				InterpretFPS: true,
				PlaybackFPS:  25,
				Transcript:   "[00:00:00:00]-[00:00:02:00] hello",
			},
		},
		{
			name: "playback fps below 25 is not supported",
			input: WorkspaceSaveInput{
				SourceType:   "visual_only",
				SourceInMs:   0,
				SourceOutMs:  5000,
				InterpretFPS: true,
				PlaybackFPS:  24,
			},
		},
		{
			name: "playback fps must be lower than source fps",
			input: WorkspaceSaveInput{
				SourceType:   "visual_only",
				SourceInMs:   0,
				SourceOutMs:  5000,
				InterpretFPS: true,
				PlaybackFPS:  30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if _, err := workspace.SaveItem(context.Background(), imported[0].ID, tt.input); err == nil {
				t.Fatalf("expected SaveItem() error")
			}
		})
	}
}

func TestWorkspaceStartVLMLabelRunsAsync(t *testing.T) {
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

	vlmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/preprocess/vlm-label" {
			t.Fatalf("unexpected vlm path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("expected bearer token, got %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if r.FormValue("product_name") != "车载氛围灯" {
			t.Fatalf("expected product_name to be forwarded, got %q", r.FormValue("product_name"))
		}
		for index := 0; index < 3; index++ {
			if _, _, err := r.FormFile(fmt.Sprintf("frame_%d", index)); err != nil {
				t.Fatalf("expected frame_%d upload: %v", index, err)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"analysis": map[string]any{
					"scene_description":  "server analyzed scene",
					"shot_size":          "medium_close_up",
					"camera_movement":    "static",
					"visual_tags":        []string{"server", "frame"},
					"quality_tags":       []string{"clear"},
					"visible_product":    true,
					"product_position":   "center",
					"scene_context":      "indoor demo",
					"action_description": "product is shown",
					"people_presence":    false,
					"face_visible":       false,
					"lighting_condition": "normal indoor lighting",
					"model_result":       map[string]any{"provider": "server-test"},
				},
			},
		})
	}))
	defer vlmServer.Close()

	queued, err := workspace.StartVLMLabel(item.ID, WorkspaceVLMLabelInput{
		SourceType:    "visual_only",
		ProductName:   "车载氛围灯",
		SourceInMs:    1000,
		SourceOutMs:   5000,
		ServerBaseURL: vlmServer.URL,
		AuthToken:     "test-token",
	})
	if err != nil {
		t.Fatalf("StartVLMLabel() error = %v", err)
	}
	if queued.VLMStatus != vlmStatusQueued {
		t.Fatalf("expected queued status, got %s", queued.VLMStatus)
	}

	var labeled WorkspaceItem
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var ok bool
		labeled, ok = workspace.GetItem(item.ID)
		if !ok {
			t.Fatalf("workspace item disappeared")
		}
		if labeled.VLMStatus == vlmStatusReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if labeled.VLMStatus != vlmStatusReady {
		t.Fatalf("expected ready status, got %s error=%s", labeled.VLMStatus, labeled.VLMError)
	}
	if labeled.Analysis == nil || labeled.Analysis.SceneDescription != "server analyzed scene" {
		t.Fatalf("expected analysis populated")
	}
	if len(labeled.PreviewFrames) != 3 {
		t.Fatalf("expected preview frames from vlm label, got %d", len(labeled.PreviewFrames))
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

	if _, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
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

func TestWorkspaceDuplicateItemSupportsMultipleCleanShotsFromSingleSource(t *testing.T) {
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
	original := imported[0]

	original, err = workspace.SaveItem(context.Background(), original.ID, WorkspaceSaveInput{
		AssetName:   "shot-1",
		SourceType:  "visual_only",
		SourceInMs:  0,
		SourceOutMs: 2500,
	})
	if err != nil {
		t.Fatalf("SaveItem() error = %v", err)
	}

	duplicate, err := workspace.DuplicateItem(original.ID)
	if err != nil {
		t.Fatalf("DuplicateItem() error = %v", err)
	}
	if duplicate.ID == original.ID {
		t.Fatalf("expected duplicate to have a new id")
	}
	if duplicate.OriginalFileName != original.OriginalFileName {
		t.Fatalf("expected duplicate to keep original file name, got %+v", duplicate)
	}
	if duplicate.SourcePath == original.SourcePath {
		t.Fatalf("expected duplicate to have its own source copy")
	}
	if duplicate.Status != workspaceStatusSaved {
		t.Fatalf("expected duplicate status saved, got %s", duplicate.Status)
	}

	duplicate, err = workspace.SaveItem(context.Background(), duplicate.ID, WorkspaceSaveInput{
		AssetName:   "shot-2",
		SourceType:  "visual_only",
		SourceInMs:  2500,
		SourceOutMs: 5000,
	})
	if err != nil {
		t.Fatalf("SaveItem() duplicate error = %v", err)
	}

	preparedOriginal, err := workspace.PrepareItem(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("PrepareItem(original) error = %v", err)
	}
	preparedDuplicate, err := workspace.PrepareItem(context.Background(), duplicate.ID)
	if err != nil {
		t.Fatalf("PrepareItem(duplicate) error = %v", err)
	}

	if preparedOriginal.CleanShotPath == "" || preparedDuplicate.CleanShotPath == "" {
		t.Fatalf("expected both clean shots to be generated")
	}
	if preparedOriginal.CleanShotPath == preparedDuplicate.CleanShotPath {
		t.Fatalf("expected each item to have its own clean shot output path")
	}
	if len(workspace.ListItems()) != 2 {
		t.Fatalf("expected 2 workspace items, got %d", len(workspace.ListItems()))
	}
}

func TestWorkspaceDoesNotCreateServerAssetUntilSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	workspace, err := NewWorkspace(root, stubProcessor{})
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	apiServer := httpserver.New(httpserver.Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})
	httpSrv := httptest.NewServer(apiServer.Engine())
	defer httpSrv.Close()

	header, cleanup := newMultipartHeader(t, "sample.mp4", []byte("video"))
	defer cleanup()

	imported, err := workspace.ImportFiles(context.Background(), []*multipart.FileHeader{header})
	if err != nil {
		t.Fatalf("ImportFiles() error = %v", err)
	}
	item := imported[0]

	if len(productAssetService.ListAssets(services.AssetFilters{})) != 0 {
		t.Fatalf("expected no server assets after import")
	}

	if _, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
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

	if len(productAssetService.ListAssets(services.AssetFilters{})) != 0 {
		t.Fatalf("expected no server assets before explicit submit")
	}

	authHeader := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	tokenBody, err := json.Marshal(map[string]any{"product_id": product.ID})
	if err != nil {
		t.Fatalf("Marshal() token request error = %v", err)
	}
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/uploads/tokens", bytes.NewReader(tokenBody))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenReq.Header.Set("Authorization", authHeader)
	tokenResp := httptest.NewRecorder()
	apiServer.Engine().ServeHTTP(tokenResp, tokenReq)
	if tokenResp.Code != http.StatusCreated {
		t.Fatalf("expected upload token status 201, got %d, body=%s", tokenResp.Code, tokenResp.Body.String())
	}

	var tokenPayload struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(tokenResp.Body.Bytes(), &tokenPayload); err != nil {
		t.Fatalf("unmarshal upload token response failed: %v", err)
	}
	if tokenPayload.Data.Token == "" {
		t.Fatalf("expected upload token")
	}

	submitted, err := workspace.SubmitItem(context.Background(), item.ID, WorkspaceSubmitInput{
		ProductID:   product.ID,
		UploadURL:   httpSrv.URL + "/api/uploads/clean-shot",
		UploadToken: tokenPayload.Data.Token,
	})
	if err != nil {
		t.Fatalf("SubmitItem() error = %v", err)
	}
	if submitted.Status != workspaceStatusSubmitted {
		t.Fatalf("expected submitted status, got %s", submitted.Status)
	}

	assets := productAssetService.ListAssets(services.AssetFilters{})
	if len(assets) != 1 {
		t.Fatalf("expected 1 server asset after submit, got %d", len(assets))
	}
	if assets[0].ID != submitted.SubmittedAssetID {
		t.Fatalf("expected submitted asset id %s, got %s", submitted.SubmittedAssetID, assets[0].ID)
	}

	if err := workspace.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if len(workspace.ListItems()) != 0 {
		t.Fatalf("expected workspace empty after clear")
	}
	assets = productAssetService.ListAssets(services.AssetFilters{})
	if len(assets) != 1 {
		t.Fatalf("expected server asset to remain after local clear, got %d", len(assets))
	}
}

func makeDevToken(user auth.User) string {
	payload, _ := json.Marshal(user)
	return base64.RawURLEncoding.EncodeToString(payload)
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
