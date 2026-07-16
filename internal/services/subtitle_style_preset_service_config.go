package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredSubtitleStylePresetService struct {
	Service *SubtitleStylePresetService
	Close   func()
}

func NewConfiguredSubtitleStylePresetService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredSubtitleStylePresetService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredSubtitleStylePresetService{Service: NewSubtitleStylePresetService(), Close: func() {}}
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory subtitle style preset service", "error", err)
		return ConfiguredSubtitleStylePresetService{Service: NewSubtitleStylePresetService(), Close: func() {}}
	}
	logger.Info("using postgres subtitle style preset service")
	return ConfiguredSubtitleStylePresetService{Service: NewSubtitleStylePresetServiceWithPool(pool), Close: pool.Close}
}
