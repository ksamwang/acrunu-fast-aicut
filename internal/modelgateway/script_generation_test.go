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

func TestBuildScriptGenerationPromptSeparatesCopyFromVisualEvidence(t *testing.T) {
	bundle := BuildScriptGenerationPrompt(ScriptGenerationInput{
		ProductName:             "束裤带",
		VariantCount:            2,
		TargetDurationSeconds:   30,
		AvailableVisualEvidence: []string{"动作：双手压紧魔术贴完成固定"},
		SellingPoints:           []ScriptGenerationSellingPoint{{Name: "魔术贴固定"}},
	})
	if bundle.Version != "workbench-script-v4" || bundle.Schema["version"] != ScriptCopyOutputSchemaVersion {
		t.Fatalf("unexpected copy prompt bundle %#v", bundle)
	}
	prompt := bundle.Prompts[0].System + " " + bundle.Prompts[0].User
	for _, expected := range []string{
		"效果广告文案", "selected_selling_points", "estimated_duration_range_seconds",
		"recommended_spoken_character_range", "122", "148", "常用小物分区放",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected copy prompt to contain %q, got %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "available_visual_evidence") || strings.Contains(prompt, "双手压紧魔术贴") {
		t.Fatalf("copy prompt must not contain raw visual evidence: %s", prompt)
	}
}

func TestBuildScriptVisualIntentPromptUsesApprovedCopyAndEvidence(t *testing.T) {
	copies := validScriptCopyResult()
	bundle := BuildScriptVisualIntentPrompt(ScriptGenerationInput{
		ProductName:             "束裤带",
		TargetDurationSeconds:   15,
		AvailableVisualEvidence: []string{"动作：双手压紧魔术贴完成固定"},
		SellingPoints:           []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	}, copies)
	if bundle.Version != ScriptVisualIntentPromptVersion || bundle.Schema["version"] != ScriptVisualIntentOutputSchemaVersion {
		t.Fatalf("unexpected visual prompt bundle %#v", bundle)
	}
	prompt := bundle.Prompts[0].System + " " + bundle.Prompts[0].User
	for _, expected := range []string{"approved_copy_variants", "available_visual_evidence", "双手压紧魔术贴", copies.Variants[0].ScriptText, "口播文案已经确认"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected visual prompt to contain %q, got %s", expected, prompt)
		}
	}
}

