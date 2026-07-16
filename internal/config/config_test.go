package config

import (
	"testing"
	"time"
)

func TestLoadReadsASRConfiguration(t *testing.T) {
	t.Setenv("ASR_BASE_URL", "http://asr.test:10096")
	t.Setenv("ASR_REQUEST_TIMEOUT_SECONDS", "45")

	cfg := Load()
	if cfg.ASRBaseURL != "http://asr.test:10096" {
		t.Fatalf("expected ASR base URL from environment, got %q", cfg.ASRBaseURL)
	}
	if cfg.ASRRequestTimeout != 45*time.Second {
		t.Fatalf("expected 45 second ASR timeout, got %s", cfg.ASRRequestTimeout)
	}
}

func TestLoadUsesASRDefaults(t *testing.T) {
	t.Setenv("ASR_BASE_URL", "")
	t.Setenv("ASR_REQUEST_TIMEOUT_SECONDS", "")

	cfg := Load()
	if cfg.ASRBaseURL != "http://127.0.0.1:10096" {
		t.Fatalf("unexpected default ASR base URL %q", cfg.ASRBaseURL)
	}
	if cfg.ASRRequestTimeout != 300*time.Second {
		t.Fatalf("unexpected default ASR timeout %s", cfg.ASRRequestTimeout)
	}
}

func TestLoadReadsTTSConfiguration(t *testing.T) {
	t.Setenv("TTS_BASE_URL", "http://tts.test:50000")
	t.Setenv("TTS_REQUEST_TIMEOUT_SECONDS", "75")

	cfg := Load()
	if cfg.TTSBaseURL != "http://tts.test:50000" {
		t.Fatalf("expected TTS base URL from environment, got %q", cfg.TTSBaseURL)
	}
	if cfg.TTSRequestTimeout != 75*time.Second {
		t.Fatalf("expected TTS timeout from environment, got %s", cfg.TTSRequestTimeout)
	}
}

func TestLoadUsesTTSDefaults(t *testing.T) {
	t.Setenv("TTS_BASE_URL", "")
	t.Setenv("TTS_REQUEST_TIMEOUT_SECONDS", "")

	cfg := Load()
	if cfg.TTSBaseURL != "http://127.0.0.1:50000" {
		t.Fatalf("unexpected default TTS base URL %q", cfg.TTSBaseURL)
	}
	if cfg.TTSRequestTimeout != 300*time.Second {
		t.Fatalf("unexpected default TTS timeout %s", cfg.TTSRequestTimeout)
	}
}

func TestLoadUsesDefaultModelGatewayTimeout(t *testing.T) {
	t.Setenv("MODEL_GATEWAY_TIMEOUT_SECONDS", "")

	cfg := Load()
	if cfg.ModelGatewayTimeout != 300*time.Second {
		t.Fatalf("unexpected default model gateway timeout %s", cfg.ModelGatewayTimeout)
	}
}
