package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type ConfiguredSystemConfigService struct {
	Service *SystemConfigService
	Close   func()
}

func NewConfiguredSystemConfigService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredSystemConfigService {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.DatabaseURL == "" {
		return ConfiguredSystemConfigService{
			Service: NewSystemConfigService(),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory system config service", "error", err)
		return ConfiguredSystemConfigService{
			Service: NewSystemConfigService(),
			Close:   func() {},
		}
	}

	service, err := NewSystemConfigServiceWithQueries(db.New(pool))
	if err != nil {
		logger.Warn("falling back to in-memory system config service", "error", err)
		pool.Close()
		return ConfiguredSystemConfigService{
			Service: NewSystemConfigService(),
			Close:   func() {},
		}
	}

	logger.Info("using postgres system config service")
	return ConfiguredSystemConfigService{
		Service: service,
		Close:   pool.Close,
	}
}
