package modelgateway

import (
	"context"
	"fmt"
	"time"
)

type Config struct {
	Provider   string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

type analyzerFunc func(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error)

func (f analyzerFunc) AnalyzeAsset(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
	return f(ctx, input)
}

func NewAnalyzer(cfg Config, base AssetAnalyzer) AssetAnalyzer {
	provider := cfg.Provider
	if provider == "" {
		provider = "mock"
	}

	if base == nil {
		switch provider {
		case "mock":
			base = NewMockAssetAnalyzer()
		default:
			base = analyzerFunc(func(context.Context, AnalyzeAssetInput) (AnalyzeAssetResult, error) {
				return AnalyzeAssetResult{}, NewError(
					ErrorCodeUnsupportedProvider,
					fmt.Sprintf("provider %q is configured but not implemented", provider),
					false,
					nil,
				)
			})
		}
	}

	analyzer := base
	if cfg.Timeout > 0 {
		analyzer = withTimeout(analyzer, cfg.Timeout)
	}
	if cfg.MaxRetries > 0 {
		analyzer = withRetry(analyzer, cfg.MaxRetries)
	}
	return withValidation(analyzer)
}

func withTimeout(next AssetAnalyzer, timeout time.Duration) AssetAnalyzer {
	return analyzerFunc(func(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		result, err := next.AnalyzeAsset(runCtx, input)
		if err != nil {
			return AnalyzeAssetResult{}, NormalizeError(err)
		}
		return result, nil
	})
}

func withRetry(next AssetAnalyzer, maxRetries int) AssetAnalyzer {
	return analyzerFunc(func(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
		var lastErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			result, err := next.AnalyzeAsset(ctx, input)
			if err == nil {
				return result, nil
			}
			lastErr = NormalizeError(err)
			if !IsRetryableError(lastErr) {
				break
			}
		}
		return AnalyzeAssetResult{}, lastErr
	})
}

func withValidation(next AssetAnalyzer) AssetAnalyzer {
	return analyzerFunc(func(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
		result, err := next.AnalyzeAsset(ctx, input)
		if err != nil {
			return AnalyzeAssetResult{}, NormalizeError(err)
		}
		if err := ValidateAnalyzeAssetResult(result); err != nil {
			return AnalyzeAssetResult{}, NormalizeError(err)
		}
		if result.ModelResult == nil {
			result.ModelResult = map[string]any{}
		}
		result.ModelResult["schema_version"] = OutputSchemaVersion
		result.ModelResult["prompt_version"] = PromptVersion
		return result, nil
	})
}
