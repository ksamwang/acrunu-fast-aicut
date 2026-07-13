package localagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	asrDraftTimeBase          = "selection_relative_ms"
	asrAudioDurationTolerance = 50
	asrServerResponseLimit    = 4 << 20
	asrServerRequestTimeout   = 330 * time.Second
)

var ErrASRSelectionChanged = errors.New("workspace selection changed during transcription")

type WorkspaceASRTranscribeInput struct {
	SourceInMs    int    `json:"source_in_ms"`
	SourceOutMs   int    `json:"source_out_ms"`
	ServerBaseURL string `json:"server_base_url"`
	AuthToken     string `json:"auth_token"`
}

type WorkspaceTranscriptSegment struct {
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
}

type WorkspaceASRDraft struct {
	Text        string                       `json:"text"`
	Segments    []WorkspaceTranscriptSegment `json:"segments"`
	SourceInMs  int                          `json:"source_in_ms"`
	SourceOutMs int                          `json:"source_out_ms"`
	TimeBase    string                       `json:"time_base"`
	GeneratedAt time.Time                    `json:"generated_at"`
}

type asrServerResponse struct {
	Text        string                       `json:"text"`
	Segments    []WorkspaceTranscriptSegment `json:"segments"`
	SourceInMs  int                          `json:"source_in_ms"`
	SourceOutMs int                          `json:"source_out_ms"`
	TimeBase    string                       `json:"time_base"`
}

