package modelgateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeAssetOutputSchemaContainsRequiredFields(t *testing.T) {
	schema := AnalyzeAssetOutputSchema()
	if schema["version"] != OutputSchemaVersion {
		t.Fatalf("expected schema version %s, got %#v", OutputSchemaVersion, schema["version"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) == 0 {
		t.Fatalf("expected required fields, got %#v", schema["required"])
	}
}

func TestBuildPromptBundleIncludesPromptSections(t *testing.T) {
	bundle := BuildPromptBundle(AnalyzeAssetInput{
		AssetID:    "asset-1",
		SourceType: "talking_head",
		DurationMs: 3200,
		Width:      1080,
		Height:     1920,
		HasAudio:   true,
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 500, StorageKey: "frames/a.jpg"},
		},
	})

	if bundle.Version != PromptVersion {
		t.Fatalf("expected prompt version %s, got %s", PromptVersion, bundle.Version)
	}
	if len(bundle.Prompts) != 1 {
		t.Fatalf("expected one prompt section, got %d", len(bundle.Prompts))
	}
	if !strings.Contains(bundle.Prompts[0].User, "asset_id=asset-1") {
		t.Fatalf("expected prompt context to include asset id, got %s", bundle.Prompts[0].User)
	}
	if !strings.Contains(bundle.Prompts[0].User, "frames are ordered chronologically") {
		t.Fatalf("expected prompt to define chronological frame order, got %s", bundle.Prompts[0].User)
	}
	if !strings.Contains(bundle.Prompts[0].User, "frame_timestamps_ms=[500]") {
		t.Fatalf("expected prompt to include frame timestamps, got %s", bundle.Prompts[0].User)
	}
	if !strings.Contains(bundle.Prompts[0].User, "push_in, pull_out, tracking") {
		t.Fatalf("expected prompt to include revised camera movement values, got %s", bundle.Prompts[0].User)
	}
}

func TestBuildPromptBundleGroundsProductAndCandidateSellingPoints(t *testing.T) {
	visualBundle := BuildPromptBundle(AnalyzeAssetInput{
		AssetID:     "asset-1",
		SourceType:  "visual_only",
		ProductName: "车载氛围灯",
		CandidateSellingPoints: []SellingPointContext{
			{Title: "自动亮灯", Description: "开门后自动点亮"},
		},
	})
	if !strings.Contains(visualBundle.Prompts[0].User, "Target product name") {
		t.Fatalf("expected visual_only prompt to include product context, got %s", visualBundle.Prompts[0].User)
	}
	if !strings.Contains(visualBundle.Prompts[0].User, "visible_product is based only on whether the target product is actually visible") {
		t.Fatalf("expected target product visibility rule, got %s", visualBundle.Prompts[0].User)
	}
	for _, expected := range []string{
		"retrieval summaries, not exhaustive inventories",
		"target product identity is authoritative",
		"exact supplied product name",
		"Internally distinguish product identity",
		"visible state",
		"temporal action",
		"visible evidence",
		"compact semantic index",
		"automatic editor's visual_goal",
		"initial state -> visible operation -> visible result",
		"Inspect every ordered frame",
		"write only the concrete visible product state",
		"visual_tags must contain only 3 to 6 retrieval terms",
		"directly demonstrated by the ordered frames",
		"zero or one candidate selling point",
		"positive demonstration or negative pain point",
		"Candidate product selling points",
		"自动亮灯",
		"visible_product must remain false",
		"people_presence must be true whenever any human body part is visible",
		"normally no more than 40 Chinese characters",
	} {
		if !strings.Contains(visualBundle.Prompts[0].User, expected) {
			t.Fatalf("expected product-centered description rule %q, got %s", expected, visualBundle.Prompts[0].User)
		}
	}
	if strings.Contains(visualBundle.Prompts[0].User, "无明显操作，展示") {
		t.Fatalf("expected phase2-v7 prompt to remove generic no-operation output, got %s", visualBundle.Prompts[0].User)
	}

	talkingHeadBundle := BuildPromptBundle(AnalyzeAssetInput{
		AssetID:     "asset-2",
		SourceType:  "talking_head",
		ProductName: "车载氛围灯",
	})
	if !strings.Contains(talkingHeadBundle.Prompts[0].User, "Target product name") {
		t.Fatalf("expected talking_head prompt to retain product context, got %s", talkingHeadBundle.Prompts[0].User)
	}
}

