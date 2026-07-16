package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredGenerationRunService struct {
	Service *GenerationRunService
	Close   func()
}

func NewConfiguredGenerationRunService(ctx context.Context, cfg config.Config, voiceovers *VoiceoverService, logger *slog.Logger) ConfiguredGenerationRunService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredGenerationRunService{
			Service: NewGenerationRunService(voiceovers),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory generation run service", "error", err)
		return ConfiguredGenerationRunService{
			Service: NewGenerationRunService(voiceovers),
			Close:   func() {},
		}
	}

	logger.Info("using postgres generation run service")
	return ConfiguredGenerationRunService{
		Service: NewGenerationRunServiceWithPool(pool, voiceovers),
		Close:   pool.Close,
	}
}
