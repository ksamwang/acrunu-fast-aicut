package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredVoiceoverService struct {
	Service *VoiceoverService
	Close   func()
}

func NewConfiguredVoiceoverService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredVoiceoverService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredVoiceoverService{
			Service: NewVoiceoverService(cfg.StorageRoot, cfg, logger),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory voiceover service", "error", err)
		return ConfiguredVoiceoverService{
			Service: NewVoiceoverService(cfg.StorageRoot, cfg, logger),
			Close:   func() {},
		}
	}

	logger.Info("using postgres voiceover service")
	return ConfiguredVoiceoverService{
		Service: NewVoiceoverServiceWithPool(cfg.StorageRoot, cfg, logger, pool),
		Close:   pool.Close,
	}
}
