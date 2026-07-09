package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBitsPerSecondStringToKbps(t *testing.T) {
	if got := bitsPerSecondStringToKbps("3200000"); got != 3200 {
		t.Fatalf("expected 3200, got %d", got)
	}
	if got := bitsPerSecondStringToKbps("invalid"); got != 0 {
		t.Fatalf("expected 0 for invalid input, got %d", got)
	}
}

func TestProbeUsesFFProbeOutput(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "probe-output.json")
	scriptPath := filepath.Join(tempDir, "ffprobe-mock.cmd")

	content := `{"streams":[{"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"30000/1001"},{"codec_type":"audio","codec_name":"aac","channels":1,"avg_frame_rate":"0/0"}],"format":{"duration":"2.066000","bit_rate":"3200000"}}`
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		t.Fatalf("write probe output failed: %v", err)
	}
	script := "@echo off\r\ntype \"" + outputPath + "\"\r\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write ffprobe mock failed: %v", err)
	}

	t.Setenv("FFPROBE_PATH", scriptPath)

	result, err := Probe(context.Background(), "demo.mp4")
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if result.DurationMs != 2066 {
		t.Fatalf("expected duration 2066ms, got %d", result.DurationMs)
	}
	if result.Width != 1080 || result.Height != 1920 {
		t.Fatalf("expected 1080x1920, got %dx%d", result.Width, result.Height)
	}
	if result.Codec != "h264" {
		t.Fatalf("expected codec h264, got %s", result.Codec)
	}
	if !result.HasAudio || result.AudioCodec != "aac" {
		t.Fatalf("expected audio aac, got hasAudio=%v codec=%s", result.HasAudio, result.AudioCodec)
	}
	if result.AudioChannels != 1 {
		t.Fatalf("expected mono audio, got %d channels", result.AudioChannels)
	}
	if result.BitrateKbps != 3200 {
		t.Fatalf("expected bitrate 3200 kbps, got %d", result.BitrateKbps)
	}
	if result.FPS < 29.9 || result.FPS > 30.1 {
		t.Fatalf("expected fps close to 29.97, got %.3f", result.FPS)
	}
}

func TestProbeReturnsCommandError(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "ffprobe-fail.cmd")
	script := "@echo off\r\nexit /b 1\r\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write ffprobe fail mock failed: %v", err)
	}

	t.Setenv("FFPROBE_PATH", scriptPath)

	if _, err := Probe(context.Background(), "broken.mp4"); err == nil {
		t.Fatalf("expected probe to fail")
	}
}

func TestLikelyHasHumanSpeech(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		result     ProbeResult
		want       bool
	}{
		{
			name:       "talking head with audio",
			sourceType: "talking_head",
			result:     ProbeResult{HasAudio: true, AudioChannels: 2},
			want:       true,
		},
		{
			name:       "visual only mono audio",
			sourceType: "visual_only",
			result:     ProbeResult{HasAudio: true, AudioChannels: 1},
			want:       true,
		},
		{
			name:       "visual only stereo audio",
			sourceType: "visual_only",
			result:     ProbeResult{HasAudio: true, AudioChannels: 2},
			want:       false,
		},
		{
			name:       "no audio",
			sourceType: "talking_head",
			result:     ProbeResult{HasAudio: false},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LikelyHasHumanSpeech(tt.sourceType, tt.result); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
