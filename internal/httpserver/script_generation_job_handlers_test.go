package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestScriptGenerationJobHandlersCreateRecoverCancelAndIsolate(t *testing.T) {
	products := services.NewProductAssetService()
	product := products.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	point, err := products.CreateSellingPoint(product.ID, services.CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	scripts := services.NewScriptGenerationService(products, services.NewSystemConfigService(), services.NewModelProviderService(), config.Config{}).
		WithGenerator(scriptGenerationHandlerGenerator{})
	jobs := services.NewScriptGenerationJobService(nil, scripts)
	storageRoot := t.TempDir()
	server := New(Options{
		Config:                     config.Config{StorageRoot: storageRoot, QueueBackend: "file"},
		ProductAssetService:        products,
		ScriptGenerationService:    scripts,
		ScriptGenerationJobService: jobs,
	})

	created := performScriptGenerationJobRequest(t, server, http.MethodPost, "/api/workbench/script-generation-jobs", voiceoverUserAuthHeader(), `{
		"product_id":"`+product.ID+`",
		"selling_point_ids":["`+point.ID+`"],
		"custom_selling_points":[],
		"variant_count":1,
		"target_duration_seconds":45,
		"mode":"replace_all",
		"base_revision":"draft-v1"
	}`, http.StatusCreated)
	if created.ID == "" || created.Status != services.ScriptGenerationJobStatusQueued || created.BaseRevision != "draft-v1" || created.Input.TargetDurationSeconds != 45 {
		t.Fatalf("unexpected created job %#v", created)
	}

	queued, err := queue.NewFileQueue(storageRoot).Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue script generation task: %v", err)
	}
	if queued.Type != queue.TypeWorkbenchScriptGenerate {
		t.Fatalf("unexpected queued task type %s", queued.Type)
	}
	var payload queue.WorkbenchScriptGeneratePayload
	if err := json.Unmarshal(queued.Payload, &payload); err != nil || payload.JobID != created.ID {
		t.Fatalf("unexpected queue payload %#v err=%v", payload, err)
	}

	latest := performScriptGenerationJobRequest(t, server, http.MethodGet, "/api/workbench/script-generation-jobs/latest", voiceoverUserAuthHeader(), "", http.StatusOK)
	if latest.ID != created.ID {
		t.Fatalf("expected latest job %s, got %#v", created.ID, latest)
	}

	otherUserHeader := "Bearer " + makeDevToken(auth.User{ID: "voice-user-2", Username: "other", DisplayName: "Other", Role: auth.RoleUser})
	request := httptest.NewRequest(http.MethodGet, "/api/workbench/script-generation-jobs/"+created.ID, nil)
	request.Header.Set("Authorization", otherUserHeader)
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected cross-user lookup to return 404, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	cancelled := performScriptGenerationJobRequest(t, server, http.MethodPost, "/api/workbench/script-generation-jobs/"+created.ID+"/cancel", voiceoverUserAuthHeader(), "", http.StatusOK)
	if cancelled.Status != services.ScriptGenerationJobStatusCancelled {
		t.Fatalf("unexpected cancelled job %#v", cancelled)
	}
}

func performScriptGenerationJobRequest(t *testing.T, server *Server, method string, path string, authHeader string, body string, expectedStatus int) services.ScriptGenerationJob {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", authHeader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)
	if recorder.Code != expectedStatus {
		t.Fatalf("expected %d, got %d body=%s", expectedStatus, recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data services.ScriptGenerationJob `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response.Data
}
