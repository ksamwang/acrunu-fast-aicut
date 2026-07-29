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

func TestBuildScriptGenerationPromptUsesInformationFeedContract(t *testing.T) {
	bundle := BuildScriptGenerationPrompt(ScriptGenerationInput{
		ProductName:             "束裤带",
		VariantCount:            2,
		TargetDurationSeconds:   30,
		AvailableVisualEvidence: []string{"动作：双手压紧魔术贴完成固定"},
		SellingPoints:           []ScriptGenerationSellingPoint{{Name: "魔术贴固定"}},
	})
	if bundle.Version != "workbench-script-v3" {
		t.Fatalf("unexpected prompt version %s", bundle.Version)
	}
	prompt := bundle.Prompts[0].System + " " + bundle.Prompts[0].User
	for _, expected := range []string{
		"information-feed", "performance-marketing", "135", "172", "target_duration_seconds",
		"available_visual_evidence", "semantic search query", "One clause should express one visible action or state",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got %s", expected, prompt)
		}
	}
}

func TestOpenAICompatibleScriptGeneratorRequestsJSONOutput(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
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
		userMessage := stringifyChatMessage(messages[1])
		if len(messages) != 2 || !strings.Contains(userMessage, "target_duration_seconds") || !strings.Contains(userMessage, "available_visual_evidence") {
			t.Fatalf("unexpected prompt messages %#v", messages)
		}
		if requests == 2 && !strings.Contains(userMessage, "failed server validation") {
			t.Fatalf("expected validation repair instruction, got %s", userMessage)
		}
		result := validGatewayScriptResult()
		if requests == 1 {
			result.Variants[0].ScriptText = result.Variants[0].Hook + "，束裤带一贴就稳。"
		}
		content, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(content)}}},
		})
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
		ProductName:             "束裤带",
		VariantCount:            1,
		TargetDurationSeconds:   15,
		AvailableVisualEvidence: []string{"动作：双手将束裤带绕过脚踝并压紧魔术贴"},
		SellingPoints: []ScriptGenerationSellingPoint{{
			Name: "避免蹭链条",
		}},
	})
	if err != nil {
		t.Fatalf("generate scripts: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected one automatic repair request, got %d", requests)
	}
	if len(result.Variants) != 1 || result.Variants[0].Hook != "骑车时裤脚总往链条上蹭" || len(result.Variants[0].Beats) != 3 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestValidateScriptGenerationResultRejectsNonVisualOnlySourceType(t *testing.T) {
	result := validGatewayScriptResult()
	result.Variants[0].Beats[0].SourceType = "talking_head"
	err := ValidateScriptGenerationResult(result, ScriptGenerationInput{VariantCount: 1, TargetDurationSeconds: 15})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateScriptGenerationResultRejectsShortAndGenericCopy(t *testing.T) {
	result := validGatewayScriptResult()
	result.Variants[0].ScriptText = result.Variants[0].Hook + "，今天给大家推荐这款实用神器。"
	err := ValidateScriptGenerationResult(result, ScriptGenerationInput{VariantCount: 1, TargetDurationSeconds: 30})
	if err == nil || !strings.Contains(err.Error(), "spoken characters") || !strings.Contains(err.Error(), "今天给大家推荐") {
		t.Fatalf("expected duration and cliche validation errors, got %v", err)
	}
}

func TestValidateScriptGenerationResultRejectsUnsupportedAndMissingSellingPoints(t *testing.T) {
	result := validGatewayScriptResult()
	for index := range result.Variants[0].Beats {
		result.Variants[0].Beats[index].SellingPoint = "模型自造卖点"
	}
	err := ValidateScriptGenerationResult(result, ScriptGenerationInput{
		VariantCount:          1,
		TargetDurationSeconds: 15,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported selling_point") || !strings.Contains(err.Error(), "does not cover selling_point") {
		t.Fatalf("expected selling point validation errors, got %v", err)
	}
}

func TestScriptDurationRules(t *testing.T) {
	minimum, maximum := ScriptSpokenCharacterRange(30)
	if minimum != 135 || maximum != 172 {
		t.Fatalf("unexpected 30 second range %d-%d", minimum, maximum)
	}
	if duration := EstimateScriptDurationMs("骑车前收紧裤脚，骑行更利落。"); duration < 8000 {
		t.Fatalf("expected minimum duration estimate, got %d", duration)
	}
}

func validGatewayScriptResult() ScriptGenerationResult {
	return ScriptGenerationResult{Variants: []ScriptGenerationVariant{{
		Hook:          "骑车时裤脚总往链条上蹭",
		ScriptText:    "骑车时裤脚总往链条上蹭，油污难清理，还可能卷进齿盘。把束裤带绕在脚踝上，调好松紧后压紧魔术贴，裤脚马上被稳稳收住。弹力贴合不勒腿，骑完卷好放进口袋，出门骑行更利落。",
		EditingIntent: "采用痛点解决角度，展示裤脚风险、固定动作和收纳结果。",
		Beats: []ScriptGenerationBeat{
			{Label: "痛点", SellingPoint: "避免蹭链条", VisualGoal: "骑行时裤脚靠近自行车链条", SourceType: "visual_only"},
			{Label: "固定", SellingPoint: "避免蹭链条", VisualGoal: "双手将束裤带绕过脚踝并压紧魔术贴", SourceType: "visual_only"},
			{Label: "结果", SellingPoint: "避免蹭链条", VisualGoal: "束裤带固定裤脚并保持贴合状态", SourceType: "visual_only"},
		},
	}}}
}

func stringifyChatMessage(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
