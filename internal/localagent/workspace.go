package localagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	workspaceStatusPending       = "pending"
	workspaceStatusSaved         = "saved"
	workspaceStatusReadyToSubmit = "ready_to_submit"
	workspaceStatusSubmitted     = "submitted"
	interpretFPSVersion          = "input-trim-setpts-v3"

	vlmStatusIdle    = "idle"
	vlmStatusQueued  = "queued"
	vlmStatusRunning = "running"
	vlmStatusReady   = "ready"
	vlmStatusFailed  = "failed"
)

type WorkspaceItem struct {
	ID                  string                   `json:"id"`
	Status              string                   `json:"status"`
	ProductID           string                   `json:"product_id,omitempty"`
	SubmittedAssetID    string                   `json:"submitted_asset_id,omitempty"`
	AssetName           string                   `json:"asset_name,omitempty"`
	SourceType          string                   `json:"source_type,omitempty"`
	OriginalFileName    string                   `json:"original_file_name"`
	OriginalSourcePath  string                   `json:"original_source_path,omitempty"`
	OriginalProbe       ffmpeg.ProbeResult       `json:"original_probe,omitempty"`
	SourceFileName      string                   `json:"source_file_name"`
	SourceFileSize      int64                    `json:"source_file_size"`
	SourcePath          string                   `json:"source_path"`
	CleanShotPath       string                   `json:"clean_shot_path,omitempty"`
	CleanShotName       string                   `json:"clean_shot_name,omitempty"`
	Checksum            string                   `json:"checksum,omitempty"`
	SourceInMs          int                      `json:"source_in_ms"`
	SourceOutMs         int                      `json:"source_out_ms"`
	InterpretFPS        bool                     `json:"interpret_fps_enabled"`
	InterpretFPSVersion string                   `json:"interpret_fps_version,omitempty"`
	PlaybackFPS         float64                  `json:"playback_fps,omitempty"`
	SpeedRatio          float64                  `json:"speed_ratio,omitempty"`
	Transcript          string                   `json:"transcript,omitempty"`
	ReviewerNotes       string                   `json:"reviewer_notes,omitempty"`
	Probe               ffmpeg.ProbeResult       `json:"probe"`
	PreviewInMs         int                      `json:"preview_in_ms,omitempty"`
	PreviewOutMs        int                      `json:"preview_out_ms,omitempty"`
	PreviewFrames       []WorkspaceFrameSnapshot `json:"preview_frame_snapshots,omitempty"`
	FrameSnapshots      []WorkspaceFrameSnapshot `json:"frame_snapshots,omitempty"`
	Analysis            *WorkspaceAnalysis       `json:"analysis,omitempty"`
	VLMStatus           string                   `json:"vlm_status,omitempty"`
	VLMError            string                   `json:"vlm_error,omitempty"`
	VLMStartedAt        *time.Time               `json:"vlm_started_at,omitempty"`
	VLMFinishedAt       *time.Time               `json:"vlm_finished_at,omitempty"`
	LastError           string                   `json:"last_error,omitempty"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
	SubmittedAt         *time.Time               `json:"submitted_at,omitempty"`
}

type WorkspaceFrameSnapshot struct {
	FrameIndex  int    `json:"frame_index"`
	TimestampMs int    `json:"timestamp_ms"`
	ImagePath   string `json:"image_path"`
}

type WorkspaceAnalysis struct {
	SceneDescription  string         `json:"scene_description"`
	ShotSize          string         `json:"shot_size"`
	CameraMovement    string         `json:"camera_movement"`
	VisualTags        []string       `json:"visual_tags,omitempty"`
	QualityTags       []string       `json:"quality_tags,omitempty"`
	VisibleProduct    bool           `json:"visible_product"`
	ProductPosition   string         `json:"product_position,omitempty"`
	SceneContext      string         `json:"scene_context,omitempty"`
	ActionDescription string         `json:"action_description,omitempty"`
	PeoplePresence    bool           `json:"people_presence"`
	FaceVisible       bool           `json:"face_visible"`
	LightingCondition string         `json:"lighting_condition,omitempty"`
	ModelResult       map[string]any `json:"model_result,omitempty"`
}

type WorkspaceSaveInput struct {
	AssetName     string  `json:"asset_name"`
	SourceType    string  `json:"source_type"`
	SourceInMs    int     `json:"source_in_ms"`
	SourceOutMs   int     `json:"source_out_ms"`
	InterpretFPS  bool    `json:"interpret_fps_enabled"`
	PlaybackFPS   float64 `json:"playback_fps"`
	Transcript    string  `json:"transcript"`
	ReviewerNotes string  `json:"reviewer_notes"`
}

type WorkspacePreviewFramesInput struct {
	SourceInMs  int `json:"source_in_ms"`
	SourceOutMs int `json:"source_out_ms"`
}

type WorkspaceVLMLabelInput struct {
	SourceType    string `json:"source_type"`
	ProductName   string `json:"product_name"`
	SourceInMs    int    `json:"source_in_ms"`
	SourceOutMs   int    `json:"source_out_ms"`
	ServerBaseURL string `json:"server_base_url"`
	AuthToken     string `json:"auth_token"`
}

type WorkspaceSubmitInput struct {
	ProductID       string   `json:"product_id"`
	UploadURL       string   `json:"upload_url"`
	UploadToken     string   `json:"upload_token"`
	SellingPointIDs []string `json:"selling_point_ids"`
}

type Workspace struct {
	root      string
	statePath string
	processor Processor

	mu    sync.Mutex
	items map[string]WorkspaceItem
	order []string
}

type workspaceState struct {
	Order []string        `json:"order"`
	Items []WorkspaceItem `json:"items"`
}

func NewWorkspace(root string, processor Processor) (*Workspace, error) {
	if root == "" {
		root = filepath.Join(os.TempDir(), "aicut-local-workspace")
	}
	if processor == nil {
		processor = NewProcessor()
	}
	if err := os.MkdirAll(filepath.Join(root, "items"), 0755); err != nil {
		return nil, err
	}
	w := &Workspace{
		root:      root,
		statePath: filepath.Join(root, "workspace.json"),
		processor: processor,
		items:     map[string]WorkspaceItem{},
	}
	if err := w.load(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Workspace) ListItems() []WorkspaceItem {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.listItemsLocked()
}

func (w *Workspace) GetItem(itemID string) (WorkspaceItem, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	item, ok := w.items[itemID]
	return item, ok
}

func (w *Workspace) ImportFiles(ctx context.Context, headers []*multipart.FileHeader) ([]WorkspaceItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	items := make([]WorkspaceItem, 0, len(headers))
	for _, header := range headers {
		item, err := w.importFileLocked(ctx, header)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := w.persistLocked(); err != nil {
		return nil, err
	}
	return items, nil
}

func (w *Workspace) DuplicateItem(itemID string) (WorkspaceItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	item, ok := w.items[itemID]
	if !ok {
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}

	duplicate, err := w.duplicateItemLocked(item)
	if err != nil {
		return WorkspaceItem{}, err
	}
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return duplicate, nil
}

func (w *Workspace) SaveItem(ctx context.Context, itemID string, input WorkspaceSaveInput) (WorkspaceItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	item, ok := w.items[itemID]
	if !ok {
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	item = normalizeWorkspaceItem(item)
	if err := validateSaveInput(input, effectiveOriginalProbe(item).FPS); err != nil {
		return WorkspaceItem{}, err
	}

	item.AssetName = strings.TrimSpace(input.AssetName)
	item.SourceType = input.SourceType
	item.SourceInMs = input.SourceInMs
	item.SourceOutMs = input.SourceOutMs
	if err := w.applyWorkingSourceLocked(ctx, &item, input); err != nil {
		return WorkspaceItem{}, err
	}
	item.Transcript = strings.TrimSpace(input.Transcript)
	item.ReviewerNotes = strings.TrimSpace(input.ReviewerNotes)
	if item.Status != workspaceStatusReadyToSubmit && item.Status != workspaceStatusSubmitted {
		item.Status = workspaceStatusSaved
	}
	item.LastError = ""
	item.UpdatedAt = time.Now()

	w.items[itemID] = item
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
}

func (w *Workspace) PrepareItem(ctx context.Context, itemID string) (WorkspaceItem, error) {
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	w.mu.Unlock()

	if err := validateItemForPrepare(item); err != nil {
		return WorkspaceItem{}, err
	}

	prepared, err := w.prepareItem(ctx, item)

	w.mu.Lock()
	defer w.mu.Unlock()

	if err != nil {
		current := w.items[itemID]
		current.LastError = err.Error()
		current.UpdatedAt = time.Now()
		w.items[itemID] = current
		_ = w.persistLocked()
		return WorkspaceItem{}, err
	}

	prepared.Status = workspaceStatusReadyToSubmit
	prepared.LastError = ""
	prepared.UpdatedAt = time.Now()
	w.items[itemID] = prepared
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return prepared, nil
}

func (w *Workspace) PreviewFrames(ctx context.Context, itemID string, input WorkspacePreviewFramesInput) (WorkspaceItem, error) {
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	w.mu.Unlock()

	sourceOutMs := input.SourceOutMs
	if sourceOutMs <= 0 {
		sourceOutMs = item.Probe.DurationMs
	}
	if err := validateSourceRange(input.SourceInMs, sourceOutMs, item.Probe.DurationMs); err != nil {
		return WorkspaceItem{}, err
	}

	frameTimestamps := resolveThreeFrameTimestampsInRange(input.SourceInMs, sourceOutMs, item.Probe.FPS)
	frameDir := filepath.Join(w.root, "items", item.ID, "preview-frames")
	frames, err := w.processor.ExtractFrames(ctx, item.SourcePath, frameDir, frameTimestamps)
	if err != nil {
		return WorkspaceItem{}, err
	}
	if err := validateExtractedFrames(frames, len(frameTimestamps)); err != nil {
		return WorkspaceItem{}, err
	}

	frameSnapshots := make([]WorkspaceFrameSnapshot, 0, len(frames))
	for _, frame := range frames {
		frameSnapshots = append(frameSnapshots, WorkspaceFrameSnapshot{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: frame.TimestampMs,
			ImagePath:   frame.OutputPath,
		})
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	current, ok := w.items[itemID]
	if !ok {
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	current.PreviewInMs = input.SourceInMs
	current.PreviewOutMs = sourceOutMs
	current.PreviewFrames = frameSnapshots
	current.LastError = ""
	current.UpdatedAt = time.Now()
	w.items[itemID] = current
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return current, nil
}

func (w *Workspace) StartVLMLabel(itemID string, input WorkspaceVLMLabelInput) (WorkspaceItem, error) {
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	sourceOutMs := input.SourceOutMs
	if sourceOutMs <= 0 {
		sourceOutMs = item.Probe.DurationMs
	}
	if err := validateSourceRange(input.SourceInMs, sourceOutMs, item.Probe.DurationMs); err != nil {
		w.mu.Unlock()
		return WorkspaceItem{}, err
	}
	sourceType := input.SourceType
	if sourceType == "" {
		sourceType = item.SourceType
	}
	if sourceType == "" {
		sourceType = "visual_only"
	}
	if sourceType != "visual_only" && sourceType != "talking_head" {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("invalid source type")
	}

	item.SourceType = sourceType
	item.SourceInMs = input.SourceInMs
	item.SourceOutMs = sourceOutMs
	item.VLMStatus = vlmStatusQueued
	item.VLMError = ""
	item.VLMStartedAt = nil
	item.VLMFinishedAt = nil
	item.Analysis = nil
	item.UpdatedAt = time.Now()
	w.items[itemID] = item
	if err := w.persistLocked(); err != nil {
		w.mu.Unlock()
		return WorkspaceItem{}, err
	}
	w.mu.Unlock()

	go w.runVLMLabel(context.Background(), itemID, WorkspaceVLMLabelInput{
		SourceType:    sourceType,
		ProductName:   input.ProductName,
		SourceInMs:    input.SourceInMs,
		SourceOutMs:   sourceOutMs,
		ServerBaseURL: input.ServerBaseURL,
		AuthToken:     input.AuthToken,
	})

	return item, nil
}

func (w *Workspace) runVLMLabel(ctx context.Context, itemID string, input WorkspaceVLMLabelInput) {
	startedAt := time.Now()
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return
	}
	item.VLMStatus = vlmStatusRunning
	item.VLMStartedAt = &startedAt
	item.VLMError = ""
	item.UpdatedAt = startedAt
	w.items[itemID] = item
	_ = w.persistLocked()
	w.mu.Unlock()

	result, previewFrames, err := w.labelItem(ctx, item, input)

	w.mu.Lock()
	defer w.mu.Unlock()
	current, ok := w.items[itemID]
	if !ok {
		return
	}
	finishedAt := time.Now()
	current.VLMFinishedAt = &finishedAt
	current.UpdatedAt = finishedAt
	if err != nil {
		current.VLMStatus = vlmStatusFailed
		current.VLMError = err.Error()
		current.LastError = err.Error()
		w.items[itemID] = current
		_ = w.persistLocked()
		return
	}
	current.PreviewInMs = input.SourceInMs
	current.PreviewOutMs = input.SourceOutMs
	current.PreviewFrames = previewFrames
	current.Analysis = workspaceAnalysisFromResult(result)
	current.VLMStatus = vlmStatusReady
	current.VLMError = ""
	current.LastError = ""
	w.items[itemID] = current
	_ = w.persistLocked()
}

func (w *Workspace) Clear() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, id := range w.order {
		_ = os.RemoveAll(filepath.Join(w.root, "items", id))
	}

	w.items = map[string]WorkspaceItem{}
	w.order = nil
	return w.persistLocked()
}

func (w *Workspace) SubmitItem(ctx context.Context, itemID string, input WorkspaceSubmitInput) (WorkspaceItem, error) {
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	w.mu.Unlock()

	if item.Status != workspaceStatusReadyToSubmit {
		return WorkspaceItem{}, fmt.Errorf("workspace item is not ready to submit")
	}
	if err := validateSubmitInput(input); err != nil {
		return WorkspaceItem{}, err
	}

	submittedAssetID, err := w.submitPreparedItem(ctx, item, input)
	if err != nil {
		w.mu.Lock()
		current := w.items[itemID]
		current.LastError = err.Error()
		current.UpdatedAt = time.Now()
		w.items[itemID] = current
		_ = w.persistLocked()
		w.mu.Unlock()
		return WorkspaceItem{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	item = w.items[itemID]
	item.Status = workspaceStatusSubmitted
	item.ProductID = input.ProductID
	item.SubmittedAssetID = submittedAssetID
	item.SubmittedAt = &now
	item.LastError = ""
	item.UpdatedAt = now
	w.items[itemID] = item
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
}

func (w *Workspace) load() error {
	data, err := os.ReadFile(w.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state workspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	w.order = append([]string(nil), state.Order...)
	for _, item := range state.Items {
		w.items[item.ID] = normalizeWorkspaceItem(item)
	}
	return nil
}

func normalizeWorkspaceItem(item WorkspaceItem) WorkspaceItem {
	if item.OriginalSourcePath == "" {
		item.OriginalSourcePath = item.SourcePath
	}
	if item.OriginalProbe == (ffmpeg.ProbeResult{}) {
		item.OriginalProbe = item.Probe
	}
	if item.PlaybackFPS == 0 && item.InterpretFPS {
		item.PlaybackFPS = item.Probe.FPS
	}
	if item.SpeedRatio == 0 {
		item.SpeedRatio = resolveSpeedRatio(effectiveOriginalProbe(item).FPS, item.InterpretFPS, item.PlaybackFPS)
	}
	return item
}

func effectiveOriginalProbe(item WorkspaceItem) ffmpeg.ProbeResult {
	if item.OriginalProbe != (ffmpeg.ProbeResult{}) {
		return item.OriginalProbe
	}
	return item.Probe
}

func (w *Workspace) persistLocked() error {
	state := workspaceState{
		Order: append([]string(nil), w.order...),
		Items: make([]WorkspaceItem, 0, len(w.items)),
	}
	for _, id := range w.order {
		if item, ok := w.items[id]; ok {
			state.Items = append(state.Items, item)
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.statePath, data, 0644)
}

func (w *Workspace) listItemsLocked() []WorkspaceItem {
	items := make([]WorkspaceItem, 0, len(w.order))
	for _, id := range w.order {
		if item, ok := w.items[id]; ok {
			items = append(items, item)
		}
	}
	return items
}

func (w *Workspace) importFileLocked(ctx context.Context, header *multipart.FileHeader) (WorkspaceItem, error) {
	file, err := header.Open()
	if err != nil {
		return WorkspaceItem{}, err
	}
	defer file.Close()

	itemID := uuid.NewString()
	itemDir := filepath.Join(w.root, "items", itemID)
	if err := os.MkdirAll(itemDir, 0755); err != nil {
		return WorkspaceItem{}, err
	}

	sourceFileName := sanitizeFileName(header.Filename)
	sourcePath := filepath.Join(itemDir, "source"+filepath.Ext(sourceFileName))
	out, err := os.Create(sourcePath)
	if err != nil {
		return WorkspaceItem{}, err
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		return WorkspaceItem{}, err
	}
	if err := out.Close(); err != nil {
		return WorkspaceItem{}, err
	}

	now := time.Now()
	item := WorkspaceItem{
		ID:                 itemID,
		Status:             workspaceStatusPending,
		OriginalFileName:   header.Filename,
		OriginalSourcePath: sourcePath,
		SourceFileName:     sourceFileName,
		SourceFileSize:     header.Size,
		SourcePath:         sourcePath,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	probe, err := w.processor.Probe(ctx, sourcePath)
	if err == nil {
		item.Probe = probe
		item.OriginalProbe = probe
		item.SourceOutMs = probe.DurationMs
	} else {
		item.LastError = err.Error()
	}

	w.items[itemID] = item
	w.order = append(w.order, itemID)
	return item, nil
}

func (w *Workspace) duplicateItemLocked(source WorkspaceItem) (WorkspaceItem, error) {
	source = normalizeWorkspaceItem(source)
	itemID := uuid.NewString()
	itemDir := filepath.Join(w.root, "items", itemID)
	if err := os.MkdirAll(itemDir, 0755); err != nil {
		return WorkspaceItem{}, err
	}

	sourceExt := filepath.Ext(source.SourceFileName)
	if sourceExt == "" {
		sourceExt = filepath.Ext(source.SourcePath)
	}
	sourcePath := filepath.Join(itemDir, "source"+sourceExt)
	if err := copyFile(source.SourcePath, sourcePath); err != nil {
		return WorkspaceItem{}, err
	}
	originalSourceExt := filepath.Ext(source.OriginalSourcePath)
	if originalSourceExt == "" {
		originalSourceExt = sourceExt
	}
	originalSourcePath := filepath.Join(itemDir, "original-source"+originalSourceExt)
	if err := copyFile(source.OriginalSourcePath, originalSourcePath); err != nil {
		return WorkspaceItem{}, err
	}

	now := time.Now()
	duplicate := WorkspaceItem{
		ID:                  itemID,
		Status:              workspaceStatusSaved,
		AssetName:           source.AssetName,
		SourceType:          source.SourceType,
		OriginalFileName:    source.OriginalFileName,
		OriginalSourcePath:  originalSourcePath,
		OriginalProbe:       source.OriginalProbe,
		SourceFileName:      source.SourceFileName,
		SourceFileSize:      source.SourceFileSize,
		SourcePath:          sourcePath,
		SourceInMs:          source.SourceInMs,
		SourceOutMs:         source.SourceOutMs,
		InterpretFPS:        source.InterpretFPS,
		InterpretFPSVersion: source.InterpretFPSVersion,
		PlaybackFPS:         source.PlaybackFPS,
		SpeedRatio:          source.SpeedRatio,
		Transcript:          source.Transcript,
		ReviewerNotes:       source.ReviewerNotes,
		Probe:               source.Probe,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if duplicate.SourceType == "" || duplicate.SourceOutMs <= duplicate.SourceInMs {
		duplicate.Status = workspaceStatusPending
	}

	w.items[itemID] = duplicate
	w.order = append(w.order, itemID)
	return duplicate, nil
}

func (w *Workspace) applyWorkingSourceLocked(ctx context.Context, item *WorkspaceItem, input WorkspaceSaveInput) error {
	originalProbe := effectiveOriginalProbe(*item)
	if input.InterpretFPS {
		resolvedProbe, err := w.resolveOriginalProbeForInterpretFPS(ctx, *item, input)
		if err != nil {
			return err
		}
		originalProbe = resolvedProbe
		item.OriginalProbe = resolvedProbe
		expectedDurationMs := interpretedDurationMs(originalProbe.DurationMs, originalProbe.FPS, input.PlaybackFPS)
		workingPath := filepath.Join(w.root, "items", item.ID, "working-source.mp4")
		needsRebuild := !item.InterpretFPS || item.InterpretFPSVersion != interpretFPSVersion || item.SourcePath != workingPath || item.PlaybackFPS != input.PlaybackFPS || item.Probe.DurationMs != expectedDurationMs
		if needsRebuild {
			if err := w.processor.InterpretFPS(ctx, item.OriginalSourcePath, workingPath, originalProbe.FPS, input.PlaybackFPS, originalProbe.DurationMs); err != nil {
				return err
			}
			probe, err := w.processor.Probe(ctx, workingPath)
			if err != nil {
				return err
			}
			probe.DurationMs = expectedDurationMs
			probe.FPS = input.PlaybackFPS
			item.SourcePath = workingPath
			item.Probe = probe
			item.SourceInMs = 0
			item.SourceOutMs = probe.DurationMs
			item.PreviewInMs = 0
			item.PreviewOutMs = 0
			item.PreviewFrames = nil
			item.FrameSnapshots = nil
			item.Analysis = nil
			item.CleanShotPath = ""
			item.CleanShotName = ""
			item.Checksum = ""
			item.LastError = ""
			item.Status = workspaceStatusSaved
		}
		item.InterpretFPS = true
		item.InterpretFPSVersion = interpretFPSVersion
		item.PlaybackFPS = input.PlaybackFPS
		item.SpeedRatio = resolveSpeedRatio(originalProbe.FPS, true, input.PlaybackFPS)
		return nil
	}

	if item.InterpretFPS {
		item.SourcePath = item.OriginalSourcePath
		item.Probe = originalProbe
		item.SourceInMs = 0
		item.SourceOutMs = originalProbe.DurationMs
		item.PreviewInMs = 0
		item.PreviewOutMs = 0
		item.PreviewFrames = nil
		item.FrameSnapshots = nil
		item.Analysis = nil
		item.CleanShotPath = ""
		item.CleanShotName = ""
		item.Checksum = ""
		item.LastError = ""
		item.Status = workspaceStatusSaved
	}
	item.InterpretFPS = false
	item.InterpretFPSVersion = ""
	item.PlaybackFPS = 0
	item.SpeedRatio = 1
	return nil
}

func (w *Workspace) resolveOriginalProbeForInterpretFPS(ctx context.Context, item WorkspaceItem, input WorkspaceSaveInput) (ffmpeg.ProbeResult, error) {
	probe := effectiveOriginalProbe(item)
	if probe.DurationMs > 0 {
		return probe, nil
	}
	if strings.TrimSpace(item.OriginalSourcePath) != "" {
		refreshed, err := w.processor.Probe(ctx, item.OriginalSourcePath)
		if err == nil {
			probe = mergeProbeForInterpretFPS(probe, refreshed)
		}
	}
	if probe.DurationMs <= 0 {
		probe.DurationMs = firstPositiveInt(item.SourceOutMs, input.SourceOutMs)
	}
	if probe.DurationMs <= 0 {
		return ffmpeg.ProbeResult{}, fmt.Errorf("source duration is required for interpret fps")
	}
	return probe, nil
}

func mergeProbeForInterpretFPS(current ffmpeg.ProbeResult, refreshed ffmpeg.ProbeResult) ffmpeg.ProbeResult {
	if refreshed.DurationMs > 0 {
		current.DurationMs = refreshed.DurationMs
	}
	if current.FPS <= 0 && refreshed.FPS > 0 {
		current.FPS = refreshed.FPS
	}
	if current.Width == 0 {
		current.Width = refreshed.Width
	}
	if current.Height == 0 {
		current.Height = refreshed.Height
	}
	if current.Codec == "" {
		current.Codec = refreshed.Codec
	}
	if !current.HasAudio && refreshed.HasAudio {
		current.HasAudio = refreshed.HasAudio
		current.AudioCodec = refreshed.AudioCodec
		current.AudioChannels = refreshed.AudioChannels
	}
	if current.BitrateKbps == 0 {
		current.BitrateKbps = refreshed.BitrateKbps
	}
	return current
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func interpretedDurationMs(sourceDurationMs int, sourceFPS float64, playbackFPS float64) int {
	if sourceDurationMs <= 0 || sourceFPS <= 0 || playbackFPS <= 0 {
		return sourceDurationMs
	}
	return int(math.Round(float64(sourceDurationMs) * sourceFPS / playbackFPS))
}

func (w *Workspace) prepareItem(ctx context.Context, item WorkspaceItem) (WorkspaceItem, error) {
	itemDir := filepath.Join(w.root, "items", item.ID)
	cleanShotPath := filepath.Join(itemDir, "clean-shot"+filepath.Ext(item.SourceFileName))

	if err := w.processor.Cut(ctx, item.SourcePath, cleanShotPath, item.SourceInMs, item.SourceOutMs, ffmpeg.CutOptions{}); err != nil {
		return WorkspaceItem{}, err
	}

	probe, err := w.processor.Probe(ctx, cleanShotPath)
	if err != nil {
		return WorkspaceItem{}, err
	}

	frameTimestamps := resolveThreeFrameTimestamps(probe.DurationMs, probe.FPS)
	frameDir := filepath.Join(itemDir, "frames")
	frames, err := w.processor.ExtractFrames(ctx, cleanShotPath, frameDir, frameTimestamps)
	if err != nil {
		return WorkspaceItem{}, err
	}
	if err := validateExtractedFrames(frames, len(frameTimestamps)); err != nil {
		return WorkspaceItem{}, err
	}

	frameSnapshots := make([]WorkspaceFrameSnapshot, 0, len(frames))
	for _, frame := range frames {
		frameSnapshots = append(frameSnapshots, WorkspaceFrameSnapshot{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: frame.TimestampMs,
			ImagePath:   frame.OutputPath,
		})
	}

	checksum, err := fileChecksum(cleanShotPath)
	if err != nil {
		return WorkspaceItem{}, err
	}

	item.CleanShotPath = cleanShotPath
	item.CleanShotName = filepath.Base(cleanShotPath)
	item.Checksum = checksum
	item.Probe = probe
	item.FrameSnapshots = frameSnapshots

	return item, nil
}

func (w *Workspace) labelItem(ctx context.Context, item WorkspaceItem, input WorkspaceVLMLabelInput) (modelgateway.AnalyzeAssetResult, []WorkspaceFrameSnapshot, error) {
	if strings.TrimSpace(input.ServerBaseURL) == "" {
		return modelgateway.AnalyzeAssetResult{}, nil, fmt.Errorf("server_base_url is required")
	}
	if strings.TrimSpace(input.AuthToken) == "" {
		return modelgateway.AnalyzeAssetResult{}, nil, fmt.Errorf("auth_token is required")
	}

	frameTimestamps := resolveThreeFrameTimestampsInRange(input.SourceInMs, input.SourceOutMs, item.Probe.FPS)
	frameDir := filepath.Join(w.root, "items", item.ID, "vlm-frames")
	frames, err := w.processor.ExtractFrames(ctx, item.SourcePath, frameDir, frameTimestamps)
	if err != nil {
		return modelgateway.AnalyzeAssetResult{}, nil, err
	}
	if err := validateExtractedFrames(frames, len(frameTimestamps)); err != nil {
		return modelgateway.AnalyzeAssetResult{}, nil, err
	}

	frameSnapshots := make([]WorkspaceFrameSnapshot, 0, len(frames))
	frameRefs := make([]modelgateway.FrameReference, 0, len(frames))
	for _, frame := range frames {
		timestampMs := frame.TimestampMs
		if frame.FrameIndex >= 0 && frame.FrameIndex < len(frameTimestamps) {
			timestampMs = frameTimestamps[frame.FrameIndex]
		}
		frameSnapshots = append(frameSnapshots, WorkspaceFrameSnapshot{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: timestampMs,
			ImagePath:   frame.OutputPath,
		})
		frameRefs = append(frameRefs, modelgateway.FrameReference{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: timestampMs,
			StorageKey:  frame.OutputPath,
		})
	}

	result, err := analyzeFramesOnServer(ctx, input.ServerBaseURL, input.AuthToken, modelgateway.AnalyzeAssetInput{
		AssetID:        item.ID,
		SourceType:     input.SourceType,
		ProductName:    input.ProductName,
		DurationMs:     input.SourceOutMs - input.SourceInMs,
		Width:          item.Probe.Width,
		Height:         item.Probe.Height,
		HasAudio:       item.Probe.HasAudio,
		AudioCodec:     item.Probe.AudioCodec,
		FrameSnapshots: frameRefs,
	})
	if err != nil {
		return modelgateway.AnalyzeAssetResult{}, nil, err
	}
	return result, frameSnapshots, nil
}

func analyzeFramesOnServer(ctx context.Context, serverBaseURL string, authToken string, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"asset_id":     input.AssetID,
		"source_type":  input.SourceType,
		"product_name": input.ProductName,
		"duration_ms":  fmt.Sprintf("%d", input.DurationMs),
		"width":        fmt.Sprintf("%d", input.Width),
		"height":       fmt.Sprintf("%d", input.Height),
		"has_audio":    fmt.Sprintf("%t", input.HasAudio),
		"audio_codec":  input.AudioCodec,
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return modelgateway.AnalyzeAssetResult{}, err
		}
	}

	for _, frame := range input.FrameSnapshots {
		if err := writer.WriteField(fmt.Sprintf("frame_%d_timestamp_ms", frame.FrameIndex), fmt.Sprintf("%d", frame.TimestampMs)); err != nil {
			return modelgateway.AnalyzeAssetResult{}, err
		}
		if err := addFilePart(writer, fmt.Sprintf("frame_%d", frame.FrameIndex), frame.StorageKey, filepath.Base(frame.StorageKey)); err != nil {
			return modelgateway.AnalyzeAssetResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return modelgateway.AnalyzeAssetResult{}, err
	}

	endpoint := strings.TrimRight(strings.TrimSpace(serverBaseURL), "/") + "/api/preprocess/vlm-label"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return modelgateway.AnalyzeAssetResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(authToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return modelgateway.AnalyzeAssetResult{}, fmt.Errorf("request server vlm label failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return modelgateway.AnalyzeAssetResult{}, fmt.Errorf("read server vlm label response failed: %w", err)
	}

	var decoded struct {
		Data struct {
			Analysis modelgateway.AnalyzeAssetResult `json:"analysis"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return modelgateway.AnalyzeAssetResult{}, fmt.Errorf("decode server vlm label response failed: %w: %s", err, string(respBody))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return modelgateway.AnalyzeAssetResult{}, fmt.Errorf("%s", decoded.Error.Message)
		}
		return modelgateway.AnalyzeAssetResult{}, fmt.Errorf("server vlm label returned status %d", resp.StatusCode)
	}
	return decoded.Data.Analysis, nil
}

