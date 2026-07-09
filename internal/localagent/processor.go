package localagent

import (
	"context"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type Processor interface {
	Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error
	Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error)
	ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error)
	Analyze(ctx context.Context, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error)
}

type processor struct {
	analyzer modelgateway.AssetAnalyzer
}

func NewProcessor(analyzer modelgateway.AssetAnalyzer) Processor {
	if analyzer == nil {
		analyzer = modelgateway.NewMockAssetAnalyzer()
	}
	return processor{analyzer: analyzer}
}

func (p processor) Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error {
	return ffmpeg.Cut(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs)
}

func (p processor) Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error) {
	return ffmpeg.Probe(ctx, filePath)
}

func (p processor) ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error) {
	return ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestampsMs)
}

func (p processor) Analyze(ctx context.Context, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return p.analyzer.AnalyzeAsset(runCtx, input)
}
