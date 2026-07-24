package services

import (
	"context"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type runtimeAssetAnalyzer struct {
	systemConfigService  *SystemConfigService
	modelProviderService *ModelProviderService
	fallbackConfig       config.Config
}

func NewRuntimeAssetAnalyzer(systemConfigService *SystemConfigService, modelProviderService *ModelProviderService, fallbackConfig config.Config) modelgateway.AssetAnalyzer {
	return &runtimeAssetAnalyzer{
		systemConfigService:  systemConfigService,
		modelProviderService: modelProviderService,
		fallbackConfig:       fallbackConfig,
	}
}

func (a *runtimeAssetAnalyzer) AnalyzeAsset(ctx context.Context, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	cfg := ResolveVLMAnalyzerConfigWithProviders(ctx, a.systemConfigService, a.modelProviderService, a.fallbackConfig)
	return modelgateway.NewAnalyzer(cfg, nil).AnalyzeAsset(ctx, input)
}
