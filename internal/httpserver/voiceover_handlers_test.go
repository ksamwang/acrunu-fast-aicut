package httpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type voiceoverHandlerSynthesizer struct{}

func (voiceoverHandlerSynthesizer) Synthesize(_ context.Context, _ modelgateway.CosyVoiceSynthesisInput) (modelgateway.CosyVoiceSynthesisResult, error) {
	return modelgateway.CosyVoiceSynthesisResult{
		Audio:      voiceoverHandlerWAV(),
		Model:      "test-cosyvoice",
		SampleRate: 1000,
	}, nil
}

type voiceoverHandlerTranscriber struct{}

func (voiceoverHandlerTranscriber) Transcribe(_ context.Context, _ modelgateway.FunASRTranscriptionInput) (modelgateway.ASRTranscriptionResult, error) {
	return modelgateway.ASRTranscriptionResult{
		Text: "固定裤脚",
		Segments: []modelgateway.ASRTranscriptSegment{{
			StartMs: 0,
			EndMs:   400,
			Text:    "固定裤脚",
		}},
	}, nil
}

func TestVoiceProfileHandlersCreateListAndQueueAudition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storageRoot := t.TempDir()
	voiceoverService := services.NewVoiceoverService(storageRoot, config.Config{StorageRoot: storageRoot}, nil).
		WithClients(voiceoverHandlerSynthesizer{}, nil)
	server := New(Options{
		Config:           config.Config{StorageRoot: storageRoot, QueueBackend: "file"},
		VoiceoverService: voiceoverService,
	})

	createRequest := voiceoverProfileMultipartRequest(t, http.MethodPost, "/api/admin/voice-profiles")
	createRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Data services.VoiceProfile `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created profile: %v", err)
	}
	if created.Data.ID == "" || created.Data.PreviewStatus != "queued" {
		t.Fatalf("unexpected created profile %#v", created.Data)
	}

	if err := voiceoverService.ProcessVoiceProfilePreview(context.Background(), created.Data.ID); err != nil {
		t.Fatalf("generate fixed preview: %v", err)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/voice-profiles", nil)
	listRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	listRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed struct {
		Data []services.VoiceProfile `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listed profiles: %v", err)
	}
	if len(listed.Data) != 1 || listed.Data[0].PreviewStatus != "ready" || listed.Data[0].PreviewAudioURL == "" {
		t.Fatalf("unexpected listed profiles %#v", listed.Data)
	}

	emptyAuditionRequest := httptest.NewRequest(http.MethodPost, "/api/voice-profiles/"+created.Data.ID+"/auditions", bytes.NewBufferString(`{"text":" "}`))
	emptyAuditionRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	emptyAuditionRequest.Header.Set("Content-Type", "application/json")
	emptyAuditionRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(emptyAuditionRecorder, emptyAuditionRequest)
	if emptyAuditionRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty audition, got %d body=%s", emptyAuditionRecorder.Code, emptyAuditionRecorder.Body.String())
	}

	auditionRequest := httptest.NewRequest(http.MethodPost, "/api/voice-profiles/"+created.Data.ID+"/auditions", bytes.NewBufferString(`{"text":"这是一段工作台试听文案。"}`))
	auditionRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	auditionRequest.Header.Set("Content-Type", "application/json")
	auditionRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(auditionRecorder, auditionRequest)
	if auditionRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", auditionRecorder.Code, auditionRecorder.Body.String())
	}
	var audition struct {
		Data services.VoiceAudition `json:"data"`
	}
	if err := json.Unmarshal(auditionRecorder.Body.Bytes(), &audition); err != nil {
		t.Fatalf("decode audition: %v", err)
	}
	if audition.Data.ID == "" || audition.Data.Status != "queued" {
		t.Fatalf("unexpected audition %#v", audition.Data)
	}

	defaultRequest := httptest.NewRequest(http.MethodPost, "/api/admin/voice-profiles/"+created.Data.ID+"/default", nil)
	defaultRequest.Header.Set("Authorization", adminAuthHeader())
	defaultRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(defaultRecorder, defaultRequest)
	if defaultRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", defaultRecorder.Code, defaultRecorder.Body.String())
	}
}

