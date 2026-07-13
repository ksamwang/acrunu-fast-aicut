package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractAudioArgsUseSelectionAndStandardWAVFormat(t *testing.T) {
	input := `E:\素材 目录\口播视频.mp4`
	output := `E:\临时 目录\选区.wav`
	args, err := extractAudioArgs(input, output, 1234, 5678)
	if err != nil {
		t.Fatalf("extractAudioArgs: %v", err)
	}
	want := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-ss", "1.234",
		"-t", "4.444",
		"-i", input,
		"-map", "0:a:0",
		"-vn",
		"-sn",
		"-dn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		output,
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected ffmpeg args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestExtractAudioArgsRejectInvalidRange(t *testing.T) {
	if _, err := extractAudioArgs("source.mp4", "audio.wav", 1000, 1000); err == nil {
		t.Fatal("expected invalid range error")
	}
	if _, err := extractAudioArgs("source.mp4", "audio.wav", -1, 1000); err == nil {
		t.Fatal("expected negative start error")
	}
}

func TestExtractAudioFromConfiguredRealSample(t *testing.T) {
	sourcePath := os.Getenv("AICUT_ASR_TEST_VIDEO")
	if sourcePath == "" {
		t.Skip("AICUT_ASR_TEST_VIDEO is not configured")
	}
	outputPath := filepath.Join(t.TempDir(), "selection.wav")
	if err := ExtractAudio(context.Background(), sourcePath, outputPath, 0, 19017); err != nil {
		t.Fatalf("ExtractAudio: %v", err)
	}
	probe, err := Probe(context.Background(), outputPath)
	if err != nil {
		t.Fatalf("Probe extracted audio: %v", err)
	}
	if !probe.HasAudio || probe.AudioCodec != "pcm_s16le" || probe.AudioChannels != 1 || probe.AudioSampleRate != 16000 {
		t.Fatalf("unexpected extracted audio probe %#v", probe)
	}
	if delta := absInt(probe.DurationMs - 19017); delta > 50 {
		t.Fatalf("expected duration close to 19017ms, got %dms", probe.DurationMs)
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
