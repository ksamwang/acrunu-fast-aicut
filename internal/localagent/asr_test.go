package localagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

type asrRecordingProcessor struct {
	stubProcessor

	mu                 sync.Mutex
	extractSourcePath  string
	extractOutputPath  string
	extractSourceInMs  int
	extractSourceOutMs int
	extractCalls       int
	extractErr         error
	audioDurationMs    int
	sourceHasAudio     bool
}

func newASRRecordingProcessor() *asrRecordingProcessor {
	return &asrRecordingProcessor{sourceHasAudio: true}
}

func (p *asrRecordingProcessor) ExtractAudio(_ context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error {
	p.mu.Lock()
	p.extractSourcePath = sourcePath
	p.extractOutputPath = outputPath
	p.extractSourceInMs = sourceInMs
	p.extractSourceOutMs = sourceOutMs
	p.extractCalls++
	extractErr := p.extractErr
	p.mu.Unlock()
	if extractErr != nil {
		return extractErr
	}
	return os.WriteFile(outputPath, append(make([]byte, 44), []byte("audio-data")...), 0644)
}

func (p *asrRecordingProcessor) Probe(ctx context.Context, path string) (ffmpeg.ProbeResult, error) {
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		p.mu.Lock()
		durationMs := p.audioDurationMs
		if durationMs == 0 {
			durationMs = p.extractSourceOutMs - p.extractSourceInMs
		}
		p.mu.Unlock()
		return ffmpeg.ProbeResult{
			DurationMs:    durationMs,
			HasAudio:      true,
			AudioCodec:    "pcm_s16le",
			AudioChannels: 1,
		}, nil
	}
	result, err := p.stubProcessor.Probe(ctx, path)
	result.HasAudio = p.sourceHasAudio
	if !result.HasAudio {
		result.AudioCodec = ""
		result.AudioChannels = 0
	}
	return result, err
}

func (p *asrRecordingProcessor) extraction() (string, string, int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.extractSourcePath, p.extractOutputPath, p.extractSourceInMs, p.extractSourceOutMs, p.extractCalls
}

func TestWorkspaceTranscribeItemPersistsSelectionRelativeDraftWithoutOverwritingTranscript(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工确认文本")

	asrServer := newLocalAgentASRServer(t, func(r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse ASR multipart: %v", err)
		}
		if r.FormValue("source_in_ms") != "1000" || r.FormValue("source_out_ms") != "5000" {
			t.Fatalf("unexpected ASR range %s-%s", r.FormValue("source_in_ms"), r.FormValue("source_out_ms"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read ASR audio part: %v", err)
		}
		defer file.Close()
		if header.Filename != "selection.wav" {
			t.Fatalf("unexpected ASR filename %q", header.Filename)
		}
		contents, err := io.ReadAll(file)
		if err != nil || len(contents) <= 44 {
			t.Fatalf("expected non-empty wav upload, bytes=%d err=%v", len(contents), err)
		}
	}, asrServerResponse{
		Text: "识别草稿",
		Segments: []WorkspaceTranscriptSegment{
			{StartMs: 120, EndMs: 1800, Text: "第一句"},
			{StartMs: 1900, EndMs: 3800, Text: "第二句"},
		},
		SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
	})
	defer asrServer.Close()

	result, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: asrServer.URL, AuthToken: "test-token",
	})
	if err != nil {
		t.Fatalf("TranscribeItem: %v", err)
	}
	if result.SourceInMs != 1000 || result.SourceOutMs != 5000 {
		t.Fatalf("ASR changed I/O to %d-%d", result.SourceInMs, result.SourceOutMs)
	}
	if result.Transcript != "人工确认文本" {
		t.Fatalf("ASR overwrote confirmed transcript with %q", result.Transcript)
	}
	if result.ASRDraft == nil || result.ASRDraft.Text != "识别草稿" || result.ASRDraft.TimeBase != asrDraftTimeBase {
		t.Fatalf("unexpected ASR draft %#v", result.ASRDraft)
	}
	if len(result.ASRDraft.Segments) != 2 || result.ASRDraft.Segments[0].StartMs != 120 || result.ASRDraft.Segments[1].EndMs != 3800 {
		t.Fatalf("unexpected ASR segments %#v", result.ASRDraft.Segments)
	}
	if result.ASRDraft.GeneratedAt.IsZero() {
		t.Fatal("expected ASR draft generation time")
	}
	if result.CleanShotPath != "" {
		t.Fatal("ASR must not create a clean shot")
	}

	sourcePath, outputPath, sourceInMs, sourceOutMs, calls := processor.extraction()
	if sourcePath != item.SourcePath || sourceInMs != 1000 || sourceOutMs != 5000 || calls != 1 {
		t.Fatalf("unexpected extraction source=%q range=%d-%d calls=%d", sourcePath, sourceInMs, sourceOutMs, calls)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("temporary wav still exists at %s, err=%v", outputPath, err)
	}
	assertASRTempRootEmpty(t, root)
	assertNoSRTFiles(t, root)

	reloaded, err := NewWorkspace(root, processor)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	persisted, ok := reloaded.GetItem(item.ID)
	if !ok || persisted.ASRDraft == nil || persisted.ASRDraft.Text != "识别草稿" {
		t.Fatalf("ASR draft was not restored after restart: %#v", persisted.ASRDraft)
	}
}

