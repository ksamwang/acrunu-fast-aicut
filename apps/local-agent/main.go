package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/localagent"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	server := localagent.New(localagent.Options{
		Addr:          cfg.LocalAgentAddr,
		Logger:        logger,
		WorkspaceRoot: filepath.Join(cfg.StorageRoot, "local-agent-workspace"),
		Processor:     localagent.NewDefaultProcessor(),
	})

	if err := server.Run(); err != nil {
		logger.Error("local agent stopped", "error", err)
		os.Exit(1)
	}
}
