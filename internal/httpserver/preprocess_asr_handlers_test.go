package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type asrTestFile struct {
	fieldName string
	filename  string
	contents  []byte
}

func TestPreprocessASRTranscribeReturnsNormalizedSelectionRelativeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		if r.URL.Path != "/v1/transcriptions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read upstream audio: %v", err)
		}
		defer file.Close()
		if header.Filename != "选区.wav" {
			t.Fatalf("unexpected upstream filename %q", header.Filename)
		}
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upstream audio contents: %v", err)
		}
		if string(contents) != "wave-data" {
			t.Fatalf("unexpected upstream audio %q", contents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"text":"第一句。第二句。",
			"timestamp":[[100,200],[5000,5200]],
			"sentence_info":[
				{"text":"第一句。","start":100,"end":2200},
				{"text":"第二句。","start":2600,"end":5200}
			]
		}`)
	}))
	defer upstream.Close()

	storageRoot := t.TempDir()
	productAssetService := services.NewProductAssetService()
	taskService := services.NewTaskService(storageRoot)
	server := New(Options{
		Config: config.Config{
			StorageRoot:       storageRoot,
			QueueBackend:      "file",
			ASRBaseURL:        upstream.URL,
			ASRRequestTimeout: time.Second,
		},
		ProductAssetService: productAssetService,
		TaskService:         taskService,
	})

	recorder := performASRRequest(t, server, map[string]string{
		"source_in_ms":  "3000",
		"source_out_ms": "12000",
	}, []asrTestFile{{fieldName: "file", filename: "选区.wav", contents: []byte("wave-data")}}, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !upstreamCalled {
		t.Fatal("expected FunASR upstream to be called")
	}

	var response struct {
		Data struct {
			Text        string `json:"text"`
			SourceInMs  int    `json:"source_in_ms"`
			SourceOutMs int    `json:"source_out_ms"`
			TimeBase    string `json:"time_base"`
			Segments    []struct {
				StartMs int    `json:"start_ms"`
				EndMs   int    `json:"end_ms"`
				Text    string `json:"text"`
			} `json:"segments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Text != "第一句。第二句。" || response.Data.SourceInMs != 3000 || response.Data.SourceOutMs != 12000 {
		t.Fatalf("unexpected response data %#v", response.Data)
	}
	if response.Data.TimeBase != preprocessASRTimeBase {
		t.Fatalf("unexpected time base %q", response.Data.TimeBase)
	}
	if len(response.Data.Segments) != 2 || response.Data.Segments[0].StartMs != 100 || response.Data.Segments[1].EndMs != 5200 {
		t.Fatalf("unexpected segments %#v", response.Data.Segments)
	}
	if len(productAssetService.ListAssets(services.AssetFilters{})) != 0 {
		t.Fatal("preprocess ASR must not create assets")
	}
	tasks, err := taskService.ListTasks(context.Background(), services.TaskFilters{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("preprocess ASR must not create tasks, got %#v", tasks)
	}
}

func TestPreprocessASRTranscribeRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalled = true
	}))
	defer upstream.Close()

	server := New(Options{Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file", ASRBaseURL: upstream.URL}})
	recorder := performASRRequest(t, server, map[string]string{
		"source_in_ms": "0", "source_out_ms": "1000",
	}, []asrTestFile{{fieldName: "file", filename: "audio.wav", contents: []byte("wave-data")}}, false)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if upstreamCalled {
		t.Fatal("unauthenticated request must not reach FunASR")
	}
}

func TestPreprocessASRTranscribeValidatesInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid request must not reach FunASR")
	}))
	defer upstream.Close()
	server := New(Options{Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file", ASRBaseURL: upstream.URL}})

	tests := []struct {
		name   string
		fields map[string]string
		files  []asrTestFile
	}{
		{name: "missing out point", fields: map[string]string{"source_in_ms": "0"}, files: []asrTestFile{{fieldName: "file", filename: "audio.wav", contents: []byte("data")}}},
		{name: "negative in point", fields: map[string]string{"source_in_ms": "-1", "source_out_ms": "1000"}, files: []asrTestFile{{fieldName: "file", filename: "audio.wav", contents: []byte("data")}}},
		{name: "empty range", fields: map[string]string{"source_in_ms": "1000", "source_out_ms": "1000"}, files: []asrTestFile{{fieldName: "file", filename: "audio.wav", contents: []byte("data")}}},
		{name: "empty audio", fields: map[string]string{"source_in_ms": "0", "source_out_ms": "1000"}, files: []asrTestFile{{fieldName: "file", filename: "audio.wav"}}},
		{name: "multiple files", fields: map[string]string{"source_in_ms": "0", "source_out_ms": "1000"}, files: []asrTestFile{
			{fieldName: "file", filename: "audio.wav", contents: []byte("data")},
			{fieldName: "extra", filename: "extra.wav", contents: []byte("data")},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performASRRequest(t, server, tt.fields, tt.files, true)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPreprocessASRTranscribeMapsUpstreamErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		handler      http.HandlerFunc
		timeout      time.Duration
		expectedCode int
		errorCode    string
	}{
		{
			name: "unavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			timeout: time.Second, expectedCode: http.StatusServiceUnavailable, errorCode: "asr_unavailable",
		},
		{
			name: "bad response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "not-json")
			},
			timeout: time.Second, expectedCode: http.StatusBadGateway, errorCode: "asr_invalid_response",
		},
		{
			name: "timeout",
			handler: func(http.ResponseWriter, *http.Request) {
				time.Sleep(150 * time.Millisecond)
			},
			timeout: 20 * time.Millisecond, expectedCode: http.StatusGatewayTimeout, errorCode: "asr_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(tt.handler)
			defer upstream.Close()
			server := New(Options{Config: config.Config{
				StorageRoot: t.TempDir(), QueueBackend: "file", ASRBaseURL: upstream.URL, ASRRequestTimeout: tt.timeout,
			}})
			recorder := performASRRequest(t, server, map[string]string{
				"source_in_ms": "0", "source_out_ms": "1000",
			}, []asrTestFile{{fieldName: "file", filename: "audio.wav", contents: []byte("data")}}, true)
			if recorder.Code != tt.expectedCode || !strings.Contains(recorder.Body.String(), `"code":"`+tt.errorCode+`"`) {
				t.Fatalf("expected %d/%s, got %d body=%s", tt.expectedCode, tt.errorCode, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPreprocessASRTranscribeCleansMultipartTemporaryFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"测试","sentence_info":[{"text":"测试","start":0,"end":1000}]}`)
	}))
	defer upstream.Close()
	server := New(Options{Config: config.Config{
		StorageRoot: t.TempDir(), QueueBackend: "file", ASRBaseURL: upstream.URL, ASRRequestTimeout: time.Second,
	}})

	recorder := performASRRequest(t, server, map[string]string{
		"source_in_ms": "0", "source_out_ms": "1000",
	}, []asrTestFile{{fieldName: "file", filename: "large.wav", contents: bytes.Repeat([]byte{1}, preprocessASRMemoryBytes+1)}}, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoMultipartTempFiles(t, tempDir)
}

func TestPreprocessASRTranscribeRejectsOversizedRequestAndCleansTemporaryFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	server := New(Options{Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file", ASRBaseURL: "http://127.0.0.1:1"}})

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_ = writer.WriteField("source_in_ms", "0")
		_ = writer.WriteField("source_out_ms", "1000")
		part, err := writer.CreateFormFile("file", "oversized.wav")
		if err == nil {
			_, _ = io.CopyN(part, zeroReader{}, preprocessASRMaxRequestBytes+1)
		}
		_ = writer.Close()
		_ = pipeWriter.Close()
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/preprocess/asr-transcribe", pipeReader)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", asrUserAuthHeader())
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)
	_ = pipeReader.Close()
	select {
	case <-writeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("oversized request writer did not stop")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoMultipartTempFiles(t, tempDir)
}

func performASRRequest(t *testing.T, server *Server, fields map[string]string, files []asrTestFile, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.fieldName, file.filename)
		if err != nil {
			t.Fatalf("create file %s: %v", file.fieldName, err)
		}
		if _, err := part.Write(file.contents); err != nil {
			t.Fatalf("write file %s: %v", file.fieldName, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/preprocess/asr-transcribe", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if authenticated {
		req.Header.Set("Authorization", asrUserAuthHeader())
	}
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)
	return recorder
}

func asrUserAuthHeader() string {
	return "Bearer " + makeDevToken(auth.User{
		ID: "asr-user", Username: "asr-user", DisplayName: "ASR User", Role: auth.RoleUser,
	})
}

func assertNoMultipartTempFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read temp directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "multipart-") {
			t.Fatalf("multipart temporary file was not removed: %s", entry.Name())
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