func TestWorkspaceASRDraftCleansPunctuationBeforeStorage(t *testing.T) {
	draft, err := workspaceASRDraftFromResponse(asrServerResponse{
		Text: "Hello, world. Test!",
		Segments: []WorkspaceTranscriptSegment{
			{StartMs: 100, EndMs: 800, Text: "Hello, world."},
			{StartMs: 900, EndMs: 1500, Text: "Test!"},
		},
		SourceInMs:  0,
		SourceOutMs: 2000,
		TimeBase:    asrDraftTimeBase,
	}, WorkspaceASRTranscribeInput{SourceInMs: 0, SourceOutMs: 2000})
	if err != nil {
		t.Fatalf("build ASR draft: %v", err)
	}
	if draft.Text != "Hello world Test" {
		t.Fatalf("unexpected cleaned text %q", draft.Text)
	}
	if draft.Segments[0].Text != "Hello world" || draft.Segments[1].Text != "Test" {
		t.Fatalf("unexpected cleaned segments %#v", draft.Segments)
	}
}

func TestWorkspaceTranscribeItemRejectsInvalidWorkspaceStateBeforeExtraction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WorkspaceItem, *asrRecordingProcessor)
		input  WorkspaceASRTranscribeInput
	}{
		{
			name: "visual only",
			mutate: func(item *WorkspaceItem, _ *asrRecordingProcessor) {
				item.SourceType = "visual_only"
			},
			input: WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: "http://server", AuthToken: "token"},
		},
		{
			name: "source has no audio",
			mutate: func(item *WorkspaceItem, _ *asrRecordingProcessor) {
				item.Probe.HasAudio = false
			},
			input: WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: "http://server", AuthToken: "token"},
		},
		{
			name:   "request range differs from workspace",
			mutate: func(*WorkspaceItem, *asrRecordingProcessor) {},
			input:  WorkspaceASRTranscribeInput{SourceInMs: 2000, SourceOutMs: 5000, ServerBaseURL: "http://server", AuthToken: "token"},
		},
		{
			name:   "range exceeds source duration",
			mutate: func(*WorkspaceItem, *asrRecordingProcessor) {},
			input:  WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 7000, ServerBaseURL: "http://server", AuthToken: "token"},
		},
		{
			name:   "missing server URL",
			mutate: func(*WorkspaceItem, *asrRecordingProcessor) {},
			input:  WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 5000, AuthToken: "token"},
		},
		{
			name:   "missing auth token",
			mutate: func(*WorkspaceItem, *asrRecordingProcessor) {},
			input:  WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: "http://server"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := newASRRecordingProcessor()
			workspace, item := newTalkingHeadWorkspaceItem(t, t.TempDir(), processor, 1000, 5000, "人工文本")
			workspace.mu.Lock()
			current := workspace.items[item.ID]
			tt.mutate(&current, processor)
			workspace.items[item.ID] = current
			workspace.mu.Unlock()

			if _, err := workspace.TranscribeItem(context.Background(), item.ID, tt.input); err == nil {
				t.Fatal("expected validation error")
			}
			_, _, _, _, calls := processor.extraction()
			if calls != 0 {
				t.Fatalf("invalid workspace state triggered %d extraction calls", calls)
			}
		})
	}
}

