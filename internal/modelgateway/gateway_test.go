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

func TestBuildPromptBundleUsesProductNameOnlyForVisualOnly(t *testing.T) {
	visualBundle := BuildPromptBundle(AnalyzeAssetInput{
		AssetID:     "asset-1",
		SourceType:  "visual_only",
		ProductName: "车载氛围灯",
	})
	if !strings.Contains(visualBundle.Prompts[0].User, "Target product name") {
		t.Fatalf("expected visual_only prompt to include product context, got %s", visualBundle.Prompts[0].User)
	}
	if !strings.Contains(visualBundle.Prompts[0].User, "visible_product means the target product is visible") {
		t.Fatalf("expected target product visibility rule, got %s", visualBundle.Prompts[0].User)
	}
	for _, expected := range []string{"retrieval summaries, not exhaustive inventories", "make it the subject of scene_description", "normally no more than 50 Chinese characters"} {
		if !strings.Contains(visualBundle.Prompts[0].User, expected) {
			t.Fatalf("expected product-centered description rule %q, got %s", expected, visualBundle.Prompts[0].User)
		}
	}

	talkingHeadBundle := BuildPromptBundle(AnalyzeAssetInput{
		AssetID:     "asset-2",
		SourceType:  "talking_head",
		ProductName: "车载氛围灯",
	})
	if strings.Contains(talkingHeadBundle.Prompts[0].User, "product_name") {
		t.Fatalf("expected talking_head prompt not to include product context, got %s", talkingHeadBundle.Prompts[0].User)
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
