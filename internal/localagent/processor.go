package localagent

import (
	"context"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

type Processor interface {
	Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error
	Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error)
	ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error)
}

type processor struct{}

func NewProcessor() Processor {
	return processor{}
}

func (p processor) Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error {
	return ffmpeg.CutWithOptions(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs, options)
}

func (p processor) Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error) {
	return ffmpeg.Probe(ctx, filePath)
}

func (p processor) ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error) {
	return ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestampsMs)
}
