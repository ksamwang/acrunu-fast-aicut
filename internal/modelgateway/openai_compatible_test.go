package modelgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		messages, _ := req["messages"].([]any)
		userMessage, _ := messages[1].(map[string]any)
		content, _ := userMessage["content"].([]any)
		if len(content) != 3 {
			t.Fatalf("expected prompt, frame marker, and image, got %#v", content)
		}
		frameMarker, _ := content[1].(map[string]any)
		if marker, _ := frameMarker["text"].(string); !strings.Contains(marker, "Video frame 1/1") || !strings.Contains(marker, "timestamp_ms=100") {
			t.Fatalf("expected explicit frame index and timestamp marker, got %#v", frameMarker)
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

func TestOpenAICompatibleAnalyzerRepairsLowValueAnalysisOnce(t *testing.T) {
	tempDir := t.TempDir()
	framePath := filepath.Join(tempDir, "frame.jpg")
	if err := os.WriteFile(framePath, []byte("jpeg"), 0644); err != nil {
		t.Fatalf("write frame failed: %v", err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		messages, _ := req["messages"].([]any)
		userMessage, _ := messages[1].(map[string]any)
		content, _ := userMessage["content"].([]any)
		promptBlock, _ := content[0].(map[string]any)
		prompt, _ := promptBlock["text"].(string)

		response := `{"scene_description":"杜邦车包固定在车把上，包体完整清晰可见","shot_size":"close_up","camera_movement":"static","visual_tags":["杜邦车包","车把安装"],"quality_tags":["画面清晰"],"visible_product":true,"product_position":"车把中央","scene_context":"户外","action_description":"视频持续展示已固定的杜邦车包，未见拆装或状态变化","people_presence":false,"face_visible":false,"lighting_condition":"自然光"}`
		if requests == 2 {
			if !strings.Contains(prompt, "The previous JSON was rejected") || !strings.Contains(prompt, "持续展示") {
				t.Fatalf("expected targeted repair instruction, got %s", prompt)
			}
			response = `{"scene_description":"杜邦车包安装在车把上，水流冲淋包体表面","shot_size":"close_up","camera_movement":"static","visual_tags":["杜邦车包","水流冲淋","防泼水展示"],"quality_tags":["画面清晰"],"visible_product":true,"product_position":"车把中央","scene_context":"户外","action_description":"水流持续冲淋车包表面，展示防泼水效果","people_presence":false,"face_visible":false,"lighting_condition":"自然光"}`
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": response}}},
		})
	}))
	defer server.Close()

	analyzer := NewOpenAICompatibleAnalyzer(Config{BaseURL: server.URL, Model: "vlm-test"})
	result, err := analyzer.AnalyzeAsset(t.Context(), AnalyzeAssetInput{
		AssetID:    "asset-1",
		SourceType: "visual_only",
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 0, StorageKey: framePath},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeAsset failed: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected one repair request, got %d requests", requests)
	}
	if result.SceneDescription != "杜邦车包安装在车把上，水流冲淋包体表面" {
		t.Fatalf("expected repaired description, got %#v", result)
	}
	if attempted, _ := result.ModelResult["repair_attempted"].(bool); !attempted {
		t.Fatalf("expected repair metadata, got %#v", result.ModelResult)
	}
}

