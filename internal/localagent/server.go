package localagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

type Options struct {
	Addr   string
	Logger *slog.Logger
}

type Server struct {
	addr   string
	logger *slog.Logger
}

type preprocessRequest struct {
	SourcePath  string `json:"source_path"`
	SourceInMs  int    `json:"source_in_ms"`
	SourceOutMs int    `json:"source_out_ms"`
	SourceType  string `json:"source_type"`
	UploadURL   string `json:"upload_url"`
	UploadToken string `json:"upload_token"`
}

type preprocessResponse struct {
	OutputPath string             `json:"output_path"`
	Checksum   string             `json:"checksum"`
	Probe      ffmpeg.ProbeResult `json:"probe"`
	UploadCode int                `json:"upload_code"`
}

func New(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:58721"
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Server{
		addr:   opts.Addr,
		logger: opts.Logger,
	}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/preprocess", s.handlePreprocess)

	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.withLogging(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.logger.Info("local agent listening", "addr", s.addr)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handlePreprocess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req preprocessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if err := validatePreprocessRequest(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	outputPath := filepath.Join(os.TempDir(), "aicut-clean-shot-"+uuid.NewString()+filepath.Ext(req.SourcePath))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	if err := ffmpeg.Cut(ctx, req.SourcePath, outputPath, req.SourceInMs, req.SourceOutMs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	probe, err := ffmpeg.Probe(ctx, outputPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	checksum, err := fileChecksum(outputPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	uploadCode, err := uploadCleanShot(ctx, req.UploadURL, req.UploadToken, req.SourceType, outputPath)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, preprocessResponse{
		OutputPath: outputPath,
		Checksum:   checksum,
		Probe:      probe,
		UploadCode: uploadCode,
	})
}

func validatePreprocessRequest(req preprocessRequest) error {
	if req.SourcePath == "" {
		return fmt.Errorf("source_path is required")
	}
	if req.SourceOutMs <= req.SourceInMs {
		return fmt.Errorf("invalid source range")
	}
	if req.SourceType != "visual_only" && req.SourceType != "talking_head" {
		return fmt.Errorf("invalid source_type")
	}
	if req.UploadURL == "" {
		return fmt.Errorf("upload_url is required")
	}
	if req.UploadToken == "" {
		return fmt.Errorf("upload_token is required")
	}
	return nil
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadCleanShot(ctx context.Context, uploadURL string, uploadToken string, sourceType string, path string) (int, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("source_type", sourceType); err != nil {
		return 0, err
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Upload-Token", uploadToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	return resp.StatusCode, nil
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("local agent request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
