package httpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
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
