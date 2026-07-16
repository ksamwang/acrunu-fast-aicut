package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type ConfiguredUserService struct {
	Service *UserService
	Close   func()
}

func NewConfiguredUserService(ctx context.Context, cfg config.Config, logger *slog.Logger) ConfiguredUserService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DatabaseURL == "" {
		return ConfiguredUserService{Service: NewUserService(cfg), Close: func() {}}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := repository.OpenPostgres(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("falling back to in-memory user service", "error", err)
		return ConfiguredUserService{Service: NewUserService(cfg), Close: func() {}}
	}

	logger.Info("using postgres user service")
	return ConfiguredUserService{
		Service: NewUserServiceWithQueries(db.New(pool)),
		Close:   pool.Close,
	}
}
