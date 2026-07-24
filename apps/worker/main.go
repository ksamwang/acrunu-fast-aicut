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
	systemConfigService := services.NewConfiguredSystemConfigService(context.Background(), cfg, logger)
	defer systemConfigService.Close()
	modelProviderService := services.NewConfiguredModelProviderService(context.Background(), cfg, logger)
	defer modelProviderService.Close()
	productAssetService := services.NewConfiguredProductAssetService(context.Background(), cfg, logger)
	defer productAssetService.Close()
	scriptGenerationService := services.NewScriptGenerationService(
		productAssetService.Service,
		systemConfigService.Service,
		modelProviderService.Service,
		cfg,
	)
	scriptGenerationJobs := services.NewConfiguredScriptGenerationJobService(context.Background(), cfg, scriptGenerationService, logger)
	defer scriptGenerationJobs.Close()
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
	renderMaxConcurrency := 1
	if runtimeSettings, err := services.GetRuntimeSettings(systemConfigService.Service); err == nil && runtimeSettings.RenderMaxConcurrency > 0 {
		renderMaxConcurrency = runtimeSettings.RenderMaxConcurrency
	}
	queueClient := queue.NewClient(cfg.RedisAddr, cfg.QueueBackend, cfg.StorageRoot)
	defer queueClient.Close()
	analyzer := services.NewRuntimeAssetAnalyzer(systemConfigService.Service, modelProviderService.Service, cfg)
	assetProcessingService := services.NewAssetProcessingService(
		cfg.StorageRoot,
		productAssetService.Service,
		taskService.Service,
		queueClient,
		analyzer,
		logger,
	).WithAssetEmbeddingService(assetEmbeddingService.Service)
	renderService := services.NewGenerationRenderService(
		cfg.StorageRoot,
		generationRunService.Service,
		voiceoverService.Service,
		productAssetService.Service,
		renderMaxConcurrency,
		logger,
	)
	workerHandler := services.NewWorkerHandler(
		taskService.Service,
		assetProcessingService,
	).WithVoiceoverService(voiceoverService.Service).WithScriptGenerationJobs(scriptGenerationJobs.Service).WithGenerationPipeline(
		generationRunService.Service,
		services.NewGenerationPlanningService(
			generationRunService.Service,
			voiceoverService.Service,
			productAssetService.Service,
			assetCandidateService.Service,
			systemConfigService.Service,
			modelProviderService.Service,
			cfg,
		).WithLogger(logger),
		queueClient,
	).WithGenerationRenderer(renderService)
	if pendingJobIDs, err := scriptGenerationJobs.Service.PendingJobIDs(context.Background()); err != nil {
		logger.Error("failed to list pending script generation jobs", "error", err)
	} else {
		for _, jobID := range pendingJobIDs {
			if err := queueClient.EnqueueWorkbenchScriptGenerate(queue.WorkbenchScriptGeneratePayload{JobID: jobID}); err != nil {
				logger.Error("failed to requeue script generation job", "job_id", jobID, "error", err)
			}
		}
	}
	if err := workerHandler.EnqueuePendingRenders(context.Background()); err != nil {
		logger.Error("failed to enqueue pending generation renders", "error", err)
	}

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
