package localagent

import (
	"context"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

const localMediaProcessConcurrency = 4

type Processor interface {
	Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error
	InterpretFPS(ctx context.Context, sourcePath string, outputPath string, sourceFPS float64, playbackFPS float64, durationMs int) error
	ExtractAudio(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error
	Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error)
	ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error)
	ExtractThumbnail(ctx context.Context, inputPath string, outputPath string, width int, height int) error
}

type processor struct {
	limiter chan struct{}
}

func NewProcessor() Processor {
	return &processor{limiter: make(chan struct{}, localMediaProcessConcurrency)}
}

func (p *processor) Cut(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int, options ffmpeg.CutOptions) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()
	return ffmpeg.CutWithOptions(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs, options)
}

func (p *processor) InterpretFPS(ctx context.Context, sourcePath string, outputPath string, sourceFPS float64, playbackFPS float64, durationMs int) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()
	return ffmpeg.InterpretFPS(ctx, sourcePath, outputPath, sourceFPS, playbackFPS, durationMs)
}

func (p *processor) ExtractAudio(ctx context.Context, sourcePath string, outputPath string, sourceInMs int, sourceOutMs int) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()
	return ffmpeg.ExtractAudio(ctx, sourcePath, outputPath, sourceInMs, sourceOutMs)
}

func (p *processor) Probe(ctx context.Context, filePath string) (ffmpeg.ProbeResult, error) {
	if err := p.acquire(ctx); err != nil {
		return ffmpeg.ProbeResult{}, err
	}
	defer p.release()
	return ffmpeg.Probe(ctx, filePath)
}

func (p *processor) ExtractFrames(ctx context.Context, inputPath string, outputDir string, timestampsMs []int) ([]ffmpeg.ExtractedFrame, error) {
	if err := p.acquire(ctx); err != nil {
		return nil, err
	}
	defer p.release()
	return ffmpeg.ExtractFrames(ctx, inputPath, outputDir, timestampsMs)
}

func (p *processor) ExtractThumbnail(ctx context.Context, inputPath string, outputPath string, width int, height int) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()
	return ffmpeg.ExtractThumbnail(ctx, inputPath, outputPath, width, height)
}

func (p *processor) acquire(ctx context.Context) error {
	select {
	case p.limiter <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *processor) release() {
	<-p.limiter
}