func TestOpenAICompatibleAnalyzerKeepsValidResultWhenRepairStillUsesGenericWording(t *testing.T) {
	tempDir := t.TempDir()
	framePath := filepath.Join(tempDir, "frame.jpg")
	if err := os.WriteFile(framePath, []byte("jpeg"), 0644); err != nil {
		t.Fatalf("write frame failed: %v", err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"scene_description":"杜邦车包安装在车把上，包体清晰可见","shot_size":"close_up","camera_movement":"static","visual_tags":["杜邦车包","车把安装"],"quality_tags":["画面清晰"],"visible_product":true,"product_position":"车把前方","scene_context":"户外","action_description":"无明显操作，持续展示车把安装状态","people_presence":false,"face_visible":false,"lighting_condition":"自然光"}`}}},
		})
	}))
	defer server.Close()

	analyzer := NewOpenAICompatibleAnalyzer(Config{BaseURL: server.URL, Model: "vlm-test"})
	result, err := analyzer.AnalyzeAsset(t.Context(), AnalyzeAssetInput{
		AssetID: "asset-1",
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 0, StorageKey: framePath},
		},
	})
	if err != nil {
		t.Fatalf("expected valid repaired result to be retained, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected exactly one repair request, got %d", requests)
	}
	if warnings, ok := result.ModelResult["quality_warnings"].([]string); !ok || len(warnings) == 0 {
		t.Fatalf("expected remaining quality warnings in model metadata, got %#v", result.ModelResult)
	}
}

func TestOpenAICompatibleAnalyzerDoesNotOverrideProviderProductVisibility(t *testing.T) {
	tempDir := t.TempDir()
	framePath := filepath.Join(tempDir, "frame.jpg")
	if err := os.WriteFile(framePath, []byte("jpeg"), 0644); err != nil {
		t.Fatalf("write frame failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"scene_description":"杜邦车包安装在车把前方","shot_size":"close_up","camera_movement":"static","visual_tags":["杜邦车包","车把安装"],"quality_tags":[],"visible_product":false,"product_position":"车把前方","scene_context":"户外","action_description":"车把安装状态","people_presence":false,"face_visible":false,"lighting_condition":"自然光"}`}}},
		})
	}))
	defer server.Close()

	analyzer := NewOpenAICompatibleAnalyzer(Config{BaseURL: server.URL, Model: "vlm-test"})
	result, err := analyzer.AnalyzeAsset(t.Context(), AnalyzeAssetInput{
		AssetID:     "asset-1",
		SourceType:  "visual_only",
		ProductName: "杜邦车包",
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 0, StorageKey: framePath},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeAsset failed: %v", err)
	}
	if result.VisibleProduct {
		t.Fatalf("expected provider product visibility to be preserved, got %#v", result)
	}
	if _, normalized := result.ModelResult["visible_product_normalized"]; normalized {
		t.Fatalf("expected no product visibility mutation metadata, got %#v", result.ModelResult)
	}
	if warnings, ok := result.ModelResult["quality_warnings"].([]string); !ok || len(warnings) == 0 {
		t.Fatalf("expected contradictory provider output to remain visible as a quality warning, got %#v", result.ModelResult)
	}
}

func TestDecodeAnalyzeAssetResultNormalizesStringBooleans(t *testing.T) {
	result, err := decodeAnalyzeAssetResult(`{
		"scene_description":"车内产品亮灯展示",
		"shot_size":"close_up",
		"camera_movement":"static",
		"visual_tags":"车内、产品特写",
		"quality_tags":"画面清晰",
		"visible_product":"可见",
		"product_position":"center",
		"scene_context":"车内",
		"action_description":"展示产品亮灯效果",
		"people_presence":"无",
		"face_visible":"false",
		"lighting_condition":"夜间弱光"
	}`)
	if err != nil {
		t.Fatalf("decodeAnalyzeAssetResult failed: %v", err)
	}
	if !result.VisibleProduct {
		t.Fatalf("expected visible_product string to become true")
	}
	if result.PeoplePresence || result.FaceVisible {
		t.Fatalf("expected negative string booleans to become false, got %#v", result)
	}
	if len(result.VisualTags) != 2 || result.VisualTags[0] != "车内" || result.VisualTags[1] != "产品特写" {
		t.Fatalf("expected string tags to split, got %#v", result.VisualTags)
	}
	if len(result.QualityTags) != 1 || result.QualityTags[0] != "画面清晰" {
		t.Fatalf("expected string quality tags to normalize, got %#v", result.QualityTags)
	}
}
