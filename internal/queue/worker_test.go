package queue

import (
	"context"
	"encoding/json"
	"testing"
)

type stubHandler struct {
	analyzedAssetID string
}

func (h *stubHandler) HandleTestTask(context.Context, TestTaskPayload) error {
	return nil
}

func (h *stubHandler) HandleAssetExtractFrames(context.Context, AssetExtractFramesPayload) error {
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
