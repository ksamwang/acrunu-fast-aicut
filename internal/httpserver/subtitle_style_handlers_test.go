package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestSubtitleStylePresetHandlersExposeEnabledStylesAndAdminMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := services.NewSubtitleStylePresetService()
	server := New(Options{
		Config:                     config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SubtitleStylePresetService: service,
	})

	listRequest := httptest.NewRequest(http.MethodGet, "/api/subtitle-presets", nil)
	listRequest.Header.Set("Authorization", voiceoverUserAuthHeader())
	listRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list presets: %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed struct {
		Data []services.SubtitleStylePreset `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil || len(listed.Data) != 1 || !listed.Data[0].IsDefault {
		t.Fatalf("unexpected presets response %#v err=%v", listed.Data, err)
	}

	input := services.DefaultSubtitleStylePresetInput()
	input.Name = "描边白字"
	input.BackgroundOpacity = 0
	input.OutlineWidth = 2
	payload, _ := json.Marshal(input)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/subtitle-presets", bytes.NewReader(payload))
	createRequest.Header.Set("Authorization", adminAuthHeader())
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create preset: %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Data services.SubtitleStylePreset `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil || created.Data.Name != input.Name {
		t.Fatalf("unexpected created preset %#v err=%v", created.Data, err)
	}
}