func addFilePart(writer *multipart.Writer, fieldName string, path string, fileName string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func (w *Workspace) submitPreparedItem(ctx context.Context, item WorkspaceItem, input WorkspaceSubmitInput) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"submission_mode":      "preprocessed",
		"source_type":          item.SourceType,
		"manual_clean_status":  "cleaned",
		"usability_status":     firstNonEmpty(itemAnalysisUsability(item), "usable"),
		"asset_name":           firstNonEmpty(item.AssetName, item.OriginalFileName),
		"source_path":          item.SourcePath,
		"source_original_name": item.OriginalFileName,
		"reviewer_notes":       item.ReviewerNotes,
		"product_id":           input.ProductID,
		"source_in_ms":         fmt.Sprintf("%d", item.SourceInMs),
		"source_out_ms":        fmt.Sprintf("%d", item.SourceOutMs),
		"duration_ms":          fmt.Sprintf("%d", item.Probe.DurationMs),
		"width":                fmt.Sprintf("%d", item.Probe.Width),
		"height":               fmt.Sprintf("%d", item.Probe.Height),
		"fps":                  fmt.Sprintf("%.3f", item.Probe.FPS),
		"codec":                item.Probe.Codec,
		"has_audio":            fmt.Sprintf("%t", item.Probe.HasAudio),
		"audio_codec":          item.Probe.AudioCodec,
		"bitrate_kbps":         fmt.Sprintf("%d", item.Probe.BitrateKbps),
		"checksum":             item.Checksum,
		"analysis_status":      "ready",
		"scene_description":    itemAnalysisSceneDescription(item),
		"shot_size":            itemAnalysisShotSize(item),
		"camera_movement":      itemAnalysisCameraMovement(item),
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return "", err
		}
	}
	if len(input.SellingPointIDs) > 0 {
		if err := writer.WriteField("selling_point_ids", strings.Join(input.SellingPointIDs, ",")); err != nil {
			return "", err
		}
	}
	if len(item.Transcript) > 0 {
		if err := writer.WriteField("transcript", item.Transcript); err != nil {
			return "", err
		}
	}
	if item.Analysis != nil {
		if err := writer.WriteField("subjects_json", mustJSONString([]string{})); err != nil {
			return "", err
		}
		if err := writer.WriteField("scene_tags_json", mustJSONString(item.Analysis.VisualTags)); err != nil {
			return "", err
		}
		if err := writer.WriteField("quality_tags_json", mustJSONString(item.Analysis.QualityTags)); err != nil {
			return "", err
		}
		if err := writer.WriteField("model_result_json", mustJSONString(item.Analysis.ModelResult)); err != nil {
			return "", err
		}
	}

	file, err := os.Open(item.CleanShotPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", item.CleanShotName)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, input.UploadURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Upload-Token", input.UploadToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("submit failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var payload struct {
		Data struct {
			Asset struct {
				ID string `json:"id"`
			} `json:"asset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return "", err
	}
	if payload.Data.Asset.ID == "" {
		return "", fmt.Errorf("submit succeeded but asset id is missing")
	}
	return payload.Data.Asset.ID, nil
}

func validateSaveInput(input WorkspaceSaveInput, sourceFPS float64) error {
	if input.SourceType != "visual_only" && input.SourceType != "talking_head" {
		return fmt.Errorf("invalid source type")
	}
	if err := validateSourceRange(input.SourceInMs, input.SourceOutMs, 0); err != nil {
		return err
	}
	if err := validateInterpretFPS(input.SourceType, sourceFPS, input.InterpretFPS, input.PlaybackFPS); err != nil {
		return err
	}
	return nil
}

func validateItemForPrepare(item WorkspaceItem) error {
	if item.SourceType != "visual_only" && item.SourceType != "talking_head" {
		return fmt.Errorf("source type is required")
	}
	if err := validateSourceRange(item.SourceInMs, item.SourceOutMs, item.Probe.DurationMs); err != nil {
		return err
	}
	if item.SourceType == "talking_head" && strings.TrimSpace(item.Transcript) == "" {
		return fmt.Errorf("transcript is required for talking head")
	}
	if err := validateInterpretFPS(item.SourceType, effectiveOriginalProbe(item).FPS, item.InterpretFPS, item.PlaybackFPS); err != nil {
		return err
	}
	return nil
}

func validateInterpretFPS(sourceType string, sourceFPS float64, enabled bool, playbackFPS float64) error {
	if !enabled {
		return nil
	}
	if sourceType != "visual_only" {
		return fmt.Errorf("interpret fps is only supported for visual-only material")
	}
	if sourceFPS < 25 {
		return fmt.Errorf("source fps must be at least 25")
	}
	if playbackFPS < 25 {
		return fmt.Errorf("playback fps must be at least 25")
	}
	if playbackFPS >= sourceFPS {
		return fmt.Errorf("playback fps must be lower than source fps")
	}
	return nil
}

func resolveSpeedRatio(sourceFPS float64, enabled bool, playbackFPS float64) float64 {
	if !enabled || sourceFPS <= 0 || playbackFPS <= 0 {
		return 1
	}
	return playbackFPS / sourceFPS
}

func validateSubmitInput(input WorkspaceSubmitInput) error {
	if strings.TrimSpace(input.ProductID) == "" {
		return fmt.Errorf("product_id is required")
	}
	if strings.TrimSpace(input.UploadURL) == "" {
		return fmt.Errorf("upload_url is required")
	}
	if strings.TrimSpace(input.UploadToken) == "" {
		return fmt.Errorf("upload_token is required")
	}
	return nil
}

func validateSourceRange(sourceInMs int, sourceOutMs int, durationMs int) error {
	if sourceInMs < 0 || sourceOutMs <= sourceInMs {
		return fmt.Errorf("invalid source range")
	}
	if durationMs > 0 && sourceOutMs > durationMs {
		return fmt.Errorf("source range exceeds source duration")
	}
	return nil
}

func resolveThreeFrameTimestamps(durationMs int, fps float64) []int {
	if durationMs <= 0 || fps <= 0 {
		return []int{0, 0, 0}
	}
	outFrame := msToFrame(durationMs, fps)
	midFrame := outFrame / 2
	return []int{
		frameToMs(0, fps),
		frameToMs(midFrame, fps),
		frameToMs(outFrame, fps),
	}
}

func resolveThreeFrameTimestampsInRange(sourceInMs int, sourceOutMs int, fps float64) []int {
	if sourceOutMs <= sourceInMs {
		return []int{sourceInMs, sourceInMs, sourceInMs}
	}
	if fps <= 0 {
		return []int{sourceInMs, sourceInMs + (sourceOutMs-sourceInMs)/2, sourceOutMs}
	}

	inFrame := msToFrame(sourceInMs, fps)
	outFrame := msToFrame(sourceOutMs, fps)
	if outFrame < inFrame {
		outFrame = inFrame
	}
	midFrame := inFrame + (outFrame-inFrame)/2
	return []int{
		frameToMs(inFrame, fps),
		frameToMs(midFrame, fps),
		frameToMs(outFrame, fps),
	}
}

func msToFrame(timestampMs int, fps float64) int {
	return int(math.Round((float64(timestampMs) / 1000) * fps))
}

func frameToMs(frame int, fps float64) int {
	return int(math.Round((float64(frame) / fps) * 1000))
}

func validateExtractedFrames(frames []ffmpeg.ExtractedFrame, expectedCount int) error {
	if len(frames) != expectedCount {
		return fmt.Errorf("expected %d extracted frames, got %d", expectedCount, len(frames))
	}
	for _, frame := range frames {
		info, err := os.Stat(frame.OutputPath)
		if err != nil {
			return fmt.Errorf("extracted frame %d missing: %w", frame.FrameIndex, err)
		}
		if info.IsDir() || info.Size() == 0 {
			return fmt.Errorf("extracted frame %d is empty", frame.FrameIndex)
		}
	}
	return nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "video.mp4"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
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

func copyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

func sortSnapshots(frames []WorkspaceFrameSnapshot) {
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].FrameIndex < frames[j].FrameIndex
	})
}

func itemAnalysisUsability(item WorkspaceItem) string {
	return "usable"
}

func itemAnalysisSceneDescription(item WorkspaceItem) string {
	if item.Analysis == nil {
		return ""
	}
	return item.Analysis.SceneDescription
}

func itemAnalysisShotSize(item WorkspaceItem) string {
	if item.Analysis == nil {
		return ""
	}
	return item.Analysis.ShotSize
}

func itemAnalysisCameraMovement(item WorkspaceItem) string {
	if item.Analysis == nil {
		return ""
	}
	return item.Analysis.CameraMovement
}

func workspaceAnalysisFromResult(result modelgateway.AnalyzeAssetResult) *WorkspaceAnalysis {
	return &WorkspaceAnalysis{
		SceneDescription:  result.SceneDescription,
		ShotSize:          result.ShotSize,
		CameraMovement:    result.CameraMovement,
		VisualTags:        append([]string(nil), result.VisualTags...),
		QualityTags:       append([]string(nil), result.QualityTags...),
		VisibleProduct:    result.VisibleProduct,
		ProductPosition:   result.ProductPosition,
		SceneContext:      result.SceneContext,
		ActionDescription: result.ActionDescription,
		PeoplePresence:    result.PeoplePresence,
		FaceVisible:       result.FaceVisible,
		LightingCondition: result.LightingCondition,
		ModelResult:       result.ModelResult,
	}
}

func mustJSONString(value any) string {
	if value == nil {
		return "null"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
