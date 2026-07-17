package localagent

import (
	"context"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

type Processor interface {
	Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error
	InterpretFPS(ctx context.Context, sourcePath string, outputPath string, sourceFPS float64, playbackFPS float64, durationMs int) error
	ExtractAudio(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error
	Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error)
	ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error)
	ExtractThumbnail(ctx context.Context, inputPath string, outputPath string, width int, height int) error
}

type processor struct{}

func NewProcessor() Processor {
	return processor{}
}

func (p processor) Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error {
	return ffmpeg.CutWithOptions(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs, options)
}

func (p processor) InterpretFPS(ctx context.Context, sourcePath string, outputPath string, sourceFPS float64, playbackFPS float64, durationMs int) error {
	return ffmpeg.InterpretFPS(ctx, sourcePath, outputPath, sourceFPS, playbackFPS, durationMs)
}

func (p processor) ExtractAudio(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error {
	return ffmpeg.ExtractAudio(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs)
}

func (p processor) Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error) {
	return ffmpeg.Probe(ctx, filePath)
}

func (p processor) ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error) {
	return ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestampsMs)
}

func (p processor) ExtractThumbnail(ctx context.Context, inputPath string, outputPath string, width int, height int) error {
	return ffmpeg.ExtractThumbnail(ctx, inputPath, outputPath, width, height)
}
