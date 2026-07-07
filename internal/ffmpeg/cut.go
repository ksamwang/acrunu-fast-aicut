package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
)

func Cut(ctx context.Context, inputPath string, outputPath string, startMs int, endMs int) error {
	if endMs <= startMs {
		return fmt.Errorf("invalid cut range: %d-%d", startMs, endMs)
	}

	startSeconds := fmt.Sprintf("%.3f", float64(startMs)/1000)
	durationSeconds := fmt.Sprintf("%.3f", float64(endMs-startMs)/1000)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-ss", startSeconds,
		"-i", inputPath,
		"-t", durationSeconds,
		"-c", "copy",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg cut failed: %w: %s", err, string(output))
	}

	return nil
}
