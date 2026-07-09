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
	if len(bundle.Prompts) != 4 {
		t.Fatalf("expected four prompt sections, got %d", len(bundle.Prompts))
	}
	if !strings.Contains(bundle.Prompts[0].User, "asset_id=asset-1") {
		t.Fatalf("expected prompt context to include asset id, got %s", bundle.Prompts[0].User)
	}
}

func TestValidateAnalyzeAssetResultRejectsInvalidValues(t *testing.T) {
	err := ValidateAnalyzeAssetResult(AnalyzeAssetResult{
		UsabilityStatus:  "bad-status",
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
			UsabilityStatus:  "usable",
			SceneDescription: "product close-up",
			ShotSize:         "close_up",
			CameraMovement:   "static",
			Subjects:         []string{"product"},
			SceneTags:        []string{"demo"},
			QualityTags:      []string{},
			ModelResult:      map[string]any{"provider": "mock"},
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
	analyzer := NewAnalyzer(Config{Provider: "openai_compatible"}, nil)

	_, err := analyzer.AnalyzeAsset(context.Background(), AnalyzeAssetInput{AssetID: "asset-1"})
	if err == nil {
		t.Fatalf("expected unsupported provider error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != ErrorCodeUnsupportedProvider {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}
