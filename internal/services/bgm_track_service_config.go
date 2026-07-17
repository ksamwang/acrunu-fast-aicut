package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredBGMTrackService struct {
	Service *BGMTrackService
	Close   func()
}

func NewConfiguredBGMTrackService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredBGMTrackService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredBGMTrackService{Service: NewBGMTrackService(cfg.StorageRoot), Close: func() {}}
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory bgm track service", "error", err)
		return ConfiguredBGMTrackService{Service: NewBGMTrackService(cfg.StorageRoot), Close: func() {}}
	}
	logger.Info("using postgres bgm track service")
	return ConfiguredBGMTrackService{Service: NewBGMTrackServiceWithPool(pool, cfg.StorageRoot), Close: pool.Close}
}
