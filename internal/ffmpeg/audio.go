package ffmpeg

import (
	"context"
	"fmt"
	"os"
)

func ExtractAudio(ctx context.Context, inputPath string, outputPath string, startMs int, endMs int) error {
	args, err := extractAudioArgs(inputPath, outputPath, startMs, endMs)
	if err != nil {
		return err
	}
	cmd := newCommandContext(ctx, ffmpegPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extract audio failed: %w: %s", err, string(output))
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("ffmpeg extract audio did not create output: %w", err)
	}
	if info.Size() <= 44 {
		return fmt.Errorf("ffmpeg extract audio wrote an empty wav file")
	}
	return nil
}

func extractAudioArgs(inputPath string, outputPath string, startMs int, endMs int) ([]string, error) {
	if inputPath == "" {
		return nil, fmt.Errorf("audio source path is required")
	}
	if outputPath == "" {
		return nil, fmt.Errorf("audio output path is required")
	}
	if startMs < 0 || endMs <= startMs {
		return nil, fmt.Errorf("invalid audio range: %d-%d", startMs, endMs)
	}
	startSeconds := fmt.Sprintf("%.3f", float64(startMs)/1000)
	durationSeconds := fmt.Sprintf("%.3f", float64(endMs-startMs)/1000)
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-ss", startSeconds,
		"-t", durationSeconds,
		"-i", inputPath,
		"-map", "0:a:0",
		"-vn",
		"-sn",
		"-dn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		outputPath,
	}, nil
}
