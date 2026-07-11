package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredModelProviderService struct {
	Service *ModelProviderService
	Close   func()
}

func NewConfiguredModelProviderService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredModelProviderService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredModelProviderService{
			Service: NewModelProviderService(),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory model provider service", "error", err)
		return ConfiguredModelProviderService{
			Service: NewModelProviderService(),
			Close:   func() {},
		}
	}

	logger.Info("using postgres model provider service")
	return ConfiguredModelProviderService{
		Service: NewModelProviderServiceWithPool(pool),
		Close:   pool.Close,
	}
}
