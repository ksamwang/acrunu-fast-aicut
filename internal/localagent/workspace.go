package localagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
)

type WorkspaceItem struct {
	ID               string                   `json:"id"`
	Status           string                   `json:"status"`
	AssetName        string                   `json:"asset_name,omitempty"`
	SourceType       string                   `json:"source_type,omitempty"`
	OriginalFileName string                   `json:"original_file_name"`
	SourceFileName   string                   `json:"source_file_name"`
	SourceFileSize   int64                    `json:"source_file_size"`
	SourcePath       string                   `json:"source_path"`
	CleanShotPath    string                   `json:"clean_shot_path,omitempty"`
	CleanShotName    string                   `json:"clean_shot_name,omitempty"`
	Checksum         string                   `json:"checksum,omitempty"`
	SourceInMs       int                      `json:"source_in_ms"`
	SourceOutMs      int                      `json:"source_out_ms"`
	Transcript       string                   `json:"transcript,omitempty"`
	ReviewerNotes    string                   `json:"reviewer_notes,omitempty"`
	Probe            ffmpeg.ProbeResult       `json:"probe"`
	FrameSnapshots   []WorkspaceFrameSnapshot `json:"frame_snapshots,omitempty"`
	Analysis         *WorkspaceAnalysis       `json:"analysis,omitempty"`
	LastError        string                   `json:"last_error,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
	SubmittedAt      *time.Time               `json:"submitted_at,omitempty"`
}

type WorkspaceFrameSnapshot struct {
	FrameIndex  int    `json:"frame_index"`
	TimestampMs int    `json:"timestamp_ms"`
	ImagePath   string `json:"image_path"`
}

type WorkspaceAnalysis struct {
	SceneDescription string         `json:"scene_description"`
	ShotSize         string         `json:"shot_size"`
	CameraMovement   string         `json:"camera_movement"`
	Subjects         []string       `json:"subjects,omitempty"`
	SceneTags        []string       `json:"scene_tags,omitempty"`
	QualityTags      []string       `json:"quality_tags,omitempty"`
	UsabilityStatus  string         `json:"usability_status"`
	ModelResult      map[string]any `json:"model_result,omitempty"`
}

type WorkspaceSaveInput struct {
	AssetName     string `json:"asset_name"`
	SourceType    string `json:"source_type"`
	SourceInMs    int    `json:"source_in_ms"`
	SourceOutMs   int    `json:"source_out_ms"`
	Transcript    string `json:"transcript"`
	ReviewerNotes string `json:"reviewer_notes"`
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
		processor = NewProcessor(nil)
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

func (w *Workspace) SaveItem(itemID string, input WorkspaceSaveInput) (WorkspaceItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	item, ok := w.items[itemID]
	if !ok {
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	if err := validateSaveInput(input); err != nil {
		return WorkspaceItem{}, err
	}

	item.AssetName = strings.TrimSpace(input.AssetName)
	item.SourceType = input.SourceType
	item.SourceInMs = input.SourceInMs
	item.SourceOutMs = input.SourceOutMs
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

func (w *Workspace) Clear() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.RemoveAll(filepath.Join(w.root, "items")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(w.root, "items"), 0755); err != nil {
		return err
	}
	w.items = map[string]WorkspaceItem{}
	w.order = nil
	if err := os.Remove(w.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.persistLocked()
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
		w.items[item.ID] = item
	}
	return nil
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
		ID:               itemID,
		Status:           workspaceStatusPending,
		OriginalFileName: header.Filename,
		SourceFileName:   sourceFileName,
		SourceFileSize:   header.Size,
		SourcePath:       sourcePath,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	probe, err := w.processor.Probe(ctx, sourcePath)
	if err == nil {
		item.Probe = probe
		item.SourceOutMs = probe.DurationMs
	} else {
		item.LastError = err.Error()
	}

	w.items[itemID] = item
	w.order = append(w.order, itemID)
	return item, nil
}

func (w *Workspace) prepareItem(ctx context.Context, item WorkspaceItem) (WorkspaceItem, error) {
	itemDir := filepath.Join(w.root, "items", item.ID)
	cleanShotPath := filepath.Join(itemDir, "clean-shot"+filepath.Ext(item.SourceFileName))

	if err := w.processor.Cut(ctx, item.SourcePath, cleanShotPath, item.SourceInMs, item.SourceOutMs); err != nil {
		return WorkspaceItem{}, err
	}

	probe, err := w.processor.Probe(ctx, cleanShotPath)
	if err != nil {
		return WorkspaceItem{}, err
	}

	frameTimestamps := resolveThreeFrameTimestamps(probe.DurationMs)
	frameDir := filepath.Join(itemDir, "frames")
	frames, err := w.processor.ExtractFrames(ctx, cleanShotPath, frameDir, frameTimestamps)
	if err != nil {
		return WorkspaceItem{}, err
	}

	frameSnapshots := make([]WorkspaceFrameSnapshot, 0, len(frames))
	frameRefs := make([]modelgateway.FrameReference, 0, len(frames))
	for _, frame := range frames {
		frameSnapshots = append(frameSnapshots, WorkspaceFrameSnapshot{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: frame.TimestampMs,
			ImagePath:   frame.OutputPath,
		})
		frameRefs = append(frameRefs, modelgateway.FrameReference{
			FrameIndex:  frame.FrameIndex,
			TimestampMs: frame.TimestampMs,
			StorageKey:  frame.OutputPath,
		})
	}

	analysisResult, err := w.processor.Analyze(ctx, modelgateway.AnalyzeAssetInput{
		AssetID:        item.ID,
		SourceType:     item.SourceType,
		DurationMs:     probe.DurationMs,
		Width:          probe.Width,
		Height:         probe.Height,
		HasAudio:       probe.HasAudio,
		AudioCodec:     probe.AudioCodec,
		FrameSnapshots: frameRefs,
	})
	if err != nil {
		return WorkspaceItem{}, err
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
	item.Analysis = &WorkspaceAnalysis{
		SceneDescription: analysisResult.SceneDescription,
		ShotSize:         analysisResult.ShotSize,
		CameraMovement:   analysisResult.CameraMovement,
		Subjects:         append([]string(nil), analysisResult.Subjects...),
		SceneTags:        append([]string(nil), analysisResult.SceneTags...),
		QualityTags:      append([]string(nil), analysisResult.QualityTags...),
		UsabilityStatus:  analysisResult.UsabilityStatus,
		ModelResult:      analysisResult.ModelResult,
	}

	return item, nil
}

func validateSaveInput(input WorkspaceSaveInput) error {
	if input.SourceType != "visual_only" && input.SourceType != "talking_head" {
		return fmt.Errorf("invalid source type")
	}
	if input.SourceOutMs <= input.SourceInMs {
		return fmt.Errorf("invalid source range")
	}
	return nil
}

func validateItemForPrepare(item WorkspaceItem) error {
	if item.SourceType != "visual_only" && item.SourceType != "talking_head" {
		return fmt.Errorf("source type is required")
	}
	if item.SourceOutMs <= item.SourceInMs {
		return fmt.Errorf("invalid source range")
	}
	if item.SourceType == "talking_head" && strings.TrimSpace(item.Transcript) == "" {
		return fmt.Errorf("transcript is required for talking head")
	}
	return nil
}

func resolveThreeFrameTimestamps(durationMs int) []int {
	if durationMs <= 0 {
		return []int{0, 0, 0}
	}
	points := []int{10, 50, 90}
	timestamps := make([]int, 0, len(points))
	for _, point := range points {
		ts := durationMs * point / 100
		if ts >= durationMs {
			ts = durationMs - 1
		}
		if ts < 0 {
			ts = 0
		}
		timestamps = append(timestamps, ts)
	}
	return timestamps
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

func sortSnapshots(frames []WorkspaceFrameSnapshot) {
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].FrameIndex < frames[j].FrameIndex
	})
}
