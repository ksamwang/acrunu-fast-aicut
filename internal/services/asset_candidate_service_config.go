package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredAssetCandidateService struct {
	Service *AssetCandidateService
	Close   func()
}

func NewConfiguredAssetCandidateService(
	ctx context.Context,
	cfg config.Config,
	productAssets *ProductAssetService,
	systemConfigs *SystemConfigService,
	modelProviders *ModelProviderService,
	logger *slog.Logger,
) ConfiguredAssetCandidateService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredAssetCandidateService{
			Service: NewAssetCandidateService(nil, productAssets, systemConfigs, modelProviders, cfg),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("asset candidate service disabled", "error", err)
		return ConfiguredAssetCandidateService{
			Service: NewAssetCandidateService(nil, productAssets, systemConfigs, modelProviders, cfg),
			Close:   func() {},
		}
	}

	logger.Info("using postgres asset candidate service")
	return ConfiguredAssetCandidateService{
		Service: NewAssetCandidateService(pool, productAssets, systemConfigs, modelProviders, cfg),
		Close:   pool.Close,
	}
}
