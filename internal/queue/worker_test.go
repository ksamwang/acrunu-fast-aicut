package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type stubHandler struct {
	testTaskID       string
	extractedAssetID string
	analyzedAssetID  string
	embeddedAssetID  string
	extractFailures  int
	extractCalls     int
}

func (h *stubHandler) HandleTestTask(_ context.Context, payload TestTaskPayload) error {
	h.testTaskID = payload.TaskID
	return nil
}

func (h *stubHandler) HandleAssetExtractFrames(_ context.Context, payload AssetExtractFramesPayload) error {
	h.extractCalls++
	h.extractedAssetID = payload.AssetID
	if h.extractFailures > 0 {
		h.extractFailures--
		return errors.New("extract failed")
	}
	return nil
}

func (h *stubHandler) HandleAssetAnalyze(_ context.Context, payload AssetAnalyzePayload) error {
	h.analyzedAssetID = payload.AssetID
	return nil
}

func (h *stubHandler) HandleAssetEmbedding(_ context.Context, payload AssetEmbeddingPayload) error {
	h.embeddedAssetID = payload.AssetID
	return nil
}

func (h *stubHandler) HandleVoiceProfilePreview(_ context.Context, _ VoiceProfilePreviewPayload) error {
	return nil
}

func (h *stubHandler) HandleVoiceAudition(_ context.Context, _ VoiceAuditionPayload) error {
	return nil
}

func (h *stubHandler) HandleVoiceoverGenerate(_ context.Context, _ VoiceoverGeneratePayload) error {
	return nil
}

func (h *stubHandler) HandleEditPlanGenerate(_ context.Context, _ EditPlanGeneratePayload) error {
	return nil
}

func TestHandleFileTaskDispatchesAssetEmbedding(t *testing.T) {
	payload, err := json.Marshal(AssetEmbeddingPayload{AssetID: "asset-3"})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	handler := &stubHandler{}
	if err := handleFileTask(context.Background(), FileTask{
		Type:    TypeAssetEmbedding,
		Payload: payload,
	}, handler); err != nil {
		t.Fatalf("handleFileTask failed: %v", err)
	}

	if handler.embeddedAssetID != "asset-3" {
		t.Fatalf("expected embedded asset id asset-3, got %s", handler.embeddedAssetID)
	}
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

func TestRunFileWorkerRetriesFailedFileTask(t *testing.T) {
	tempDir := t.TempDir()
	fileQueue := NewFileQueue(tempDir)
	payload, err := json.Marshal(AssetExtractFramesPayload{
		TaskID:     "task-1",
		AssetID:    "asset-1",
		StorageKey: "assets/a.mp4",
		DurationMs: 1200,
	})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := fileQueue.Enqueue(context.Background(), TypeAssetExtractFrames, payload, 2); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	handler := &stubHandler{extractFailures: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err = RunFileWorker(ctx, tempDir, handler, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected worker to stop on context deadline, got %v", err)
	}
	if handler.extractCalls != 2 {
		t.Fatalf("expected one retry after failure, got %d calls", handler.extractCalls)
	}
	if handler.extractedAssetID != "asset-1" {
		t.Fatalf("expected retried asset id asset-1, got %s", handler.extractedAssetID)
	}
}
