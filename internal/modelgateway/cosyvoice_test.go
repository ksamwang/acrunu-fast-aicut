package modelgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCosyVoiceClientSynthesizeForwardsZeroShotInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/synthesize" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("text"); got != "生成旁白" {
			t.Fatalf("unexpected text %q", got)
		}
		if got := r.FormValue("mode"); got != "zero_shot" {
			t.Fatalf("unexpected mode %q", got)
		}
		if got := r.FormValue("prompt_text"); got != "参考文本" {
			t.Fatalf("unexpected prompt text %q", got)
		}
		var units []CosyVoiceSynthesisUnit
		if err := json.Unmarshal([]byte(r.FormValue("segments_json")), &units); err != nil {
			t.Fatalf("decode synthesis units: %v", err)
		}
		if len(units) != 2 || units[0].Text != "生成" || units[0].PauseAfterMs != 120 || units[1].Text != "旁白" || units[1].PauseAfterMs != 0 {
			t.Fatalf("unexpected synthesis units %#v", units)
		}
		file, header, err := r.FormFile("prompt_audio")
		if err != nil {
			t.Fatalf("read prompt audio: %v", err)
		}
		defer file.Close()
		if header.Filename != "reference.wav" {
			t.Fatalf("unexpected filename %q", header.Filename)
		}
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read prompt audio: %v", err)
		}
		if string(contents) != "reference-audio" {
			t.Fatalf("unexpected prompt audio %q", contents)
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-AICUT-TTS-Model", "Fun-CosyVoice3-test")
		w.Header().Set("X-AICUT-TTS-Sample-Rate", "24000")
		w.Header().Set("X-AICUT-TTS-Timing-Version", "1")
		w.Header().Set("X-AICUT-TTS-Speech-Samples", "12000,18000")
		w.Header().Set("X-AICUT-TTS-Unit-Samples", "14880,18000")
		_, _ = w.Write([]byte("wav-audio"))
	}))
	defer server.Close()

	result, err := NewCosyVoiceClient(server.URL, time.Second).Synthesize(context.Background(), CosyVoiceSynthesisInput{
		Text:                "生成旁白",
		PromptAudio:         strings.NewReader("reference-audio"),
		PromptAudioFilename: `C:\voice\reference.wav`,
		PromptText:          "参考文本",
		Units: []CosyVoiceSynthesisUnit{
			{Text: "生成", PauseAfterMs: 120},
			{Text: "旁白"},
		},
	})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if string(result.Audio) != "wav-audio" || result.Model != "Fun-CosyVoice3-test" || result.SampleRate != 24000 {
		t.Fatalf("unexpected result %#v", result)
	}
	if len(result.Units) != 2 || result.Units[0].SpeechSamples != 12000 || result.Units[0].TotalSamples != 14880 || result.Units[1].TotalSamples != 18000 {
		t.Fatalf("unexpected segmented timing %#v", result.Units)
	}
}

func TestCosyVoiceClientRejectsMissingSegmentTiming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-AICUT-TTS-Sample-Rate", "24000")
		_, _ = w.Write([]byte("wav-audio"))
	}))
	defer server.Close()

	_, err := NewCosyVoiceClient(server.URL, time.Second).Synthesize(context.Background(), CosyVoiceSynthesisInput{
		Text: "生成旁白", PromptAudio: strings.NewReader("reference-audio"), PromptText: "参考文本",
		Units: []CosyVoiceSynthesisUnit{{Text: "生成旁白"}},
	})
	var cosyErr *CosyVoiceError
	if !errors.As(err, &cosyErr) || cosyErr.Kind != CosyVoiceErrorBadResponse || !strings.Contains(err.Error(), "bad_response") {
		t.Fatalf("expected missing timing to be rejected, got %T %v", err, err)
	}
}

func TestCosyVoiceClientClassifiesUnavailableAndInvalidResponses(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		expected CosyVoiceErrorKind
	}{
		{
			name: "unavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expected: CosyVoiceErrorUnavailable,
		},
		{
			name: "invalid response type",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"error":"bad"}`))
			},
			expected: CosyVoiceErrorBadResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			_, err := NewCosyVoiceClient(server.URL, time.Second).Synthesize(context.Background(), CosyVoiceSynthesisInput{
				Text:        "测试",
				PromptAudio: strings.NewReader("reference-audio"),
				PromptText:  "参考文本",
			})
			var cosyErr *CosyVoiceError
			if !errors.As(err, &cosyErr) || cosyErr.Kind != tt.expected {
				t.Fatalf("expected %s, got %T %v", tt.expected, err, err)
			}
		})
	}
}
