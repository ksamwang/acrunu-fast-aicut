package localagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Addr          string
	Logger        *slog.Logger
	WorkspaceRoot string
	Processor     Processor
}

type Server struct {
	addr      string
	logger    *slog.Logger
	workspace *Workspace
}

func New(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:58721"
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	workspaceRoot := opts.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(os.TempDir(), "aicut-local-workspace")
	}
	workspace, err := NewWorkspace(workspaceRoot, opts.Processor)
	if err != nil {
		panic(fmt.Errorf("create local workspace: %w", err))
	}

	return &Server{
		addr:      opts.Addr,
		logger:    opts.Logger,
		workspace: workspace,
	}
}

func NewDefaultProcessor() Processor {
	return NewProcessor()
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/workspace/import", s.handleWorkspaceImport)
	mux.HandleFunc("/workspace/items", s.handleWorkspaceItems)
	mux.HandleFunc("/workspace/items/", s.handleWorkspaceItemRoute)
	mux.HandleFunc("/workspace/clear", s.handleWorkspaceClear)

	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.withMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.logger.Info("local agent listening", "addr", s.addr)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleWorkspaceImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing files"})
		return
	}

	items, err := s.workspace.ImportFiles(r.Context(), files)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichItems(r, items)})
}

func (s *Server) handleWorkspaceItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichItems(r, s.workspace.ListItems())})
}

