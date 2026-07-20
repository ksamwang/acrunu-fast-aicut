package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const localAgentReleaseRelativeDir = "local-agent/windows-x64"

type localAgentReleaseManifest struct {
	Version         string `json:"version"`
	Platform        string `json:"platform"`
	ProtocolVersion int    `json:"protocol_version"`
	SHA256          string `json:"sha256"`
	Filename        string `json:"filename"`
}

type localAgentReleaseResponse struct {
	Version         string `json:"version"`
	Platform        string `json:"platform"`
	ProtocolVersion int    `json:"protocol_version"`
	SHA256          string `json:"sha256"`
	DownloadURL     string `json:"download_url"`
}

func (s *Server) handleGetLatestLocalAgentRelease(c *gin.Context) {
	manifest, _, err := s.loadLatestLocalAgentRelease()
	if err != nil {
		s.logger.Error("load local agent release", "error", err)
		Fail(c, http.StatusNotFound, "local_agent_release_unavailable", "Local Agent 安装包暂不可用")
		return
	}
	c.Header("Cache-Control", "no-store")
	OK(c, localAgentReleaseResponse{
		Version:         manifest.Version,
		Platform:        manifest.Platform,
		ProtocolVersion: manifest.ProtocolVersion,
		SHA256:          manifest.SHA256,
		DownloadURL:     "/api/client-releases/local-agent/download",
	})
}

func (s *Server) handleDownloadLatestLocalAgentRelease(c *gin.Context) {
	manifest, installerPath, err := s.loadLatestLocalAgentRelease()
	if err != nil {
		s.logger.Error("download local agent release", "error", err)
		Fail(c, http.StatusNotFound, "local_agent_release_unavailable", "Local Agent 安装包暂不可用")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.FileAttachment(installerPath, manifest.Filename)
}

func (s *Server) loadLatestLocalAgentRelease() (localAgentReleaseManifest, string, error) {
	releaseRoot := strings.TrimSpace(s.cfg.ClientReleaseRoot)
	if releaseRoot == "" {
		releaseRoot = filepath.Join(s.cfg.StorageRoot, "client-releases")
	}
	releaseDir := filepath.Join(releaseRoot, filepath.FromSlash(localAgentReleaseRelativeDir))
	manifestPath := filepath.Join(releaseDir, "release.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return localAgentReleaseManifest{}, "", fmt.Errorf("read release manifest: %w", err)
	}
	var manifest localAgentReleaseManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return localAgentReleaseManifest{}, "", fmt.Errorf("decode release manifest: %w", err)
	}
	manifest.Filename = strings.TrimSpace(manifest.Filename)
	if manifest.Version == "" || manifest.Platform != "windows-x64" || manifest.ProtocolVersion < 1 ||
		manifest.Filename == "" || filepath.Base(manifest.Filename) != manifest.Filename {
		return localAgentReleaseManifest{}, "", fmt.Errorf("invalid release manifest")
	}
	installerPath := filepath.Join(releaseDir, manifest.Filename)
	info, err := os.Stat(installerPath)
	if err != nil {
		return localAgentReleaseManifest{}, "", fmt.Errorf("stat release installer: %w", err)
	}
	if !info.Mode().IsRegular() {
		return localAgentReleaseManifest{}, "", fmt.Errorf("release installer is not a regular file")
	}
	return manifest, installerPath, nil
}