func TestOpenAICompatibleScriptGeneratorUsesSeparatedCopyAndVisualRequests(t *testing.T) {
	requests := 0
	copyRequests := 0
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
		if responseFormat["type"] != "json_object" || request["max_tokens"] != float64(defaultScriptGenerationMaxTokens) {
			t.Fatalf("unexpected request contract %#v", request)
		}
		messages, _ := request["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("unexpected prompt messages %#v", messages)
		}
		userMessage := stringifyChatMessage(messages[1])
		var content []byte
		if strings.Contains(userMessage, "approved_copy_variants") {
			if !strings.Contains(userMessage, "available_visual_evidence") || !strings.Contains(userMessage, validScriptCopyResult().Variants[0].ScriptText) {
				t.Fatalf("visual request is missing approved copy or evidence: %s", userMessage)
			}
			content, _ = json.Marshal(validScriptVisualIntentResult())
		} else {
			copyRequests++
			if strings.Contains(userMessage, "available_visual_evidence") || strings.Contains(userMessage, "双手将束裤带") {
				t.Fatalf("copy request leaked raw visual evidence: %s", userMessage)
			}
			result := validScriptCopyResult()
			if copyRequests == 1 {
				result.Variants[0].ScriptText = result.Variants[0].Hook + "，束裤带一贴就稳。"
			} else if !strings.Contains(userMessage, "未通过服务端校验") {
				t.Fatalf("expected copy repair instruction, got %s", userMessage)
			}
			content, _ = json.Marshal(result)
		}
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
		SellingPoints:           []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if err != nil {
		t.Fatalf("generate scripts: %v", err)
	}
	if requests != 3 || copyRequests != 2 {
		t.Fatalf("expected copy, copy repair, and visual requests; got requests=%d copy=%d", requests, copyRequests)
	}
	if len(result.Variants) != 1 || result.Variants[0].Hook != validScriptCopyResult().Variants[0].Hook || len(result.Variants[0].Beats) != 3 {
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
	if err == nil || !strings.Contains(err.Error(), "estimated duration") || !strings.Contains(err.Error(), "今天给大家推荐") {
		t.Fatalf("expected duration and cliche validation errors, got %v", err)
	}
}

func TestValidateScriptCopyResultRejectsProductionDirections(t *testing.T) {
	result := validScriptCopyResult()
	result.Variants[0].ScriptText = strings.Replace(result.Variants[0].ScriptText, "骑完卷好放进口袋", "最后拉上拉链，双手回到车把", 1)
	err := ValidateScriptCopyResult(result, ScriptGenerationInput{
		VariantCount:          1,
		TargetDurationSeconds: 15,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if err == nil || !strings.Contains(err.Error(), "production-direction") {
		t.Fatalf("expected production-direction validation error, got %v", err)
	}
}

func TestValidateScriptVisualIntentRejectsSellingPointOutsideApprovedCopy(t *testing.T) {
	copies := validScriptCopyResult()
	visuals := validScriptVisualIntentResult()
	visuals.Plans[0].Beats[0].SellingPoint = "模型从素材自造的卖点"
	err := ValidateScriptVisualIntentResult(visuals, copies, ScriptGenerationInput{TargetDurationSeconds: 15})
	if err == nil || !strings.Contains(err.Error(), "outside its approved copy") {
		t.Fatalf("expected approved-copy selling point error, got %v", err)
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
	if minimum != 122 || maximum != 148 {
		t.Fatalf("unexpected 30 second drafting range %d-%d", minimum, maximum)
	}
	minimumDuration, maximumDuration := ScriptEstimatedDurationRangeMs(30)
	if minimumDuration != 27000 || maximumDuration != 33600 {
		t.Fatalf("unexpected 30 second duration range %d-%d", minimumDuration, maximumDuration)
	}
	duration := EstimateScriptDurationMs(validScriptCopyResult().Variants[0].ScriptText)
	if duration < 13500 || duration > 16800 {
		t.Fatalf("expected valid 15 second estimate, got %d", duration)
	}
}

func validScriptCopyResult() ScriptCopyResult {
	return ScriptCopyResult{Variants: []ScriptCopyVariant{{
		VariantIndex:          1,
		Angle:                 "骑行裤脚安全",
		SelectedSellingPoints: []string{"避免蹭链条"},
		Hook:                  "骑车时裤脚总往链条上蹭",
		ScriptText:            "骑车时裤脚总往链条上蹭，沾上油污还可能卷进齿盘。出门前用束裤带收紧裤脚，弹力贴合腿部又不影响蹬车。骑完卷好放进口袋，收纳不占地方，骑行也更利落。",
	}}}
}

func validScriptVisualIntentResult() ScriptVisualIntentResult {
	return ScriptVisualIntentResult{Plans: []ScriptVisualIntentPlan{{
		VariantIndex:  1,
		EditingIntent: "从裤脚风险切入，呈现束裤带固定裤脚与便携收纳结果。",
		Beats: []ScriptGenerationBeat{
			{Label: "裤脚风险", SellingPoint: "避免蹭链条", VisualGoal: "骑行时裤脚靠近自行车链条", SourceType: TTSVisualSourceType},
			{Label: "收紧裤脚", SellingPoint: "避免蹭链条", VisualGoal: "束裤带环绕脚踝并收紧裤脚", SourceType: TTSVisualSourceType},
			{Label: "便携收纳", SellingPoint: "避免蹭链条", VisualGoal: "束裤带卷起后放入口袋收纳", SourceType: TTSVisualSourceType},
		},
	}}}
}

func validGatewayScriptResult() ScriptGenerationResult {
	result, err := mergeScriptGenerationResult(validScriptCopyResult(), validScriptVisualIntentResult())
	if err != nil {
		panic(err)
	}
	return result
}

func stringifyChatMessage(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