func TestWorkspaceTranscribeItemRejectsExtractedAudioDurationMismatch(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	processor.audioDurationMs = 3000
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")

	_, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: "http://server", AuthToken: "token",
	})
	if err == nil || !strings.Contains(err.Error(), "duration mismatch") {
		t.Fatalf("expected duration mismatch, got %v", err)
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceTranscribeItemCleansTemporaryAudioWhenExtractionFails(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	processor.extractErr = errors.New("ffmpeg failed")
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")

	_, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: "http://server", AuthToken: "token",
	})
	if err == nil || !strings.Contains(err.Error(), "ffmpeg failed") {
		t.Fatalf("expected extraction failure, got %v", err)
	}
	_, _, _, _, calls := processor.extraction()
	if calls != 1 {
		t.Fatalf("expected one extraction call, got %d", calls)
	}
	current, _ := workspace.GetItem(item.ID)
	if current.LastError == "" {
		t.Fatal("expected extraction failure to be recorded")
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceTranscribeItemCleansTemporaryAudioWhenContextIsCanceled(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")
	asrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(150 * time.Millisecond)
		writeLocalAgentASRResponse(t, w, asrServerResponse{
			Text: "不会返回", Segments: []WorkspaceTranscriptSegment{{StartMs: 0, EndMs: 1000, Text: "不会返回"}},
			SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
		})
	}))
	defer asrServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := workspace.TranscribeItem(ctx, item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: asrServer.URL, AuthToken: "token",
	}); err == nil {
		t.Fatal("expected canceled ASR request")
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceTranscribeItemRejectsServerRangeMismatchWithoutReplacingDraft(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")
	asrServer := newLocalAgentASRServer(t, nil, asrServerResponse{
		Text: "错误范围", Segments: []WorkspaceTranscriptSegment{{StartMs: 0, EndMs: 1000, Text: "错误范围"}},
		SourceInMs: 0, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
	})
	defer asrServer.Close()
	if _, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: asrServer.URL, AuthToken: "token",
	}); err == nil || !strings.Contains(err.Error(), "range does not match") {
		t.Fatalf("expected server range mismatch, got %v", err)
	}
	current, _ := workspace.GetItem(item.ID)
	if current.ASRDraft != nil || current.Transcript != "人工文本" || current.LastError == "" {
		t.Fatalf("range mismatch changed draft or transcript: %#v", current)
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceTranscribeItemDoesNotLetOldResultOverwriteChangedSelection(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	asrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(requestStarted)
		<-releaseResponse
		writeLocalAgentASRResponse(t, w, asrServerResponse{
			Text:       "旧选区结果",
			Segments:   []WorkspaceTranscriptSegment{{StartMs: 100, EndMs: 1000, Text: "旧选区"}},
			SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
		})
	}))
	defer asrServer.Close()

	resultCh := make(chan error, 1)
	go func() {
		_, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
			SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: asrServer.URL, AuthToken: "token",
		})
		resultCh <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("ASR request did not start")
	}
	if _, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: 2000, SourceOutMs: 5000, Transcript: "人工文本",
	}); err != nil {
		t.Fatalf("change I/O while ASR is running: %v", err)
	}
	close(releaseResponse)
	if err := <-resultCh; !errors.Is(err, ErrASRSelectionChanged) {
		t.Fatalf("expected selection changed error, got %v", err)
	}
	current, _ := workspace.GetItem(item.ID)
	if current.SourceInMs != 2000 || current.SourceOutMs != 5000 || current.ASRDraft != nil {
		t.Fatalf("old ASR result changed current selection or draft: %#v", current)
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceTranscribeItemPreservesExistingDraftAndTranscriptOnServerFailure(t *testing.T) {
	root := t.TempDir()
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, root, processor, 1000, 5000, "人工文本")
	successServer := newLocalAgentASRServer(t, nil, asrServerResponse{
		Text:       "上一次草稿",
		Segments:   []WorkspaceTranscriptSegment{{StartMs: 100, EndMs: 1000, Text: "上一次"}},
		SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
	})
	if _, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: successServer.URL, AuthToken: "token",
	}); err != nil {
		t.Fatalf("seed ASR draft: %v", err)
	}
	successServer.Close()

	failureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "asr_unavailable", "message": "暂不可用"}})
	}))
	defer failureServer.Close()
	if _, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: failureServer.URL, AuthToken: "token",
	}); err == nil {
		t.Fatal("expected server failure")
	}
	current, _ := workspace.GetItem(item.ID)
	if current.Transcript != "人工文本" || current.ASRDraft == nil || current.ASRDraft.Text != "上一次草稿" {
		t.Fatalf("failure overwrote confirmed text or valid draft: %#v", current)
	}
	if current.LastError == "" {
		t.Fatal("expected ASR failure to be recorded")
	}
	assertASRTempRootEmpty(t, root)
}

