package ffmpeg

import (
	"context"
	"fmt"
	"os"
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
		actualTimestampMs, err := extractSingleFrame(ctx, inputPath, outputPath, timestampMs)
		if err != nil {
			return nil, err
		}
		results = append(results, ExtractedFrame{
			FrameIndex:  index,
			TimestampMs: actualTimestampMs,
			OutputPath:  outputPath,
		})
	}

	return results, nil
}

func extractSingleFrame(ctx context.Context, inputPath string, outputPath string, timestampMs int) (int, error) {
	if timestampMs < 0 {
		timestampMs = 0
	}

	var lastOutput []byte
	var lastErr error = fmt.Errorf("no extract attempts were run")
	for _, candidateTimestampMs := range frameCandidateTimestamps(timestampMs) {
		attempts := [][]string{
			extractFrameArgs(inputPath, outputPath, candidateTimestampMs, true, false),
			extractFrameArgs(inputPath, outputPath, candidateTimestampMs, false, false),
			extractFrameArgs(inputPath, outputPath, candidateTimestampMs, true, true),
		}
		for _, args := range attempts {
			_ = os.Remove(outputPath)
			cmd := newCommandContext(ctx, ffmpegPath(), args...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				if fileExistsAndNotEmpty(outputPath) {
					return candidateTimestampMs, nil
				}
				err = fmt.Errorf("ffmpeg exited successfully but did not write output frame")
			}
			lastOutput = output
			lastErr = err
		}
	}

	return 0, fmt.Errorf("ffmpeg extract frame failed: %w: %s", lastErr, string(lastOutput))
}

func frameCandidateTimestamps(timestampMs int) []int {
	offsets := []int{0, -10, -20, -50, -100, -200, -500}
	candidates := make([]int, 0, len(offsets))
	seen := map[int]struct{}{}
	for _, offset := range offsets {
		candidate := timestampMs + offset
		if candidate < 0 {
			candidate = 0
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func fileExistsAndNotEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func extractFrameArgs(inputPath string, outputPath string, timestampMs int, precise bool, ignoreEditList bool) []string {
	commonOutputArgs := []string{
		"-map", "0:v:0",
		"-frames:v", "1",
		"-an",
		"-sn",
		"-dn",
		"-q:v", "2",
		"-pix_fmt", "yuvj420p",
		outputPath,
	}

	args := []string{"-y"}
	if precise {
		preSeekMs := timestampMs - 1000
		if preSeekMs < 0 {
			preSeekMs = 0
		}
		offsetMs := timestampMs - preSeekMs
		args = append(args, "-ss", formatSeekSeconds(preSeekMs))
		if ignoreEditList {
			args = append(args, "-ignore_editlist", "1")
		}
		args = append(args, "-i", inputPath, "-ss", formatSeekSeconds(offsetMs))
		args = append(args, commonOutputArgs...)
		return args
	}

	args = append(args, "-ss", formatSeekSeconds(timestampMs))
	if ignoreEditList {
		args = append(args, "-ignore_editlist", "1")
	}
	args = append(args, "-i", inputPath)
	args = append(args, commonOutputArgs...)
	return args
}

func formatSeekSeconds(timestampMs int) string {
	return fmt.Sprintf("%.3f", float64(timestampMs)/1000)
}
