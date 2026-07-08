package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ExtractedFrame struct {
	FrameIndex  int
	TimestampMs int
	OutputPath  string
}

func ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ExtractedFrame, error) {
	if len(timestampsMs) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	results := make([]ExtractedFrame, 0, len(timestampsMs))
	for index, timestampMs := range timestampsMs {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("frame_%03d.jpg", index))
		if err := extractSingleFrame(ctx, inputPath, outputPath, timestampMs); err != nil {
			return nil, err
		}
		results = append(results, ExtractedFrame{
			FrameIndex:  index,
			TimestampMs: timestampMs,
			OutputPath:  outputPath,
		})
	}

	return results, nil
}

func extractSingleFrame(ctx context.Context, inputPath string, outputPath string, timestampMs int) error {
	if timestampMs < 0 {
		timestampMs = 0
	}
	seekSeconds := fmt.Sprintf("%.3f", float64(timestampMs)/1000)

	cmd := exec.CommandContext(
		ctx,
		ffmpegPath(),
		"-y",
		"-ss", seekSeconds,
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "2",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extract frame failed: %w: %s", err, string(output))
	}
	return nil
}
