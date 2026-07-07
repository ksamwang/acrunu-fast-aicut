package main

import (
	"log/slog"
	"os"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	taskService := services.NewTaskService(cfg.StorageRoot)

	server := queue.NewServer(cfg.RedisAddr, cfg.WorkerConcurrency)
	mux := queue.NewServeMux(taskService)

	logger.Info("worker starting", "redis", cfg.RedisAddr, "concurrency", cfg.WorkerConcurrency)
	if err := server.Run(mux); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
