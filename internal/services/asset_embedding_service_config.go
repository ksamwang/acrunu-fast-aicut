package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredAssetEmbeddingService struct {
	Service *AssetEmbeddingService
	Close   func()
}

func NewConfiguredAssetEmbeddingService(ctx context.Context, cfg config.Config, productAssetService *ProductAssetService, systemConfigService *SystemConfigService, modelProviderService *ModelProviderService, logger *slog.Logger) ConfiguredAssetEmbeddingService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredAssetEmbeddingService{
			Service: NewAssetEmbeddingService(nil, productAssetService, systemConfigService, modelProviderService, cfg),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("asset embedding service disabled", "error", err)
		return ConfiguredAssetEmbeddingService{
			Service: NewAssetEmbeddingService(nil, productAssetService, systemConfigService, modelProviderService, cfg),
			Close:   func() {},
		}
	}

	logger.Info("using postgres asset embedding service")
	return ConfiguredAssetEmbeddingService{
		Service: NewAssetEmbeddingService(pool, productAssetService, systemConfigService, modelProviderService, cfg),
		Close:   pool.Close,
	}
}