func TestWorkspaceSaveClearsDraftWhenSelectionChangesAndKeepsItForTextOnlyEdit(t *testing.T) {
	processor := newASRRecordingProcessor()
	workspace, item := newTalkingHeadWorkspaceItem(t, t.TempDir(), processor, 1000, 5000, "人工文本")
	workspace.mu.Lock()
	current := workspace.items[item.ID]
	current.ASRDraft = &WorkspaceASRDraft{
		Text: "草稿", Segments: []WorkspaceTranscriptSegment{{StartMs: 0, EndMs: 1000, Text: "草稿"}},
		SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase, GeneratedAt: time.Now(),
	}
	workspace.items[item.ID] = current
	workspace.mu.Unlock()

	textEdited, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: 1000, SourceOutMs: 5000, Transcript: "修改后的人工文本",
	})
	if err != nil {
		t.Fatalf("save text-only edit: %v", err)
	}
	if textEdited.ASRDraft == nil {
		t.Fatal("text-only edit should keep range-bound ASR draft")
	}
	rangeEdited, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: 2000, SourceOutMs: 5000, Transcript: "修改后的人工文本",
	})
	if err != nil {
		t.Fatalf("save range edit: %v", err)
	}
	if rangeEdited.ASRDraft != nil {
		t.Fatal("range edit should clear stale ASR draft")
	}
}