func TestRetryFailedVoiceoverWorkReusesCompletedNarration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storageRoot := t.TempDir()
	voiceoverService := services.NewVoiceoverService(storageRoot, config.Config{StorageRoot: storageRoot}, nil).
		WithClients(voiceoverHandlerSynthesizer{}, voiceoverHandlerTranscriber{})
	taskService := services.NewTaskService(storageRoot)
	productService := services.NewProductAssetService()
	generationRuns := services.NewGenerationRunService(voiceoverService)
	server := New(Options{
		Config:               config.Config{StorageRoot: storageRoot, QueueBackend: "file"},
		TaskService:          taskService,
		VoiceoverService:     voiceoverService,
		ProductAssetService:  productService,
		GenerationRunService: generationRuns,
	})

	profileRequest := voiceoverProfileMultipartRequest(t, http.MethodPost, "/api/admin/voice-profiles")
	profileRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(profileRecorder, profileRequest)
	if profileRecorder.Code != http.StatusCreated {
		t.Fatalf("create profile: %d body=%s", profileRecorder.Code, profileRecorder.Body.String())
	}
	var profileResponse struct {
		Data services.VoiceProfile `json:"data"`
	}
	if err := json.Unmarshal(profileRecorder.Body.Bytes(), &profileResponse); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if err := voiceoverService.ProcessVoiceProfilePreview(context.Background(), profileResponse.Data.ID); err != nil {
		t.Fatalf("generate voice profile preview: %v", err)
	}

	product := productService.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	run, err := generationRuns.Create(context.Background(), services.CreateGenerationRunInput{
		ProductID:       product.ID,
		CreatedByUserID: "voice-user-1",
		ConfigSnapshot:  map[string]any{"variant_index": 1},
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	voiceTask, err := taskService.CreateVoiceoverGenerateTask(context.Background(), "voice-user-1", product.ID, queue.VoiceoverGeneratePayload{GenerationRunID: run.ID})
	if err != nil {
		t.Fatalf("create voice task: %v", err)
	}
	if err := generationRuns.LinkTask(context.Background(), run.ID, voiceTask.ID, "voiceover"); err != nil {
		t.Fatalf("link voice task: %v", err)
	}
	_, scriptVariantID, voiceoverID, err := voiceoverService.CreateVoiceoverWork(context.Background(), services.CreateVoiceoverWorkInput{
		TaskID:         voiceTask.ID,
		ProductID:      product.ID,
		ProductName:    product.Name,
		VoiceProfileID: profileResponse.Data.ID,
		VariantIndex:   1,
		Variant: services.VoiceoverVariantInput{
			Hook:       "裤脚不再蹭链条",
			ScriptText: "固定裤脚，骑行更安心。",
			Beats: []services.VoiceoverBeat{{
				Label: "固定", VisualGoal: "展示束裤带固定裤脚", SourceType: "visual_only",
			}},
		},
	})
	if err != nil {
		t.Fatalf("create voice work: %v", err)
	}
	if err := generationRuns.AttachVoiceoverArtifacts(context.Background(), run.ID, voiceTask.ID, scriptVariantID, voiceoverID); err != nil {
		t.Fatalf("attach voice work: %v", err)
	}
	if err := voiceoverService.ProcessVoiceoverGenerate(context.Background(), queue.VoiceoverGeneratePayload{
		TaskID: voiceTask.ID, GenerationRunID: run.ID, ScriptVariantID: scriptVariantID, VoiceoverID: voiceoverID,
	}); err != nil {
		t.Fatalf("generate source voiceover: %v", err)
	}
	if err := generationRuns.MarkFailed(context.Background(), run.ID, errors.New("edit planner timeout")); err != nil {
		t.Fatalf("mark failed run: %v", err)
	}

	retryRequest := httptest.NewRequest(http.MethodPost, "/api/workbench/works/"+run.ID+"/retry", nil)
	retryRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	retryRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(retryRecorder, retryRequest)
	if retryRecorder.Code != http.StatusOK {
		t.Fatalf("retry failed work: %d body=%s", retryRecorder.Code, retryRecorder.Body.String())
	}
	var retried struct {
		Data services.VoiceoverWork `json:"data"`
	}
	if err := json.Unmarshal(retryRecorder.Body.Bytes(), &retried); err != nil {
		t.Fatalf("decode retried work: %v", err)
	}
	if retried.Data.ID != run.ID || retried.Data.Status != "generating" || retried.Data.StageLabel != "召回素材" || retried.Data.AudioURL == "" {
		t.Fatalf("unexpected retried work %#v", retried.Data)
	}
	if _, exists, err := generationRuns.FindTaskByStage(context.Background(), run.ID, "edit_plan"); err != nil || !exists {
		t.Fatalf("expected retried edit plan task link: exists=%t err=%v", exists, err)
	}
}

func voiceoverProfileMultipartRequest(t *testing.T, method string, path string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for name, value := range map[string]string{
		"name":            "测试旁白",
		"language":        "中文",
		"style_tags_json": `["自然","亲和"]`,
		"reference_text":  "这是参考音频对应的文本。",
		"preview_text":    "这是固定的样音文本。",
		"status":          "enabled",
		"is_default":      "true",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	file, err := writer.CreateFormFile("reference_audio", "reference.wav")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := file.Write(voiceoverHandlerWAV()); err != nil {
		t.Fatalf("write multipart audio: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", adminAuthHeader())
	return request
}

func voiceoverUserAuthHeader() string {
	return "Bearer " + makeDevToken(auth.User{
		ID:          "voice-user-1",
		Username:    "voice-user",
		DisplayName: "Voice User",
		Role:        auth.RoleUser,
	})
}

func voiceoverHandlerWAV() []byte {
	const sampleRate = 1000
	const durationMs = 400
	const byteRate = sampleRate * 2
	const dataSize = byteRate * durationMs / 1000
	payload := make([]byte, 44+dataSize)
	copy(payload[0:4], "RIFF")
	binary.LittleEndian.PutUint32(payload[4:8], uint32(36+dataSize))
	copy(payload[8:12], "WAVE")
	copy(payload[12:16], "fmt ")
	binary.LittleEndian.PutUint32(payload[16:20], 16)
	binary.LittleEndian.PutUint16(payload[20:22], 1)
	binary.LittleEndian.PutUint16(payload[22:24], 1)
	binary.LittleEndian.PutUint32(payload[24:28], sampleRate)
	binary.LittleEndian.PutUint32(payload[28:32], byteRate)
	binary.LittleEndian.PutUint16(payload[32:34], 2)
	binary.LittleEndian.PutUint16(payload[34:36], 16)
	copy(payload[36:40], "data")
	binary.LittleEndian.PutUint32(payload[40:44], dataSize)
	return payload
}
