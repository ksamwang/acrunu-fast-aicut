//go:build !windows

package main

import (
	"log/slog"

	"github.com/ksamwang/acrunu-fast-aicut/internal/localagent"
)

func runLocalAgent(server *localagent.Server, _ *slog.Logger) error {
	return server.Run()
}
