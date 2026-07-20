package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/localagent"
)

var (
	version   = "dev"
	buildMode = "development"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	workspaceRoot, err := parseWorkspaceRoot()
	if err != nil {
		return err
	}
	logger, closer, err := newLogger()
	if err != nil {
		return err
	}
	defer closer.Close()

	cfg := config.Load()
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(cfg.StorageRoot, "local-agent-workspace")
	}

	server := localagent.New(localagent.Options{
		Addr:          cfg.LocalAgentAddr,
		Logger:        logger,
		WorkspaceRoot: workspaceRoot,
		Processor:     localagent.NewDefaultProcessor(),
		AppVersion:    version,
	})

	logger.Info("local agent starting", "version", version, "workspace_root", workspaceRoot)
	return runLocalAgent(server, logger)
}

func parseWorkspaceRoot() (string, error) {
	flags := flag.NewFlagSet("local-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	workspaceRoot := flags.String("workspace-root", "", "local workspace directory")
	protocolURL := flags.String("protocol", "", "custom protocol invocation")
	_ = flags.Bool("autostart", false, "started at Windows sign-in")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return "", fmt.Errorf("parse local agent arguments: %w", err)
	}
	if *protocolURL != "" && !validProtocolURL(*protocolURL) {
		return "", fmt.Errorf("unsupported protocol action")
	}
	if strings.TrimSpace(*workspaceRoot) != "" {
		return filepath.Clean(*workspaceRoot), nil
	}
	if buildMode != "installer" {
		return "", nil
	}
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not available")
	}
	return filepath.Join(localAppData, "ACRUNU", "FastCut", "workspace"), nil
}

func validProtocolURL(value string) bool {
	normalized := strings.TrimRight(strings.ToLower(strings.TrimSpace(value)), "/")
	return normalized == "acrunu-fastcut://launch"
}

func newLogger() (*slog.Logger, io.Closer, error) {
	if buildMode != "installer" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil)), io.NopCloser(strings.NewReader("")), nil
	}
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		return nil, nil, fmt.Errorf("LOCALAPPDATA is not available")
	}
	logDir := filepath.Join(root, "ACRUNU", "FastCut", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create local agent log directory: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "local-agent.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open local agent log: %w", err)
	}
	return slog.New(slog.NewJSONHandler(logFile, nil)), logFile, nil
}
