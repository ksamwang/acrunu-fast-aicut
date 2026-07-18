package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/httpserver"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	userService := services.NewConfiguredUserService(context.Background(), cfg, logger)
	defer userService.Close()
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
	subtitleStylePresetService := services.NewConfiguredSubtitleStylePresetService(context.Background(), cfg, logger)
	defer subtitleStylePresetService.Close()
	bgmTrackService := services.NewConfiguredBGMTrackService(context.Background(), cfg, logger)
	defer bgmTrackService.Close()
	scriptGenerationService := services.NewScriptGenerationService(
		productAssetService.Service,
		systemConfigService.Service,
		modelProviderService.Service,
		cfg,
	)
	scriptGenerationJobs := services.NewConfiguredScriptGenerationJobService(context.Background(), cfg, scriptGenerationService, logger)
	defer scriptGenerationJobs.Close()

	server := httpserver.New(httpserver.Options{
		Config:                     cfg,
		Logger:                     logger,
		UserService:                userService.Service,
		TaskService:                taskService.Service,
		SystemConfigService:        systemConfigService.Service,
		ModelProviderService:       modelProviderService.Service,
		ProductAssetService:        productAssetService.Service,
		AssetEmbeddingService:      assetEmbeddingService.Service,
		VoiceoverService:           voiceoverService.Service,
		ScriptGenerationService:    scriptGenerationService,
		ScriptGenerationJobService: scriptGenerationJobs.Service,
		GenerationRunService:       generationRunService.Service,
		SubtitleStylePresetService: subtitleStylePresetService.Service,
		BGMTrackService:            bgmTrackService.Service,
	})

	if err := server.Run(); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
