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
	taskService := services.NewConfiguredTaskService(context.Background(), cfg, logger)
	defer taskService.Close()
	systemConfigService := services.NewConfiguredSystemConfigService(context.Background(), cfg, logger)
	defer systemConfigService.Close()
	modelProviderService := services.NewConfiguredModelProviderService(context.Background(), cfg, logger)
	defer modelProviderService.Close()
	productAssetService := services.NewConfiguredProductAssetService(context.Background(), cfg, logger)
	defer productAssetService.Close()

	server := httpserver.New(httpserver.Options{
		Config:               cfg,
		Logger:               logger,
		TaskService:          taskService.Service,
		SystemConfigService:  systemConfigService.Service,
		ModelProviderService: modelProviderService.Service,
		ProductAssetService:  productAssetService.Service,
	})

	if err := server.Run(); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