func (w *Workspace) TranscribeItem(ctx context.Context, itemID string, input WorkspaceASRTranscribeInput) (WorkspaceItem, error) {
	w.mu.Lock()
	item, ok := w.items[itemID]
	if !ok {
		w.mu.Unlock()
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	item = normalizeWorkspaceItem(item)
	if err := validateASRTranscribeInput(item, input); err != nil {
		w.mu.Unlock()
		return WorkspaceItem{}, err
	}
	w.mu.Unlock()

	tempRoot := filepath.Join(w.root, "temp")
	if err := os.MkdirAll(tempRoot, 0755); err != nil {
		return WorkspaceItem{}, fmt.Errorf("create ASR temp root: %w", err)
	}
	tempDir, err := os.MkdirTemp(tempRoot, "asr-")
	if err != nil {
		return WorkspaceItem{}, fmt.Errorf("create ASR temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	audioPath := filepath.Join(tempDir, "selection.wav")
	if err := w.processor.ExtractAudio(ctx, item.SourcePath, audioPath, input.SourceInMs, input.SourceOutMs); err != nil {
		w.recordASRFailure(itemID, item, err)
		return WorkspaceItem{}, err
	}
	audioProbe, err := w.processor.Probe(ctx, audioPath)
	if err != nil {
		w.recordASRFailure(itemID, item, err)
		return WorkspaceItem{}, fmt.Errorf("probe extracted ASR audio: %w", err)
	}
	if err := validateExtractedASRAudio(audioProbe.DurationMs, audioProbe.HasAudio, input.SourceOutMs-input.SourceInMs); err != nil {
		w.recordASRFailure(itemID, item, err)
		return WorkspaceItem{}, err
	}

	response, err := transcribeAudioOnServer(ctx, input, audioPath)
	if err != nil {
		w.recordASRFailure(itemID, item, err)
		return WorkspaceItem{}, err
	}
	draft, err := workspaceASRDraftFromResponse(response, input)
	if err != nil {
		w.recordASRFailure(itemID, item, err)
		return WorkspaceItem{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	current, ok := w.items[itemID]
	if !ok {
		return WorkspaceItem{}, fmt.Errorf("workspace item not found")
	}
	if !sameASRSelection(current, item) {
		return current, ErrASRSelectionChanged
	}
	current.ASRDraft = &draft
	current.LastError = ""
	current.UpdatedAt = draft.GeneratedAt
	w.items[itemID] = current
	if err := w.persistLocked(); err != nil {
		return WorkspaceItem{}, err
	}
	return current, nil
}

func validateASRTranscribeInput(item WorkspaceItem, input WorkspaceASRTranscribeInput) error {
	if item.Status == workspaceStatusSubmitted {
		return fmt.Errorf("submitted workspace item cannot be transcribed")
	}
	if item.SourceType != "talking_head" {
		return fmt.Errorf("ASR transcription requires talking_head source type")
	}
	if item.InterpretFPS {
		return fmt.Errorf("ASR transcription does not support interpreted FPS material")
	}
	if !item.Probe.HasAudio {
		return fmt.Errorf("workspace source has no audio stream")
	}
	if strings.TrimSpace(item.SourcePath) == "" {
		return fmt.Errorf("workspace source path is required")
	}
	if strings.TrimSpace(input.ServerBaseURL) == "" {
		return fmt.Errorf("server_base_url is required")
	}
	if strings.TrimSpace(input.AuthToken) == "" {
		return fmt.Errorf("auth_token is required")
	}
	if err := validateSourceRange(input.SourceInMs, input.SourceOutMs, item.Probe.DurationMs); err != nil {
		return err
	}
	if input.SourceInMs != item.SourceInMs || input.SourceOutMs != item.SourceOutMs {
		return fmt.Errorf("ASR source range must match the current workspace selection")
	}
	return nil
}

func validateExtractedASRAudio(durationMs int, hasAudio bool, expectedDurationMs int) error {
	if !hasAudio {
		return fmt.Errorf("extracted ASR wav has no audio stream")
	}
	if durationMs <= 0 {
		return fmt.Errorf("extracted ASR wav duration is missing")
	}
	if absDuration(durationMs-expectedDurationMs) > asrAudioDurationTolerance {
		return fmt.Errorf("extracted ASR wav duration mismatch: expected %dms, got %dms", expectedDurationMs, durationMs)
	}
	return nil
}

func transcribeAudioOnServer(ctx context.Context, input WorkspaceASRTranscribeInput, audioPath string) (asrServerResponse, error) {
	audio, err := os.Open(audioPath)
	if err != nil {
		return asrServerResponse{}, err
	}
	defer audio.Close()

	pipeReader, pipeWriter := io.Pipe()
	formWriter := multipart.NewWriter(pipeWriter)
	endpoint := strings.TrimRight(strings.TrimSpace(input.ServerBaseURL), "/") + "/api/preprocess/asr-transcribe"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		return asrServerResponse{}, err
	}
	req.Header.Set("Content-Type", formWriter.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(input.AuthToken))

	writeDone := make(chan error, 1)
	go func() {
		writeErr := formWriter.WriteField("source_in_ms", fmt.Sprintf("%d", input.SourceInMs))
		if writeErr == nil {
			writeErr = formWriter.WriteField("source_out_ms", fmt.Sprintf("%d", input.SourceOutMs))
		}
		var part io.Writer
		if writeErr == nil {
			part, writeErr = formWriter.CreateFormFile("file", filepath.Base(audioPath))
		}
		if writeErr == nil {
			_, writeErr = io.Copy(part, audio)
		}
		if closeErr := formWriter.Close(); writeErr == nil {
			writeErr = closeErr
		}
		_ = pipeWriter.CloseWithError(writeErr)
		writeDone <- writeErr
	}()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	resp, requestErr := (&http.Client{Transport: transport, Timeout: asrServerRequestTimeout}).Do(req)
	if requestErr != nil {
		_ = pipeReader.CloseWithError(requestErr)
		<-writeDone
		return asrServerResponse{}, fmt.Errorf("request server ASR transcription failed: %w", requestErr)
	}
	defer resp.Body.Close()
	writeErr := <-writeDone
	if writeErr != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return asrServerResponse{}, fmt.Errorf("upload ASR audio failed: %w", writeErr)
	}

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, asrServerResponseLimit+1))
	if err != nil {
		return asrServerResponse{}, fmt.Errorf("read server ASR response failed: %w", err)
	}
	if len(responseBody) > asrServerResponseLimit {
		return asrServerResponse{}, fmt.Errorf("server ASR response is too large")
	}
	var payload struct {
		Data  asrServerResponse `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return asrServerResponse{}, fmt.Errorf("decode server ASR response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return asrServerResponse{}, fmt.Errorf("server ASR transcription failed (%s): %s", payload.Error.Code, payload.Error.Message)
		}
		return asrServerResponse{}, fmt.Errorf("server ASR transcription failed with status %d", resp.StatusCode)
	}
	return payload.Data, nil
}

func workspaceASRDraftFromResponse(response asrServerResponse, input WorkspaceASRTranscribeInput) (WorkspaceASRDraft, error) {
	if response.SourceInMs != input.SourceInMs || response.SourceOutMs != input.SourceOutMs {
		return WorkspaceASRDraft{}, fmt.Errorf("server ASR source range does not match request")
	}
	if response.TimeBase != asrDraftTimeBase {
		return WorkspaceASRDraft{}, fmt.Errorf("unsupported ASR time base %q", response.TimeBase)
	}
	durationMs := input.SourceOutMs - input.SourceInMs
	segments := make([]WorkspaceTranscriptSegment, 0, len(response.Segments))
	for _, segment := range response.Segments {
		segment.Text = cleanASRText(segment.Text)
		if segment.Text == "" {
			continue
		}
		if segment.StartMs < 0 || segment.EndMs <= segment.StartMs || segment.EndMs > durationMs {
			return WorkspaceASRDraft{}, fmt.Errorf("server ASR returned an invalid transcript segment")
		}
		segments = append(segments, segment)
	}
	return WorkspaceASRDraft{
		Text:        cleanASRText(response.Text),
		Segments:    segments,
		SourceInMs:  input.SourceInMs,
		SourceOutMs: input.SourceOutMs,
		TimeBase:    response.TimeBase,
		GeneratedAt: time.Now(),
	}, nil
}

func cleanASRText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, character := range value {
		if unicode.IsPunct(character) {
			continue
		}
		builder.WriteRune(character)
	}
	return strings.TrimSpace(builder.String())
}

func (w *Workspace) recordASRFailure(itemID string, snapshot WorkspaceItem, failure error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	current, ok := w.items[itemID]
	if !ok || !sameASRSelection(current, snapshot) {
		return
	}
	current.LastError = failure.Error()
	current.UpdatedAt = time.Now()
	w.items[itemID] = current
	_ = w.persistLocked()
}

func sameASRSelection(current WorkspaceItem, snapshot WorkspaceItem) bool {
	return current.SourceType == snapshot.SourceType &&
		current.SourcePath == snapshot.SourcePath &&
		current.SourceInMs == snapshot.SourceInMs &&
		current.SourceOutMs == snapshot.SourceOutMs
}

func invalidateASRDraft(item *WorkspaceItem) {
	if item.ASRDraft == nil {
		return
	}
	if item.SourceType != "talking_head" ||
		item.ASRDraft.TimeBase != asrDraftTimeBase ||
		item.ASRDraft.SourceInMs != item.SourceInMs ||
		item.ASRDraft.SourceOutMs != item.SourceOutMs {
		item.ASRDraft = nil
	}
}

func cloneASRDraft(source *WorkspaceASRDraft) *WorkspaceASRDraft {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Segments = append([]WorkspaceTranscriptSegment(nil), source.Segments...)
	return &clone
}

func absDuration(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
