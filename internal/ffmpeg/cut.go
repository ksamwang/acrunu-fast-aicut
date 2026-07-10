package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
)

type CutOptions struct {
	InterpretFPSEnabled bool
	SourceFPS           float64
	PlaybackFPS         float64
}

func Cut(ctx context.Context, inputPath string, outputPath string, startMs int, endMs int) error {
	return CutWithOptions(ctx, inputPath, outputPath, startMs, endMs, CutOptions{})
}

func CutWithOptions(ctx context.Context, inputPath string, outputPath string, startMs int, endMs int, options CutOptions) error {
	if endMs <= startMs {
		return fmt.Errorf("invalid cut range: %d-%d", startMs, endMs)
	}
	if err := validateCutOptions(options); err != nil {
		return err
	}

	startSeconds := fmt.Sprintf("%.3f", float64(startMs)/1000)
	durationSeconds := fmt.Sprintf("%.3f", float64(endMs-startMs)/1000)

	args := []string{
		"-y",
		"-ss", startSeconds,
		"-i", inputPath,
		"-t", durationSeconds,
	}
	if options.InterpretFPSEnabled {
		args = append(args,
			"-an",
			"-sn",
			"-dn",
			"-vf", fmt.Sprintf("setpts=N/(%.8f*TB)", options.PlaybackFPS),
			"-fps_mode", "passthrough",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-crf", "18",
			"-movflags", "+faststart",
		)
	} else {
		args = append(args, "-c", "copy")
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, ffmpegPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg cut failed: %w: %s", err, string(output))
	}

	return nil
}

func InterpretFPS(ctx context.Context, inputPath string, outputPath string, sourceFPS float64, playbackFPS float64, durationMs int) error {
	return CutWithOptions(ctx, inputPath, outputPath, 0, durationMs, CutOptions{
		InterpretFPSEnabled: true,
		SourceFPS:           sourceFPS,
		PlaybackFPS:         playbackFPS,
	})
}

func validateCutOptions(options CutOptions) error {
	if !options.InterpretFPSEnabled {
		return nil
	}
	if options.SourceFPS <= 0 {
		return fmt.Errorf("source fps is required for interpret fps")
	}
	if options.PlaybackFPS < 25 {
		return fmt.Errorf("playback fps must be at least 25")
	}
	if options.PlaybackFPS >= options.SourceFPS {
		return fmt.Errorf("playback fps must be lower than source fps")
	}
	return nil
}
