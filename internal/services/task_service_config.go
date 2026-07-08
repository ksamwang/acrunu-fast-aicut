package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type ConfiguredTaskService struct {
	Service *TaskService
	Close   func()
}

func NewConfiguredTaskService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredTaskService {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.DatabaseURL == "" {
		return ConfiguredTaskService{
			Service: NewTaskService(cfg.StorageRoot),
			Close:   func() {},
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to file task store", "error", err)
		return ConfiguredTaskService{
			Service: NewTaskService(cfg.StorageRoot),
			Close:   func() {},
		}
	}

	logger.Info("using postgres task store")
	return ConfiguredTaskService{
		Service: NewTaskServiceWithStore(NewPostgresTaskStore(db.New(pool))),
		Close:   pool.Close,
	}
}
