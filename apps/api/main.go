package main

import (
	"log/slog"
	"os"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/httpserver"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	server := httpserver.New(httpserver.Options{
		Config: cfg,
		Logger: logger,
	})

	if err := server.Run(); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