func TestValidateAnalyzeAssetResultRejectsInvalidValues(t *testing.T) {
	err := ValidateAnalyzeAssetResult(AnalyzeAssetResult{
		SceneDescription: "",
		ShotSize:         "close_up",
		CameraMovement:   "static",
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != ErrorCodeInvalidResponse {
		t.Fatalf("expected invalid response error, got %v", err)
	}
}

func TestValidateAnalyzeAssetResultRejectsContradictoryHumanMetadata(t *testing.T) {
	result := AnalyzeAssetResult{
		SceneDescription:  "人物佩戴杜邦车包，包体完整清晰可见",
		ShotSize:          "medium_close_up",
		CameraMovement:    "static",
		VisualTags:        []string{"杜邦车包", "斜挎"},
		VisibleProduct:    true,
		ProductPosition:   "人物腰侧",
		SceneContext:      "户外",
		ActionDescription: "视频持续展示人物佩戴的杜邦车包，未见位置变化",
		PeoplePresence:    false,
		FaceVisible:       false,
		LightingCondition: "自然光",
	}

	err := ValidateAnalyzeAssetResult(result)
	if err == nil {
		t.Fatalf("expected low-value and contradictory result to be rejected")
	}
	if !strings.Contains(err.Error(), "people_presence must be true") {
		t.Fatalf("expected validation error to include human metadata contradiction, got %v", err)
	}
}

func TestAnalyzeAssetResultGenericPhrasesAreRepairIssuesNotValidationErrors(t *testing.T) {
	result := AnalyzeAssetResult{
		SceneDescription:  "杜邦车包固定在车把上，包体清晰可见",
		ShotSize:          "close_up",
		CameraMovement:    "static",
		VisualTags:        []string{"杜邦车包", "车把安装"},
		VisibleProduct:    true,
		ProductPosition:   "车把前方",
		SceneContext:      "户外",
		ActionDescription: "无明显操作，持续展示车把安装状态",
		PeoplePresence:    false,
		FaceVisible:       false,
		LightingCondition: "自然光",
	}

	if err := ValidateAnalyzeAssetResult(result); err != nil {
		t.Fatalf("expected generic wording not to discard valid analysis, got %v", err)
	}
	issues := strings.Join(analyzeAssetResultRepairIssues(result), " ")
	for _, expected := range []string{"清晰可见", "无明显操作"} {
		if !strings.Contains(issues, expected) {
			t.Fatalf("expected repair issues to include %q, got %s", expected, issues)
		}
	}
}

func TestValidateAnalyzeAssetResultAcceptsConcreteStaticRetrievalDescription(t *testing.T) {
	err := ValidateAnalyzeAssetResult(AnalyzeAssetResult{
		SceneDescription:  "杜邦车包斜挎在人物腰侧，展示贴身携带效果",
		ShotSize:          "medium_close_up",
		CameraMovement:    "static",
		VisualTags:        []string{"杜邦车包", "斜挎携带"},
		VisibleProduct:    true,
		ProductPosition:   "人物腰侧",
		SceneContext:      "户外骑行场景",
		ActionDescription: "斜挎贴合腰侧的佩戴状态",
		PeoplePresence:    true,
		FaceVisible:       false,
		LightingCondition: "自然光",
	})
	if err != nil {
		t.Fatalf("expected concrete static retrieval description to pass, got %v", err)
	}
}

func TestNewAnalyzerAppliesRetryAndValidation(t *testing.T) {
	attempts := 0
	analyzer := NewAnalyzer(Config{
		Provider:   "mock",
		MaxRetries: 2,
	}, analyzerFunc(func(context.Context, AnalyzeAssetInput) (AnalyzeAssetResult, error) {
		attempts++
		if attempts == 1 {
			return AnalyzeAssetResult{}, errors.New("temporary upstream failure")
		}
		return AnalyzeAssetResult{
			SceneDescription:  "product close-up",
			ShotSize:          "close_up",
			CameraMovement:    "static",
			VisualTags:        []string{"product", "demo"},
			QualityTags:       []string{},
			VisibleProduct:    true,
			ProductPosition:   "center",
			SceneContext:      "demo",
			ActionDescription: "product shown",
			LightingCondition: "normal",
			ModelResult:       map[string]any{"provider": "mock"},
		}, nil
	}))

	result, err := analyzer.AnalyzeAsset(context.Background(), AnalyzeAssetInput{AssetID: "asset-1"})
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected one retry, got %d attempts", attempts)
	}
	if result.ModelResult["schema_version"] != OutputSchemaVersion {
		t.Fatalf("expected schema version to be injected, got %#v", result.ModelResult)
	}
}

func TestNewAnalyzerNormalizesTimeoutError(t *testing.T) {
	analyzer := NewAnalyzer(Config{
		Provider: "mock",
		Timeout:  10 * time.Millisecond,
	}, analyzerFunc(func(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
		<-ctx.Done()
		return AnalyzeAssetResult{}, ctx.Err()
	}))

	_, err := analyzer.AnalyzeAsset(context.Background(), AnalyzeAssetInput{AssetID: "asset-1"})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != ErrorCodeTimeout {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestNewAnalyzerRejectsUnsupportedProvider(t *testing.T) {
	analyzer := NewAnalyzer(Config{Provider: "unknown"}, nil)

	_, err := analyzer.AnalyzeAsset(context.Background(), AnalyzeAssetInput{AssetID: "asset-1"})
	if err == nil {
		t.Fatalf("expected unsupported provider error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != ErrorCodeUnsupportedProvider {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}
