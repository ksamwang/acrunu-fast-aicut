package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type ConfiguredProductAssetService struct {
	Service *ProductAssetService
	Close   func()
}

func NewConfiguredProductAssetService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredProductAssetService {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.DatabaseURL == "" {
		return ConfiguredProductAssetService{
			Service: NewProductAssetService(),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory product asset service", "error", err)
		return ConfiguredProductAssetService{
			Service: NewProductAssetService(),
			Close:   func() {},
		}
	}

	logger.Info("using postgres product asset service")
	return ConfiguredProductAssetService{
		Service: NewProductAssetServiceWithQueries(db.New(pool)),
		Close:   pool.Close,
	}
}
