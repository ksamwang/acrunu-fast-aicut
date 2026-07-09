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
	DurationMs    int
	Width         int
	Height        int
	FPS           float64
	Codec         string
	HasAudio      bool
	AudioCodec    string
	AudioChannels int
	BitrateKbps   int
}

type probeOutput struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Channels     int    `json:"channels"`
		AvgFrameRate string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
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
		DurationMs:  secondsStringToMs(parsed.Format.Duration),
		BitrateKbps: bitsPerSecondStringToKbps(parsed.Format.BitRate),
	}

	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if result.Width != 0 || result.Height != 0 || result.Codec != "" {
				continue
			}
			result.Width = stream.Width
			result.Height = stream.Height
			result.Codec = stream.CodecName
			result.FPS = parseFrameRate(stream.AvgFrameRate)
		case "audio":
			if result.HasAudio {
				continue
			}
			result.HasAudio = true
			result.AudioCodec = stream.CodecName
			result.AudioChannels = stream.Channels
		}
	}

	return result, nil
}

func LikelyHasHumanSpeech(sourceType string, result ProbeResult) bool {
	if !result.HasAudio {
		return false
	}
	if sourceType == "talking_head" {
		return true
	}
	return result.AudioChannels == 1
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

func bitsPerSecondStringToKbps(value string) int {
	bitsPerSecond, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(bitsPerSecond / 1000)
}
