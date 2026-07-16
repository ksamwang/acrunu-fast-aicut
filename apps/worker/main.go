package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	taskService := services.NewConfiguredTaskService(context.Background(), cfg, logger)
	defer taskService.Close()
	systemConfigService := services.NewConfiguredSystemConfigService(context.Background(), cfg, logger)
	defer systemConfigService.Close()
	modelProviderService := services.NewConfiguredModelProviderService(context.Background(), cfg, logger)
	defer modelProviderService.Close()
	productAssetService := services.NewConfiguredProductAssetService(context.Background(), cfg, logger)
	defer productAssetService.Close()
	assetEmbeddingService := services.NewConfiguredAssetEmbeddingService(context.Background(), cfg, productAssetService.Service, systemConfigService.Service, modelProviderService.Service, logger)
	defer assetEmbeddingService.Close()
	voiceoverService := services.NewConfiguredVoiceoverService(context.Background(), cfg, logger)
	defer voiceoverService.Close()
	generationRunService := services.NewConfiguredGenerationRunService(context.Background(), cfg, voiceoverService.Service, logger)
	defer generationRunService.Close()
	assetCandidateService := services.NewConfiguredAssetCandidateService(
		context.Background(),
		cfg,
		productAssetService.Service,
		systemConfigService.Service,
		modelProviderService.Service,
		logger,
	)
	defer assetCandidateService.Close()
	queueClient := queue.NewClient(cfg.RedisAddr, cfg.QueueBackend, cfg.StorageRoot)
	defer queueClient.Close()
	analyzer := modelgateway.NewAnalyzer(services.ResolveVLMAnalyzerConfigWithProviders(context.Background(), systemConfigService.Service, modelProviderService.Service, cfg), nil)
	assetProcessingService := services.NewAssetProcessingService(
		cfg.StorageRoot,
		productAssetService.Service,
		taskService.Service,
		queueClient,
		analyzer,
		logger,
	).WithAssetEmbeddingService(assetEmbeddingService.Service)
	workerHandler := services.NewWorkerHandler(
		taskService.Service,
		assetProcessingService,
	).WithVoiceoverService(voiceoverService.Service).WithGenerationPipeline(
		generationRunService.Service,
		services.NewGenerationPlanningService(
			generationRunService.Service,
			voiceoverService.Service,
			productAssetService.Service,
			assetCandidateService.Service,
			systemConfigService.Service,
			modelProviderService.Service,
			cfg,
		),
		queueClient,
	)

	if cfg.QueueBackend == "file" {
		logger.Info("worker starting", "queue_backend", cfg.QueueBackend, "storage_root", cfg.StorageRoot)
		if err := queue.RunFileWorker(context.Background(), cfg.StorageRoot, workerHandler, 500*time.Millisecond); err != nil {
			logger.Error("worker stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	server := queue.NewServer(cfg.RedisAddr, cfg.WorkerConcurrency)
	mux := queue.NewServeMux(workerHandler)

	logger.Info("worker starting", "queue_backend", cfg.QueueBackend, "redis", cfg.RedisAddr, "concurrency", cfg.WorkerConcurrency)
	if err := server.Run(mux); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
