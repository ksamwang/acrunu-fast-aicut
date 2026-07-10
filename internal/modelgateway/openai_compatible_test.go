package modelgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAICompatibleAnalyzerRequestsJSONOutput(t *testing.T) {
	tempDir := t.TempDir()
	framePath := filepath.Join(tempDir, "frame.jpg")
	if err := os.WriteFile(framePath, []byte("jpeg"), 0644); err != nil {
		t.Fatalf("write frame failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header %q", got)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		responseFormat, _ := req["response_format"].(map[string]any)
		if responseFormat["type"] != "json_object" {
			t.Fatalf("expected json_object response_format, got %#v", responseFormat)
		}
		if req["max_tokens"].(float64) < 8192 {
			t.Fatalf("expected high max_tokens, got %#v", req["max_tokens"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"scene_description":"车内产品亮灯展示","shot_size":"close_up","camera_movement":"static","visual_tags":["车内","产品特写"],"quality_tags":["画面清晰"],"visible_product":true,"product_position":"center","scene_context":"车内","action_description":"展示产品亮灯效果","people_presence":false,"face_visible":false,"lighting_condition":"夜间弱光"}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	analyzer := NewOpenAICompatibleAnalyzer(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "vlm-test",
	})
	result, err := analyzer.AnalyzeAsset(t.Context(), AnalyzeAssetInput{
		AssetID:    "asset-1",
		SourceType: "visual_only",
		DurationMs: 1200,
		Width:      1080,
		Height:     1920,
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 100, StorageKey: framePath},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeAsset failed: %v", err)
	}
	if result.SceneDescription == "" || len(result.VisualTags) == 0 || !result.VisibleProduct {
		t.Fatalf("unexpected result %#v", result)
	}
}
