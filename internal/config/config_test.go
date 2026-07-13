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
