package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestBGMTrackHandlersLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storageRoot := t.TempDir()
	bgmService := services.NewBGMTrackService(storageRoot).WithProbe(func(context.Context, string) (ffmpeg.ProbeResult, error) {
		return ffmpeg.ProbeResult{HasAudio: true, DurationMs: 9_000, AudioSampleRate: 48_000, AudioChannels: 2}, nil
	})
	server := New(Options{Config: config.Config{StorageRoot: storageRoot, QueueBackend: "file"}, BGMTrackService: bgmService})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", "轻快节奏")
	_ = writer.WriteField("mood", "轻快")
	_ = writer.WriteField("bpm", "120")
	_ = writer.WriteField("tags_json", `["骑行","活力"]`)
	file, err := writer.CreateFormFile("audio", "ride.mp3")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("fake-audio"))
	_ = writer.Close()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/bgm-tracks", body)
	createRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	createRequest.Header.Set("Content-Type", writer.FormDataContentType())
	createRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create track: %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Data services.BGMTrack `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/bgm-tracks?include_inactive=true", nil)
	listRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	listRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || !bytes.Contains(listRecorder.Body.Bytes(), []byte(created.Data.ID)) {
		t.Fatalf("list tracks: %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	updatePayload := bytes.NewBufferString(`{"name":"夜骑节奏","bpm":108,"mood":"科技","tags":["夜骑"],"status":"disabled"}`)
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/bgm-tracks/"+created.Data.ID, updatePayload)
	updateRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(updateRecorder, updateRequest)
	if updateRecorder.Code != http.StatusOK || !bytes.Contains(updateRecorder.Body.Bytes(), []byte(`"status":"disabled"`)) {
		t.Fatalf("update track: %d body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	archiveRequest := httptest.NewRequest(http.MethodDelete, "/api/bgm-tracks/"+created.Data.ID, nil)
	archiveRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	archiveRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(archiveRecorder, archiveRequest)
	if archiveRecorder.Code != http.StatusOK || !bytes.Contains(archiveRecorder.Body.Bytes(), []byte(`"status":"archived"`)) {
		t.Fatalf("archive track: %d body=%s", archiveRecorder.Code, archiveRecorder.Body.String())
	}
}

func TestVoiceoverTaskBatchResolvesDistinctRandomBGM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	storageRoot := t.TempDir()
	bgmService := services.NewBGMTrackService(storageRoot).WithProbe(func(context.Context, string) (ffmpeg.ProbeResult, error) {
		return ffmpeg.ProbeResult{HasAudio: true, DurationMs: 9_000}, nil
	})
	for _, name := range []string{"轻快 A", "轻快 B"} {
		if _, err := bgmService.Create(ctx, services.BGMTrackUpload{
			BGMTrackInput: services.BGMTrackInput{Name: name}, FileName: name + ".mp3", Reader: bytes.NewBufferString(name),
		}, ""); err != nil {
			t.Fatalf("create BGM fixture: %v", err)
		}
	}
	voiceovers := services.NewVoiceoverService(storageRoot, config.Config{StorageRoot: storageRoot}, nil).
		WithClients(voiceoverHandlerSynthesizer{}, nil)
	products := services.NewProductAssetService()
	runs := services.NewGenerationRunService(voiceovers)
	server := New(Options{
		Config: config.Config{StorageRoot: storageRoot, QueueBackend: "file"}, VoiceoverService: voiceovers,
		ProductAssetService: products, GenerationRunService: runs, BGMTrackService: bgmService,
	})
	profileRequest := voiceoverProfileMultipartRequest(t, http.MethodPost, "/api/admin/voice-profiles")
	profileRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(profileRecorder, profileRequest)
	var profile struct {
		Data services.VoiceProfile `json:"data"`
	}
	if profileRecorder.Code != http.StatusCreated || json.Unmarshal(profileRecorder.Body.Bytes(), &profile) != nil {
		t.Fatalf("create voice profile: %d body=%s", profileRecorder.Code, profileRecorder.Body.String())
	}
	if err := voiceovers.ProcessVoiceProfilePreview(ctx, profile.Data.ID); err != nil {
		t.Fatalf("prepare voice profile: %v", err)
	}
	product := products.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	payload, _ := json.Marshal(map[string]any{
		"product_id": product.ID, "voice_profile_id": profile.Data.ID, "output_ratio": services.OutputRatioNineSixteen,
		"variants": []map[string]any{
			{"hook": "文案 A", "script_text": "第一条文案。", "editing_intent": "展示产品", "bgm": map[string]any{"mode": "random", "gain_db": -12}},
			{"hook": "文案 B", "script_text": "第二条文案。", "editing_intent": "展示产品", "bgm": map[string]any{"mode": "random", "gain_db": -12}},
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/workbench/voiceover-tasks", bytes.NewReader(payload))
	request.Header.Set("Authorization", voiceoverUserAuthHeader())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create voiceover tasks: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []services.VoiceoverWork `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 2 || response.Data[0].BGM == nil || response.Data[1].BGM == nil {
		t.Fatalf("unexpected work BGM projection %#v", response.Data)
	}
	if response.Data[0].BGM.TrackID == response.Data[1].BGM.TrackID || response.Data[0].BGM.GainDB != -12 || response.Data[1].BGM.GainDB != -12 {
		t.Fatalf("random BGM was not resolved distinctly: %#v %#v", response.Data[0].BGM, response.Data[1].BGM)
	}
}
