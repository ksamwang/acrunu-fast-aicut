package modelgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFunASRClientTranscribeForwardsAudioAndNormalizesSentences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/transcriptions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read audio part: %v", err)
		}
		defer file.Close()
		if header.Filename != "测试音频.wav" {
			t.Fatalf("unexpected filename %q", header.Filename)
		}
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read audio: %v", err)
		}
		if string(contents) != "wave-data" {
			t.Fatalf("unexpected audio contents %q", contents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"text":"第一句。第二句。",
			"timestamp":[[100,200],[5100,5300]],
			"sentence_info":[
				{"text":"第二句。","start":2800,"end":5300},
				{"text":"第一句。","start":100,"end":2500}
			]
		}`)
	}))
	defer server.Close()

	client := NewFunASRClient(server.URL, time.Second)
	result, err := client.Transcribe(context.Background(), FunASRTranscriptionInput{
		Filename:   `C:\素材\测试音频.wav`,
		Audio:      strings.NewReader("wave-data"),
		DurationMs: 5000,
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if result.Text != "第一句。第二句。" {
		t.Fatalf("unexpected text %q", result.Text)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", result.Segments)
	}
	if result.Segments[0].StartMs != 100 || result.Segments[0].EndMs != 2500 || result.Segments[0].Text != "第一句。" {
		t.Fatalf("unexpected first segment %#v", result.Segments[0])
	}
	if result.Segments[1].StartMs != 2800 || result.Segments[1].EndMs != 5000 || result.Segments[1].Text != "第二句。" {
		t.Fatalf("unexpected clamped second segment %#v", result.Segments[1])
	}
}

func TestFunASRClientFallsBackToTopLevelTextAndTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"只有完整文本","timestamp":[[320,600],[600,2480]],"sentence_info":[]}`)
	}))
	defer server.Close()

	result, err := NewFunASRClient(server.URL, time.Second).Transcribe(context.Background(), FunASRTranscriptionInput{
		Filename:   "audio.wav",
		Audio:      strings.NewReader("wave-data"),
		DurationMs: 3000,
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if len(result.Segments) != 1 || result.Segments[0].StartMs != 320 || result.Segments[0].EndMs != 2480 {
		t.Fatalf("unexpected fallback segments %#v", result.Segments)
	}
}

func TestFunASRClientClassifiesUpstreamErrors(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		timeout  time.Duration
		expected FunASRErrorKind
	}{
		{
			name: "unavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			timeout:  time.Second,
			expected: FunASRErrorUnavailable,
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "<html>bad gateway</html>")
			},
			timeout:  time.Second,
			expected: FunASRErrorBadResponse,
		},
		{
			name: "timeout",
			handler: func(http.ResponseWriter, *http.Request) {
				time.Sleep(150 * time.Millisecond)
			},
			timeout:  20 * time.Millisecond,
			expected: FunASRErrorTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			_, err := NewFunASRClient(server.URL, tt.timeout).Transcribe(context.Background(), FunASRTranscriptionInput{
				Filename:   "audio.wav",
				Audio:      strings.NewReader("wave-data"),
				DurationMs: 1000,
			})
			var asrErr *FunASRError
			if !errors.As(err, &asrErr) {
				t.Fatalf("expected FunASRError, got %T %v", err, err)
			}
			if asrErr.Kind != tt.expected {
				t.Fatalf("expected error kind %s, got %s", tt.expected, asrErr.Kind)
			}
		})
	}
}

func TestFunASRClientClassifiesConnectionFailureAsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	_, err := NewFunASRClient(baseURL, time.Second).Transcribe(context.Background(), FunASRTranscriptionInput{
		Filename:   "audio.wav",
		Audio:      strings.NewReader("wave-data"),
		DurationMs: 1000,
	})
	var asrErr *FunASRError
	if !errors.As(err, &asrErr) || asrErr.Kind != FunASRErrorUnavailable {
		t.Fatalf("expected unavailable error, got %T %v", err, err)
	}
}
