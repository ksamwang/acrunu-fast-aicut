package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubHandler struct {
	testTaskID       string
	extractedAssetID string
	analyzedAssetID  string
}

func (h *stubHandler) HandleTestTask(_ context.Context, payload TestTaskPayload) error {
	h.testTaskID = payload.TaskID
	return nil
}

func (h *stubHandler) HandleAssetExtractFrames(_ context.Context, payload AssetExtractFramesPayload) error {
	h.extractedAssetID = payload.AssetID
	return nil
}

func (h *stubHandler) HandleAssetAnalyze(_ context.Context, payload AssetAnalyzePayload) error {
	h.analyzedAssetID = payload.AssetID
	return nil
}

func TestHandleFileTaskDispatchesAssetAnalyze(t *testing.T) {
	payload, err := json.Marshal(AssetAnalyzePayload{AssetID: "asset-1"})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	handler := &stubHandler{}
	if err := handleFileTask(context.Background(), FileTask{
		Type:    TypeAssetAnalyze,
		Payload: payload,
	}, handler); err != nil {
		t.Fatalf("handleFileTask failed: %v", err)
	}

	if handler.analyzedAssetID != "asset-1" {
		t.Fatalf("expected analyzed asset id asset-1, got %s", handler.analyzedAssetID)
	}
}

func TestHandleFileTaskDispatchesAssetExtractFrames(t *testing.T) {
	payload, err := json.Marshal(AssetExtractFramesPayload{AssetID: "asset-2"})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	handler := &stubHandler{}
	if err := handleFileTask(context.Background(), FileTask{
		Type:    TypeAssetExtractFrames,
		Payload: payload,
	}, handler); err != nil {
		t.Fatalf("handleFileTask failed: %v", err)
	}

	if handler.extractedAssetID != "asset-2" {
		t.Fatalf("expected extracted asset id asset-2, got %s", handler.extractedAssetID)
	}
}

func TestHandleFileTaskDispatchesTestTask(t *testing.T) {
	payload, err := json.Marshal(TestTaskPayload{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	handler := &stubHandler{}
	if err := handleFileTask(context.Background(), FileTask{
		Type:    TypeTestTask,
		Payload: payload,
	}, handler); err != nil {
		t.Fatalf("handleFileTask failed: %v", err)
	}

	if handler.testTaskID != "task-1" {
		t.Fatalf("expected test task id task-1, got %s", handler.testTaskID)
	}
}

func TestHandleFileTaskReturnsPayloadDecodeError(t *testing.T) {
	handler := &stubHandler{}
	err := handleFileTask(context.Background(), FileTask{
		Type:    TypeAssetExtractFrames,
		Payload: []byte("{invalid-json"),
	}, handler)
	if err == nil {
		t.Fatalf("expected payload decode error")
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected json syntax error, got %T", err)
	}
}

func TestHandleFileTaskIgnoresUnknownTaskType(t *testing.T) {
	handler := &stubHandler{}
	if err := handleFileTask(context.Background(), FileTask{
		Type:    "unknown:task",
		Payload: []byte(`{}`),
	}, handler); err != nil {
		t.Fatalf("expected unknown task type to be ignored, got %v", err)
	}
}
