package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	taskService := services.NewConfiguredTaskService(context.Background(), cfg, logger)
	defer taskService.Close()

	if cfg.QueueBackend == "file" {
		logger.Info("worker starting", "queue_backend", cfg.QueueBackend, "storage_root", cfg.StorageRoot)
		if err := queue.RunFileWorker(context.Background(), cfg.StorageRoot, taskService.Service, 500*time.Millisecond); err != nil {
			logger.Error("worker stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	server := queue.NewServer(cfg.RedisAddr, cfg.WorkerConcurrency)
	mux := queue.NewServeMux(taskService.Service)

	logger.Info("worker starting", "queue_backend", cfg.QueueBackend, "redis", cfg.RedisAddr, "concurrency", cfg.WorkerConcurrency)
	if err := server.Run(mux); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