func (s *Server) handleWorkspaceItemRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/workspace/items/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}

	itemID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleWorkspaceItemDetail(w, r, itemID)
		case http.MethodPut:
			s.handleWorkspaceItemSave(w, r, itemID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
		return
	}

	switch parts[1] {
	case "duplicate":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemDuplicate(w, r, itemID)
	case "prepare":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemPrepare(w, r, itemID)
	case "preview-frames":
		if r.Method == http.MethodPost && len(parts) == 2 {
			s.handleWorkspaceItemPreviewFrames(w, r, itemID)
			return
		}
		if r.Method == http.MethodGet && len(parts) == 3 {
			index, err := strconv.Atoi(parts[2])
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid frame index"})
				return
			}
			s.handleWorkspaceItemFile(w, r, itemID, "preview-frame", index)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case "vlm-label":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemVLMLabel(w, r, itemID)
	case "submit":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemSubmit(w, r, itemID)
	case "source":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemFile(w, r, itemID, "source", 0)
	case "clean-shot":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleWorkspaceItemFile(w, r, itemID, "clean-shot", 0)
	case "frames":
		if r.Method != http.MethodGet || len(parts) != 3 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		index, err := strconv.Atoi(parts[2])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid frame index"})
			return
		}
		s.handleWorkspaceItemFile(w, r, itemID, "frame", index)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (s *Server) handleWorkspaceItemDetail(w http.ResponseWriter, r *http.Request, itemID string) {
	item, ok := s.workspace.GetItem(itemID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "workspace item not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemSave(w http.ResponseWriter, r *http.Request, itemID string) {
	var input WorkspaceSaveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	item, err := s.workspace.SaveItem(itemID, input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemPrepare(w http.ResponseWriter, r *http.Request, itemID string) {
	item, err := s.workspace.PrepareItem(context.Background(), itemID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemPreviewFrames(w http.ResponseWriter, r *http.Request, itemID string) {
	var input WorkspacePreviewFramesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	item, err := s.workspace.PreviewFrames(r.Context(), itemID, input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemVLMLabel(w http.ResponseWriter, r *http.Request, itemID string) {
	var input WorkspaceVLMLabelInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	item, err := s.workspace.StartVLMLabel(itemID, input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemDuplicate(w http.ResponseWriter, r *http.Request, itemID string) {
	item, err := s.workspace.DuplicateItem(itemID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemSubmit(w http.ResponseWriter, r *http.Request, itemID string) {
	var input WorkspaceSubmitInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	item, err := s.workspace.SubmitItem(context.Background(), itemID, input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.enrichItem(r, item)})
}

func (s *Server) handleWorkspaceItemFile(w http.ResponseWriter, r *http.Request, itemID string, kind string, frameIndex int) {
	item, ok := s.workspace.GetItem(itemID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "workspace item not found"})
		return
	}

	var path string
	switch kind {
	case "source":
		path = item.SourcePath
	case "clean-shot":
		path = item.CleanShotPath
	case "preview-frame":
		for _, frame := range item.PreviewFrames {
			if frame.FrameIndex == frameIndex {
				path = frame.ImagePath
				break
			}
		}
	case "frame":
		for _, frame := range item.FrameSnapshots {
			if frame.FrameIndex == frameIndex {
				path = frame.ImagePath
				break
			}
		}
	}

	if path == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not found"})
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not found", "path": path})
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) handleWorkspaceClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if err := s.workspace.Clear(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

func (s *Server) enrichItems(r *http.Request, items []WorkspaceItem) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, s.enrichItem(r, item))
	}
	return result
}

func (s *Server) enrichItem(r *http.Request, item WorkspaceItem) map[string]any {
	data := map[string]any{
		"id":                    item.ID,
		"status":                item.Status,
		"product_id":            item.ProductID,
		"submitted_asset_id":    item.SubmittedAssetID,
		"asset_name":            item.AssetName,
		"source_type":           item.SourceType,
		"original_file_name":    item.OriginalFileName,
		"source_file_name":      item.SourceFileName,
		"source_file_size":      item.SourceFileSize,
		"source_in_ms":          item.SourceInMs,
		"source_out_ms":         item.SourceOutMs,
		"interpret_fps_enabled": item.InterpretFPS,
		"playback_fps":          item.PlaybackFPS,
		"speed_ratio":           item.SpeedRatio,
		"transcript":            item.Transcript,
		"reviewer_notes":        item.ReviewerNotes,
		"probe":                 item.Probe,
		"preview_in_ms":         item.PreviewInMs,
		"preview_out_ms":        item.PreviewOutMs,
		"analysis":              item.Analysis,
		"vlm_status":            firstNonEmpty(item.VLMStatus, vlmStatusIdle),
		"vlm_error":             item.VLMError,
		"vlm_started_at":        item.VLMStartedAt,
		"vlm_finished_at":       item.VLMFinishedAt,
		"last_error":            item.LastError,
		"created_at":            item.CreatedAt,
		"updated_at":            item.UpdatedAt,
		"submitted_at":          item.SubmittedAt,
		"checksum":              item.Checksum,
		"source_url":            s.fileURL(r, item.ID, "source", 0),
		"clean_shot_url":        s.fileURL(r, item.ID, "clean-shot", 0),
	}

	frames := make([]map[string]any, 0, len(item.FrameSnapshots))
	for _, frame := range item.FrameSnapshots {
		frames = append(frames, map[string]any{
			"frame_index":  frame.FrameIndex,
			"timestamp_ms": frame.TimestampMs,
			"image_url":    s.fileURL(r, item.ID, "frame", frame.FrameIndex),
		})
	}
	data["frame_snapshots"] = frames

	previewFrames := make([]map[string]any, 0, len(item.PreviewFrames))
	for _, frame := range item.PreviewFrames {
		previewFrames = append(previewFrames, map[string]any{
			"frame_index":  frame.FrameIndex,
			"timestamp_ms": frame.TimestampMs,
			"image_url":    s.fileURL(r, item.ID, "preview-frame", frame.FrameIndex),
		})
	}
	data["preview_frame_snapshots"] = previewFrames
	return data
}

func (s *Server) fileURL(r *http.Request, itemID string, kind string, frameIndex int) string {
	base := "http://" + r.Host
	switch kind {
	case "source":
		return base + "/workspace/items/" + itemID + "/source"
	case "clean-shot":
		return base + "/workspace/items/" + itemID + "/clean-shot"
	case "preview-frame":
		return fmt.Sprintf("%s/workspace/items/%s/preview-frames/%d", base, itemID, frameIndex)
	case "frame":
		return fmt.Sprintf("%s/workspace/items/%s/frames/%d", base, itemID, frameIndex)
	default:
		return ""
	}
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return s.withLogging(s.withCORS(next))
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("local agent request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
