package main

import (
	"log/slog"
	"os"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/localagent"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	server := localagent.New(localagent.Options{
		Addr:   cfg.LocalAgentAddr,
		Logger: logger,
	})

	if err := server.Run(); err != nil {
		logger.Error("local agent stopped", "error", err)
		os.Exit(1)
	}
}
