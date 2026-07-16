package modelgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleScriptGeneratorRequestsJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer script-key" {
			t.Fatalf("unexpected authorization %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		responseFormat, _ := request["response_format"].(map[string]any)
		if responseFormat["type"] != "json_object" {
			t.Fatalf("expected json_object response format, got %#v", responseFormat)
		}
		if request["max_tokens"] != float64(defaultScriptGenerationMaxTokens) {
			t.Fatalf("unexpected max_tokens %#v", request["max_tokens"])
		}
		messages, _ := request["messages"].([]any)
		if len(messages) != 2 || !strings.Contains(stringifyChatMessage(messages[1]), "variant_count") {
			t.Fatalf("unexpected prompt messages %#v", messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"variants\":[{\"hook\":\"骑行更利落\",\"script_text\":\"裤脚总会蹭到链条？轻轻一贴，骑行更利落。\",\"editing_intent\":\"从骑行痛点切入，再展示固定效果。\",\"beats\":[{\"label\":\"开头\",\"selling_point\":\"避免蹭链条\",\"visual_goal\":\"展示裤脚靠近链条。\",\"source_type\":\"visual_only\"},{\"label\":\"展示\",\"selling_point\":\"避免蹭链条\",\"visual_goal\":\"展示贴合动作。\",\"source_type\":\"visual_only\"},{\"label\":\"收束\",\"selling_point\":\"避免蹭链条\",\"visual_goal\":\"展示骑行结果。\",\"source_type\":\"visual_only\"}]}]}"}}]}`))
	}))
	defer server.Close()

	generator := NewOpenAICompatibleScriptGenerator(Config{
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		APIKey:   "script-key",
		Model:    "script-model",
		Timeout:  time.Second,
	})
	result, err := generator.GenerateScripts(context.Background(), ScriptGenerationInput{
		ProductName:  "束裤带",
		VariantCount: 1,
		SellingPoints: []ScriptGenerationSellingPoint{{
			Name: "避免蹭链条",
		}},
	})
	if err != nil {
		t.Fatalf("generate scripts: %v", err)
	}
	if len(result.Variants) != 1 || result.Variants[0].Hook != "骑行更利落" || len(result.Variants[0].Beats) != 3 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestValidateScriptGenerationResultRejectsNonVisualOnlySourceType(t *testing.T) {
	err := ValidateScriptGenerationResult(ScriptGenerationResult{Variants: []ScriptGenerationVariant{{
		Hook:          "钩子",
		ScriptText:    "一段文案",
		EditingIntent: "一个意图",
		Beats: []ScriptGenerationBeat{
			{Label: "开头", SellingPoint: "卖点", VisualGoal: "画面", SourceType: "talking_head"},
			{Label: "展示", SellingPoint: "卖点", VisualGoal: "画面", SourceType: "visual_only"},
			{Label: "收束", SellingPoint: "卖点", VisualGoal: "画面", SourceType: "visual_only"},
		},
	}}}, 1)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func stringifyChatMessage(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
