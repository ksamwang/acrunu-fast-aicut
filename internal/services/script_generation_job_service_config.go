package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
)

type ConfiguredScriptGenerationJobService struct {
	Service *ScriptGenerationJobService
	Close   func()
}

func NewConfiguredScriptGenerationJobService(
	ctx context.Context,
	cfg config.Config,
	generator *ScriptGenerationService,
	logger *slog.Logger,
) ConfiguredScriptGenerationJobService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredScriptGenerationJobService{
			Service: NewScriptGenerationJobService(nil, generator),
			Close:   func() {},
		}
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory script generation jobs", "error", err)
		return ConfiguredScriptGenerationJobService{
			Service: NewScriptGenerationJobService(nil, generator),
			Close:   func() {},
		}
	}
	logger.Info("using postgres script generation jobs")
	return ConfiguredScriptGenerationJobService{
		Service: NewScriptGenerationJobService(pool, generator),
		Close:   pool.Close,
	}
}
