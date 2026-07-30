package modelgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	if bundle.Version != "workbench-script-v7" || bundle.Schema["version"] != ScriptCopyOutputSchemaVersion {
		t.Fatalf("unexpected copy prompt bundle %#v", bundle)
	}
	prompt := bundle.Prompts[0].System + " " + bundle.Prompts[0].User
	for _, expected := range []string{
		"商品信息流口播", "selected_selling_points", "preferred_estimated_duration_seconds", "recommended_spoken_character_range",
		"111", "127", "21", "42", "还在找好用的骑行车头包", "闭眼入、一包两用、不用慌", "不限制每条卖点数量",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected copy prompt to contain %q, got %s", expected, prompt)
		}
	}
	for _, forbidden := range []string{"maximum_selling_points_per_variant", "semantic_clause_range", "不要把一条文案写成完整功能清单"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("copy prompt must not contain legacy restriction %q: %s", forbidden, prompt)
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
			} else if !strings.Contains(userMessage, "需要优化") {
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

func TestOpenAICompatibleScriptGeneratorAcceptsNaturalDurationWithoutRepair(t *testing.T) {
	requests := 0
	copyRequests := 0
	copyResult := reasonableNearTargetCopyResult()
	visualResult := ScriptVisualIntentResult{Plans: []ScriptVisualIntentPlan{{
		VariantIndex:  1,
		EditingIntent: "从裤脚风险切入，依次呈现收紧、固定和骑行结果。",
		Beats: []ScriptGenerationBeat{
			{Label: "裤脚风险", SellingPoint: "避免蹭链条", VisualGoal: "骑行时裤脚靠近自行车链条", SourceType: TTSVisualSourceType},
			{Label: "收紧裤脚", SellingPoint: "避免蹭链条", VisualGoal: "束裤带环绕脚踝并收紧裤脚", SourceType: TTSVisualSourceType},
			{Label: "压紧固定", SellingPoint: "避免蹭链条", VisualGoal: "双手压紧束裤带魔术贴完成固定", SourceType: TTSVisualSourceType},
			{Label: "骑行结果", SellingPoint: "避免蹭链条", VisualGoal: "束裤带固定裤脚后的骑行状态", SourceType: TTSVisualSourceType},
		},
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages, _ := request["messages"].([]any)
		userMessage := stringifyChatMessage(messages[1])
		var content []byte
		if strings.Contains(userMessage, "approved_copy_variants") {
			content, _ = json.Marshal(visualResult)
		} else {
			copyRequests++
			content, _ = json.Marshal(copyResult)
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
		ProductName:           "束裤带",
		VariantCount:          1,
		TargetDurationSeconds: 30,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if err != nil {
		t.Fatalf("near-target copy must survive the quality repair: %v", err)
	}
	if requests != 2 || copyRequests != 1 {
		t.Fatalf("expected one copy and one visual request without duration repair; got requests=%d copy=%d", requests, copyRequests)
	}
	if len(result.Variants) != 1 || result.Variants[0].ScriptText != copyResult.Variants[0].ScriptText {
		t.Fatalf("unexpected generated result %#v", result)
	}
}

func TestOpenAICompatibleScriptGeneratorAcceptsBelowPreferredDurationAfterRepair(t *testing.T) {
	requests := 0
	copyRequests := 0
	original := scriptCopyResultWithBodyLength(1, "骑行裤脚安全", "骑车裤脚容易蹭到链条", "避免蹭链条", "稳", 135)
	repaired := scriptCopyResultWithBodyLength(1, "骑行裤脚安全", "骑车裤脚容易蹭到链条", "避免蹭链条", "牢", 140)
	for name, result := range map[string]ScriptCopyResult{"original": original, "repaired": repaired} {
		durationMs := EstimateScriptDurationMs(result.Variants[0].ScriptText)
		if durationMs < 29000 || durationMs > 32000 {
			t.Fatalf("%s fixture must stay near 30 seconds, got %dms", name, durationMs)
		}
	}
	visualResult := scriptVisualIntentResultForDuration(1, "避免蹭链条", 45)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages, _ := request["messages"].([]any)
		userMessage := stringifyChatMessage(messages[1])
		var content []byte
		if strings.Contains(userMessage, "approved_copy_variants") {
			content, _ = json.Marshal(visualResult)
		} else {
			copyRequests++
			if copyRequests == 1 {
				content, _ = json.Marshal(original)
			} else {
				if !strings.Contains(userMessage, "需要优化") {
					t.Fatalf("expected copy repair instruction, got %s", userMessage)
				}
				content, _ = json.Marshal(repaired)
			}
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
		ProductName:           "束裤带",
		VariantCount:          1,
		TargetDurationSeconds: 45,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if err != nil {
		t.Fatalf("below-preferred copy must remain usable after one repair: %v", err)
	}
	if requests != 3 || copyRequests != 2 {
		t.Fatalf("expected copy, repair, and visual requests; got requests=%d copy=%d", requests, copyRequests)
	}
	if len(result.Variants) != 1 || result.Variants[0].ScriptText != repaired.Variants[0].ScriptText {
		t.Fatalf("expected the closer repair to be used, got %#v", result)
	}
}

func TestMergeScriptCopyQualityRepairKeepsQualifiedVariantsUnchanged(t *testing.T) {
	input := ScriptGenerationInput{
		VariantCount:          2,
		TargetDurationSeconds: 45,
		SellingPoints: []ScriptGenerationSellingPoint{
			{Name: "避免蹭链条"},
			{Name: "方便收纳"},
		},
	}
	original := ScriptCopyResult{Variants: []ScriptCopyVariant{
		scriptCopyResultWithBodyLength(1, "骑行裤脚安全", "骑车裤脚容易蹭到链条", "避免蹭链条", "稳", 135).Variants[0],
		scriptCopyResultWithBodyLength(2, "随身便携收纳", "骑完以后束裤带方便收纳", "方便收纳", "收", 180).Variants[0],
	}}
	repaired := ScriptCopyResult{Variants: []ScriptCopyVariant{
		scriptCopyResultWithBodyLength(1, "骑行裤脚安全", "骑车裤脚容易蹭到链条", "避免蹭链条", "牢", 140).Variants[0],
		scriptCopyResultWithBodyLength(2, "被模型改写的合格版本", "模型改写了本来合格的版本", "方便收纳", "改", 185).Variants[0],
	}}
	if issues := validateScriptCopyVariantQualityIssues(2, original.Variants[1], input); len(issues) != 0 {
		t.Fatalf("second original variant must be qualified, got %#v", issues)
	}

	merged := mergeScriptCopyQualityRepair(original, repaired, input)
	if !reflect.DeepEqual(merged.Variants[0], repaired.Variants[0]) {
		t.Fatalf("expected the problem variant to use its closer repair, got %#v", merged.Variants[0])
	}
	if !reflect.DeepEqual(merged.Variants[1], original.Variants[1]) {
		t.Fatalf("qualified variant must remain unchanged, got %#v", merged.Variants[1])
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

func TestValidateScriptGenerationResultRejectsObviouslyShortCopy(t *testing.T) {
	result := validGatewayScriptResult()
	result.Variants[0].ScriptText = result.Variants[0].Hook + "，今天给大家推荐这款实用神器。"
	err := ValidateScriptGenerationResult(result, ScriptGenerationInput{VariantCount: 1, TargetDurationSeconds: 30})
	if err == nil || !strings.Contains(err.Error(), "minimum usable copy") {
		t.Fatalf("expected unusable-copy validation error, got %v", err)
	}
}

func TestScriptCopyQualityIssuesDetectProductionDirections(t *testing.T) {
	result := validScriptCopyResult()
	result.Variants[0].ScriptText = strings.Replace(result.Variants[0].ScriptText, "骑完卷好放进口袋", "最后拉上拉链，双手回到车把", 1)
	issues := validateScriptCopyQualityIssues(result, ScriptGenerationInput{
		VariantCount:          1,
		TargetDurationSeconds: 15,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	})
	if !strings.Contains(strings.Join(issues, "; "), "production-direction") {
		t.Fatalf("expected production-direction quality issue, got %#v", issues)
	}
}

func TestValidateScriptCopyResultAcceptsNaturalDurationWithoutQualityRepair(t *testing.T) {
	result := reasonableNearTargetCopyResult()
	input := ScriptGenerationInput{
		VariantCount:          1,
		TargetDurationSeconds: 30,
		SellingPoints:         []ScriptGenerationSellingPoint{{Name: "避免蹭链条"}},
	}
	estimatedDurationMs := EstimateScriptDurationMs(result.Variants[0].ScriptText)
	preferredMinimumMs, preferredMaximumMs := ScriptPreferredDurationRangeMs(30)
	if estimatedDurationMs < preferredMinimumMs || estimatedDurationMs > preferredMaximumMs {
		t.Fatalf("test copy duration %dms is outside the preferred boundaries", estimatedDurationMs)
	}
	if err := ValidateScriptCopyResult(result, input); err != nil {
		t.Fatalf("reasonable near-target copy must not fail generation: %v", err)
	}
	if issues := validateScriptCopyQualityIssues(result, input); len(issues) != 0 {
		t.Fatalf("natural duration must not trigger a rewrite, got %#v", issues)
	}
}

func TestDenseProductPitchSupportsAllSellingPointsAndVisualBeats(t *testing.T) {
	input, copies := denseHandlebarBagCopyFixture()
	if err := ValidateScriptCopyResult(copies, input); err != nil {
		t.Fatalf("dense product pitch must pass copy validation: %v", err)
	}
	if issues := validateScriptCopyQualityIssues(copies, input); len(issues) != 0 {
		t.Fatalf("dense product pitch must not trigger a quality rewrite: %#v", issues)
	}
	minimumBeats, maximumBeats := ScriptVisualBeatCountRange(30, len(copies.Variants[0].SelectedSellingPoints))
	if minimumBeats != 4 || maximumBeats != 9 {
		t.Fatalf("unexpected dense pitch beat range %d-%d", minimumBeats, maximumBeats)
	}
	prompt := BuildScriptVisualIntentPrompt(input, copies).Prompts[0].User
	if !strings.Contains(prompt, "beat_count_ranges") || !strings.Contains(prompt, `"maximum":9`) {
		t.Fatalf("visual prompt must allow one beat per selected selling point: %s", prompt)
	}

	visuals := ScriptVisualIntentResult{Plans: []ScriptVisualIntentPlan{{
		VariantIndex:  1,
		EditingIntent: "按防水、容量、固定、携带和夜间安全依次呈现产品卖点。",
		Beats: []ScriptGenerationBeat{
			{Label: "防水面料", SellingPoint: "防水面料", VisualGoal: "水珠落在车头包防水面料表面", SourceType: TTSVisualSourceType},
			{Label: "压胶拉链", SellingPoint: "压胶拉链", VisualGoal: "拉动车头包压胶拉链闭合袋口", SourceType: TTSVisualSourceType},
			{Label: "两升容量", SellingPoint: "2升大容量", VisualGoal: "多件骑行用品装入车头包主仓", SourceType: TTSVisualSourceType},
			{Label: "内部隔层", SellingPoint: "隔层设计", VisualGoal: "打开车头包展示内部收纳隔层", SourceType: TTSVisualSourceType},
			{Label: "三点固定", SellingPoint: "三点固定", VisualGoal: "三条绑带将车头包固定在车把上", SourceType: TTSVisualSourceType},
			{Label: "魔术贴", SellingPoint: "魔术贴安装", VisualGoal: "双手压紧车头包魔术贴绑带", SourceType: TTSVisualSourceType},
			{Label: "肩背携带", SellingPoint: "肩背设计", VisualGoal: "人物将车头包斜挎在身体侧面", SourceType: TTSVisualSourceType},
			{Label: "弹力外挂", SellingPoint: "侧边弹力绳", VisualGoal: "骑行物品固定在车头包侧边弹力绳", SourceType: TTSVisualSourceType},
			{Label: "夜间反光", SellingPoint: "反光标", VisualGoal: "车灯照射车头包反光标产生反光", SourceType: TTSVisualSourceType},
		},
	}}}
	if err := ValidateScriptVisualIntentResult(visuals, copies, input); err != nil {
		t.Fatalf("dense visual intent must pass validation: %v", err)
	}
	result, err := mergeScriptGenerationResult(copies, visuals)
	if err != nil {
		t.Fatalf("merge dense product pitch: %v", err)
	}
	if err := ValidateScriptGenerationResult(result, input); err != nil {
		t.Fatalf("dense product pitch must pass final validation: %v", err)
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
	if minimum != 111 || maximum != 127 {
		t.Fatalf("unexpected 30 second drafting range %d-%d", minimum, maximum)
	}
	preferredMinimum, preferredMaximum := ScriptPreferredDurationRangeMs(30)
	if preferredMinimum != 21000 || preferredMaximum != 42000 {
		t.Fatalf("unexpected 30 second preferred duration range %d-%d", preferredMinimum, preferredMaximum)
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

func reasonableNearTargetCopyResult() ScriptCopyResult {
	hook := strings.Repeat("骑", 15)
	clauses := []string{hook}
	for _, value := range []string{"行", "裤", "脚", "链", "条", "束", "带"} {
		clauses = append(clauses, strings.Repeat(value, 15))
	}
	return ScriptCopyResult{Variants: []ScriptCopyVariant{{
		VariantIndex:          1,
		Angle:                 "骑行裤脚安全",
		SelectedSellingPoints: []string{"避免蹭链条"},
		Hook:                  hook,
		ScriptText:            strings.Join(clauses, "，"),
	}}}
}

func scriptCopyResultWithBodyLength(index int, angle string, hook string, sellingPoint string, fill string, bodyLength int) ScriptCopyResult {
	return ScriptCopyResult{Variants: []ScriptCopyVariant{{
		VariantIndex:          index,
		Angle:                 angle,
		SelectedSellingPoints: []string{sellingPoint},
		Hook:                  hook,
		ScriptText:            hook + "，" + strings.Repeat(fill, bodyLength) + "。",
	}}}
}

func scriptVisualIntentResultForDuration(variantIndex int, sellingPoint string, targetDurationSeconds int) ScriptVisualIntentResult {
	minimumBeats, _ := ScriptVisualBeatCountRange(targetDurationSeconds, 1)
	beats := make([]ScriptGenerationBeat, 0, minimumBeats)
	for index := 0; index < minimumBeats; index++ {
		beats = append(beats, ScriptGenerationBeat{
			Label:        "动作阶段",
			SellingPoint: sellingPoint,
			VisualGoal:   fmt.Sprintf("束裤带固定裤脚的第%d个动作阶段", index+1),
			SourceType:   TTSVisualSourceType,
		})
	}
	return ScriptVisualIntentResult{Plans: []ScriptVisualIntentPlan{{
		VariantIndex:  variantIndex,
		EditingIntent: "依次呈现束裤带固定裤脚的完整使用过程。",
		Beats:         beats,
	}}}
}

func denseHandlebarBagCopyFixture() (ScriptGenerationInput, ScriptCopyResult) {
	names := []string{
		"防水面料", "压胶拉链", "2升大容量", "隔层设计", "三点固定",
		"魔术贴安装", "肩背设计", "侧边弹力绳", "反光标",
	}
	sellingPoints := make([]ScriptGenerationSellingPoint, 0, len(names))
	for _, name := range names {
		sellingPoints = append(sellingPoints, ScriptGenerationSellingPoint{Name: name})
	}
	return ScriptGenerationInput{
			ProductName:           "杜邦车包",
			VariantCount:          1,
			TargetDurationSeconds: 30,
			SellingPoints:         sellingPoints,
		}, ScriptCopyResult{Variants: []ScriptCopyVariant{{
			VariantIndex:          1,
			Angle:                 "卖点直给",
			SelectedSellingPoints: names,
			Hook:                  "还在找好用的骑行车头包？",
			ScriptText:            "还在找好用的骑行车头包？这款真的闭眼入！防水面料搭配压胶拉链，突发小雨不用慌。2升大容量，内带隔层。三点固定加魔术贴安装，牢牢固定不晃荡。自带肩带，骑完车直接变身斜挎包，一包两用。侧边弹力绳可外挂物品，反光标夜间骑行更安全！",
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
