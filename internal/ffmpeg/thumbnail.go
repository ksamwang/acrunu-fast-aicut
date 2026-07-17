package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func ExtractThumbnail(ctx context.Context, inputPath string, outputPath string, width int, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("thumbnail dimensions must be positive")
	}
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black",
		width, height, width, height,
	)
	var lastOutput []byte
	var lastErr error
	for _, timestamp := range []string{"0.200", "0"} {
		_ = os.Remove(outputPath)
		args := []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-ss", timestamp, "-i", inputPath,
			"-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn",
			"-vf", filter, "-q:v", "4", outputPath,
		}
		lastOutput, lastErr = exec.CommandContext(ctx, ffmpegPath(), args...).CombinedOutput()
		if lastErr == nil && fileExistsAndNotEmpty(outputPath) {
			return nil
		}
	}
	return fmt.Errorf("ffmpeg thumbnail extraction failed: %w: %s", lastErr, string(lastOutput))
}
