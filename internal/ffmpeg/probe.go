package ffmpeg

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ProbeResult struct {
	DurationMs int
	Width      int
	Height     int
	FPS        float64
	Codec      string
}

type probeOutput struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func Probe(ctx context.Context, filePath string) (ProbeResult, error) {
	cmd := exec.CommandContext(
		ctx,
		ffprobePath(),
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return ProbeResult{}, err
	}

	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return ProbeResult{}, err
	}

	result := ProbeResult{
		DurationMs: secondsStringToMs(parsed.Format.Duration),
	}

	for _, stream := range parsed.Streams {
		if stream.CodecType != "video" {
			continue
		}
		result.Width = stream.Width
		result.Height = stream.Height
		result.Codec = stream.CodecName
		result.FPS = parseFrameRate(stream.AvgFrameRate)
		break
	}

	return result, nil
}

func ffprobePath() string {
	path := os.Getenv("FFPROBE_PATH")
	if path == "" {
		return "ffprobe"
	}
	return path
}

func secondsStringToMs(value string) int {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(seconds * 1000)
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator == 0 {
		return 0
	}
	return numerator / denominator
}