func TestLocalAgentTranscribeRouteReturnsEnrichedDraft(t *testing.T) {
	processor := newASRRecordingProcessor()
	server := New(Options{WorkspaceRoot: t.TempDir(), Processor: processor})
	header, cleanup := newMultipartHeader(t, "口播.mp4", []byte("video"))
	defer cleanup()
	items, err := server.workspace.ImportFiles(context.Background(), []*multipart.FileHeader{header})
	if err != nil {
		t.Fatalf("import workspace file: %v", err)
	}
	item, err := server.workspace.SaveItem(context.Background(), items[0].ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: 1000, SourceOutMs: 5000, Transcript: "人工文本",
	})
	if err != nil {
		t.Fatalf("save talking head: %v", err)
	}
	asrServer := newLocalAgentASRServer(t, nil, asrServerResponse{
		Text: "接口草稿", Segments: []WorkspaceTranscriptSegment{{StartMs: 100, EndMs: 1000, Text: "接口草稿"}},
		SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase,
	})
	defer asrServer.Close()

	body, _ := json.Marshal(WorkspaceASRTranscribeInput{
		SourceInMs: 1000, SourceOutMs: 5000, ServerBaseURL: asrServer.URL, AuthToken: "token",
	})
	req := httptest.NewRequest(http.MethodPost, "/workspace/items/"+item.ID+"/transcribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.withMiddleware(http.HandlerFunc(server.handleWorkspaceItemRoute)).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Item struct {
			SourceInMs  int                `json:"source_in_ms"`
			SourceOutMs int                `json:"source_out_ms"`
			Transcript  string             `json:"transcript"`
			ASRDraft    *WorkspaceASRDraft `json:"asr_draft"`
		} `json:"item"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode local-agent response: %v", err)
	}
	if response.Item.ASRDraft == nil || response.Item.ASRDraft.Text != "接口草稿" || response.Item.Transcript != "人工文本" {
		t.Fatalf("unexpected enriched item %#v", response.Item)
	}
	if response.Item.SourceInMs != 1000 || response.Item.SourceOutMs != 5000 {
		t.Fatalf("route changed I/O to %d-%d", response.Item.SourceInMs, response.Item.SourceOutMs)
	}
}

func TestTranscribeAudioOnServerBypassesEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	audioPath := filepath.Join(t.TempDir(), "selection.wav")
	if err := os.WriteFile(audioPath, append(make([]byte, 44), []byte("audio")...), 0644); err != nil {
		t.Fatalf("write test audio: %v", err)
	}
	asrServer := newLocalAgentASRServer(t, nil, asrServerResponse{
		Text: "直连成功", Segments: []WorkspaceTranscriptSegment{{StartMs: 0, EndMs: 1000, Text: "直连成功"}},
		SourceInMs: 0, SourceOutMs: 1000, TimeBase: asrDraftTimeBase,
	})
	defer asrServer.Close()
	response, err := transcribeAudioOnServer(context.Background(), WorkspaceASRTranscribeInput{
		SourceInMs: 0, SourceOutMs: 1000, ServerBaseURL: asrServer.URL, AuthToken: "token",
	}, audioPath)
	if err != nil {
		t.Fatalf("expected direct ASR request to bypass environment proxy: %v", err)
	}
	if response.Text != "直连成功" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestWorkspaceASRDraftRejectsMismatchedRangeTimeBaseAndSegments(t *testing.T) {
	input := WorkspaceASRTranscribeInput{SourceInMs: 1000, SourceOutMs: 5000}
	tests := []asrServerResponse{
		{SourceInMs: 0, SourceOutMs: 5000, TimeBase: asrDraftTimeBase},
		{SourceInMs: 1000, SourceOutMs: 5000, TimeBase: "source_absolute_ms"},
		{SourceInMs: 1000, SourceOutMs: 5000, TimeBase: asrDraftTimeBase, Segments: []WorkspaceTranscriptSegment{{StartMs: 0, EndMs: 5000, Text: "越界"}}},
	}
	for index, response := range tests {
		if _, err := workspaceASRDraftFromResponse(response, input); err == nil {
			t.Fatalf("case %d expected validation error", index)
		}
	}
}

func TestWorkspaceTranscribeConfiguredRealSample(t *testing.T) {
	sourcePath := os.Getenv("AICUT_ASR_TEST_VIDEO")
	serverBaseURL := os.Getenv("AICUT_ASR_SERVER_BASE_URL")
	authToken := os.Getenv("AICUT_ASR_AUTH_TOKEN")
	if sourcePath == "" || serverBaseURL == "" || authToken == "" {
		t.Skip("real ASR sample environment is not configured")
	}

	probe, err := ffmpeg.Probe(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("probe real sample: %v", err)
	}
	if !probe.HasAudio || probe.DurationMs <= 0 {
		t.Fatalf("real sample has no usable audio probe: %#v", probe)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat real sample: %v", err)
	}

	root := t.TempDir()
	workspace, err := NewWorkspace(root, NewProcessor())
	if err != nil {
		t.Fatalf("create real-sample workspace: %v", err)
	}
	now := time.Now()
	item := WorkspaceItem{
		ID:                 "real-asr-sample",
		Status:             workspaceStatusSaved,
		SourceType:         "talking_head",
		OriginalFileName:   filepath.Base(sourcePath),
		OriginalSourcePath: sourcePath,
		OriginalProbe:      probe,
		SourceFileName:     filepath.Base(sourcePath),
		SourceFileSize:     info.Size(),
		SourcePath:         sourcePath,
		SourceInMs:         0,
		SourceOutMs:        probe.DurationMs,
		Transcript:         "人工文本保留验证",
		Probe:              probe,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	workspace.mu.Lock()
	workspace.items[item.ID] = item
	workspace.order = append(workspace.order, item.ID)
	if err := workspace.persistLocked(); err != nil {
		workspace.mu.Unlock()
		t.Fatalf("persist real sample item: %v", err)
	}
	workspace.mu.Unlock()

	result, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: 0, SourceOutMs: probe.DurationMs, ServerBaseURL: serverBaseURL, AuthToken: authToken,
	})
	if err != nil {
		t.Fatalf("transcribe real sample: %v", err)
	}
	if result.SourceInMs != 0 || result.SourceOutMs != probe.DurationMs {
		t.Fatalf("real transcription changed I/O to %d-%d", result.SourceInMs, result.SourceOutMs)
	}
	if result.Transcript != "人工文本保留验证" {
		t.Fatalf("real transcription overwrote manual text with %q", result.Transcript)
	}
	if result.ASRDraft == nil || strings.TrimSpace(result.ASRDraft.Text) == "" || len(result.ASRDraft.Segments) == 0 {
		t.Fatalf("real transcription returned no usable draft: %#v", result.ASRDraft)
	}
	for _, segment := range result.ASRDraft.Segments {
		if segment.StartMs < 0 || segment.EndMs <= segment.StartMs || segment.EndMs > probe.DurationMs {
			t.Fatalf("real transcript segment is out of range: %#v", segment)
		}
	}
	assertASRTempRootEmpty(t, root)
	assertNoSRTFiles(t, root)
	t.Logf("real ASR text=%q segments=%d duration_ms=%d", result.ASRDraft.Text, len(result.ASRDraft.Segments), probe.DurationMs)

	const subrangeInMs = 3000
	const subrangeOutMs = 12000
	updated, err := workspace.SaveItem(context.Background(), item.ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: subrangeInMs, SourceOutMs: subrangeOutMs, Transcript: "人工文本保留验证",
	})
	if err != nil {
		t.Fatalf("save real sample subrange: %v", err)
	}
	if updated.ASRDraft != nil {
		t.Fatal("changing the real sample I/O should invalidate the old draft")
	}
	subrangeResult, err := workspace.TranscribeItem(context.Background(), item.ID, WorkspaceASRTranscribeInput{
		SourceInMs: subrangeInMs, SourceOutMs: subrangeOutMs, ServerBaseURL: serverBaseURL, AuthToken: authToken,
	})
	if err != nil {
		t.Fatalf("transcribe real sample subrange: %v", err)
	}
	if subrangeResult.SourceInMs != subrangeInMs || subrangeResult.SourceOutMs != subrangeOutMs {
		t.Fatalf("subrange transcription changed I/O to %d-%d", subrangeResult.SourceInMs, subrangeResult.SourceOutMs)
	}
	if subrangeResult.ASRDraft == nil || strings.TrimSpace(subrangeResult.ASRDraft.Text) == "" || len(subrangeResult.ASRDraft.Segments) == 0 {
		t.Fatalf("real subrange returned no usable draft: %#v", subrangeResult.ASRDraft)
	}
	for _, segment := range subrangeResult.ASRDraft.Segments {
		if segment.StartMs < 0 || segment.EndMs <= segment.StartMs || segment.EndMs > subrangeOutMs-subrangeInMs {
			t.Fatalf("subrange segment is not selection-relative: %#v", segment)
		}
		absoluteStartMs := subrangeInMs + segment.StartMs
		absoluteEndMs := subrangeInMs + segment.EndMs
		if absoluteStartMs < subrangeInMs || absoluteEndMs > subrangeOutMs {
			t.Fatalf("subrange segment maps outside source I/O: %d-%d", absoluteStartMs, absoluteEndMs)
		}
	}
	assertASRTempRootEmpty(t, root)
	t.Logf("real ASR subrange text=%q segments=%d range=%d-%d", subrangeResult.ASRDraft.Text, len(subrangeResult.ASRDraft.Segments), subrangeInMs, subrangeOutMs)
}

func newTalkingHeadWorkspaceItem(t *testing.T, root string, processor Processor, sourceInMs int, sourceOutMs int, transcript string) (*Workspace, WorkspaceItem) {
	t.Helper()
	workspace, err := NewWorkspace(root, processor)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	header, cleanup := newMultipartHeader(t, "口播素材.mp4", []byte("video"))
	t.Cleanup(cleanup)
	items, err := workspace.ImportFiles(context.Background(), []*multipart.FileHeader{header})
	if err != nil {
		t.Fatalf("ImportFiles: %v", err)
	}
	item, err := workspace.SaveItem(context.Background(), items[0].ID, WorkspaceSaveInput{
		SourceType: "talking_head", SourceInMs: sourceInMs, SourceOutMs: sourceOutMs, Transcript: transcript,
	})
	if err != nil {
		t.Fatalf("SaveItem: %v", err)
	}
	return workspace, item
}

func newLocalAgentASRServer(t *testing.T, inspect func(*http.Request), response asrServerResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/preprocess/asr-transcribe" {
			t.Fatalf("unexpected ASR path %s", r.URL.Path)
		}
		if inspect != nil {
			inspect(r)
		} else {
			_, _ = io.Copy(io.Discard, r.Body)
		}
		writeLocalAgentASRResponse(t, w, response)
	}))
}

func writeLocalAgentASRResponse(t *testing.T, w http.ResponseWriter, response asrServerResponse) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": response}); err != nil {
		t.Fatalf("encode ASR response: %v", err)
	}
}

func assertASRTempRootEmpty(t *testing.T, root string) {
	t.Helper()
	tempRoot := filepath.Join(root, "temp")
	entries, err := os.ReadDir(tempRoot)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read ASR temp root: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("ASR temp root is not empty: %v", names)
	}
}

func assertNoSRTFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".srt") {
			return fmt.Errorf("unexpected SRT file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
