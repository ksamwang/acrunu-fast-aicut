package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxCosyVoiceResponseBytes = 128 << 20
	maxCosyVoiceUnits         = 512
	maxCosyVoicePauseMs       = 2000
)

type CosyVoiceErrorKind string

const (
	CosyVoiceErrorUnavailable CosyVoiceErrorKind = "unavailable"
	CosyVoiceErrorTimeout     CosyVoiceErrorKind = "timeout"
	CosyVoiceErrorBadResponse CosyVoiceErrorKind = "bad_response"
)

type CosyVoiceError struct {
	Kind       CosyVoiceErrorKind
	StatusCode int
	Cause      error
}

func (e *CosyVoiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("cosyvoice %s (status %d)", e.Kind, e.StatusCode)
	}
	if e.Cause != nil {
		return fmt.Sprintf("cosyvoice %s: %v", e.Kind, e.Cause)
	}
	return "cosyvoice " + string(e.Kind)
}

func (e *CosyVoiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type CosyVoiceSynthesisInput struct {
	Text                string
	PromptAudio         io.Reader
	PromptAudioFilename string
	PromptText          string
	Units               []CosyVoiceSynthesisUnit
}

type CosyVoiceSynthesisUnit struct {
	Text         string `json:"text"`
	PauseAfterMs int    `json:"pause_after_ms"`
}

type CosyVoiceSynthesisUnitResult struct {
	SpeechSamples int
	TotalSamples  int
}

type CosyVoiceSynthesisResult struct {
	Audio      []byte
	Model      string
	SampleRate int
	Units      []CosyVoiceSynthesisUnitResult
}

type CosyVoiceClient struct {
	baseURL string
	client  *http.Client
}

func NewCosyVoiceClient(baseURL string, timeout time.Duration) *CosyVoiceClient {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &CosyVoiceClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *CosyVoiceClient) Synthesize(ctx context.Context, input CosyVoiceSynthesisInput) (CosyVoiceSynthesisResult, error) {
	if c == nil || c.baseURL == "" {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: errors.New("base URL is required")}
	}
	if strings.TrimSpace(input.Text) == "" {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, Cause: errors.New("text is required")}
	}
	if input.PromptAudio == nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, Cause: errors.New("prompt audio is required")}
	}
	if strings.TrimSpace(input.PromptText) == "" {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, Cause: errors.New("prompt text is required")}
	}
	if err := validateCosyVoiceUnits(input.Text, input.Units); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, Cause: err}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("text", strings.TrimSpace(input.Text)); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	if err := writer.WriteField("mode", "zero_shot"); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	if err := writer.WriteField("prompt_text", strings.TrimSpace(input.PromptText)); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	if len(input.Units) > 0 {
		encodedUnits, err := json.Marshal(input.Units)
		if err != nil {
			return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, Cause: err}
		}
		if err := writer.WriteField("segments_json", string(encodedUnits)); err != nil {
			return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
		}
	}
	promptPart, err := writer.CreateFormFile("prompt_audio", cosyVoiceMultipartFilename(input.PromptAudioFilename))
	if err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	if _, err := io.Copy(promptPart, input.PromptAudio); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	if err := writer.Close(); err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/synthesize", &body)
	if err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "audio/wav")

	response, err := c.client.Do(request)
	if err != nil {
		return CosyVoiceSynthesisResult{}, classifyCosyVoiceRequestError(ctx, err)
	}
	defer response.Body.Close()

	audio, err := io.ReadAll(io.LimitReader(response.Body, maxCosyVoiceResponseBytes+1))
	if err != nil {
		return CosyVoiceSynthesisResult{}, classifyCosyVoiceRequestError(ctx, err)
	}
	if len(audio) > maxCosyVoiceResponseBytes {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, StatusCode: response.StatusCode, Cause: errors.New("response is too large")}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CosyVoiceSynthesisResult{}, classifyCosyVoiceStatus(response.StatusCode)
	}
	if !isWAVContentType(response.Header.Get("Content-Type")) {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, StatusCode: response.StatusCode, Cause: errors.New("expected audio/wav response")}
	}
	if len(audio) == 0 {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, StatusCode: response.StatusCode, Cause: errors.New("empty audio response")}
	}

	sampleRate := parseCosyVoiceSampleRate(response.Header.Get("X-AICUT-TTS-Sample-Rate"))
	unitResults, err := parseCosyVoiceUnitResults(response.Header, len(input.Units), sampleRate)
	if err != nil {
		return CosyVoiceSynthesisResult{}, &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, StatusCode: response.StatusCode, Cause: err}
	}
	return CosyVoiceSynthesisResult{
		Audio:      audio,
		Model:      strings.TrimSpace(response.Header.Get("X-AICUT-TTS-Model")),
		SampleRate: sampleRate,
		Units:      unitResults,
	}, nil
}

func validateCosyVoiceUnits(text string, units []CosyVoiceSynthesisUnit) error {
	if len(units) == 0 {
		return nil
	}
	if len(units) > maxCosyVoiceUnits {
		return fmt.Errorf("synthesis units must not exceed %d", maxCosyVoiceUnits)
	}
	var combined strings.Builder
	for index, unit := range units {
		unitText := strings.TrimSpace(unit.Text)
		if unitText == "" {
			return fmt.Errorf("synthesis unit %d text is required", index+1)
		}
		if unit.PauseAfterMs < 0 || unit.PauseAfterMs > maxCosyVoicePauseMs {
			return fmt.Errorf("synthesis unit %d pause is invalid", index+1)
		}
		combined.WriteString(unitText)
	}
	if units[len(units)-1].PauseAfterMs != 0 {
		return errors.New("final synthesis unit must not append a pause")
	}
	if combined.String() != strings.TrimSpace(text) {
		return errors.New("synthesis unit text does not match full text")
	}
	return nil
}

func parseCosyVoiceUnitResults(header http.Header, unitCount int, sampleRate int) ([]CosyVoiceSynthesisUnitResult, error) {
	if unitCount == 0 {
		return nil, nil
	}
	if strings.TrimSpace(header.Get("X-AICUT-TTS-Timing-Version")) != "1" {
		return nil, errors.New("segmented synthesis timing metadata is missing")
	}
	if sampleRate <= 0 {
		return nil, errors.New("segmented synthesis sample rate is missing")
	}
	speechSamples, err := parseCosyVoiceSampleList(header.Get("X-AICUT-TTS-Speech-Samples"), unitCount)
	if err != nil {
		return nil, fmt.Errorf("invalid segmented speech samples: %w", err)
	}
	totalSamples, err := parseCosyVoiceSampleList(header.Get("X-AICUT-TTS-Unit-Samples"), unitCount)
	if err != nil {
		return nil, fmt.Errorf("invalid segmented unit samples: %w", err)
	}
	results := make([]CosyVoiceSynthesisUnitResult, unitCount)
	for index := range results {
		if speechSamples[index] > totalSamples[index] {
			return nil, fmt.Errorf("unit %d speech exceeds its total samples", index+1)
		}
		results[index] = CosyVoiceSynthesisUnitResult{SpeechSamples: speechSamples[index], TotalSamples: totalSamples[index]}
	}
	return results, nil
}

func parseCosyVoiceSampleList(value string, count int) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) != count {
		return nil, fmt.Errorf("expected %d values, got %d", count, len(parts))
	}
	result := make([]int, count)
	for index, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("value %d is invalid", index+1)
		}
		result[index] = parsed
	}
	return result, nil
}

func cosyVoiceMultipartFilename(value string) string {
	filename := filepath.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if filename == "" || filename == "." || filename == ".." {
		return "reference.wav"
	}
	return filename
}

func isWAVContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return mediaType == "audio/wav" || mediaType == "audio/x-wav" || mediaType == "audio/wave"
}

func parseCosyVoiceSampleRate(value string) int {
	sampleRate, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || sampleRate <= 0 {
		return 0
	}
	return sampleRate
}

func classifyCosyVoiceRequestError(ctx context.Context, err error) *CosyVoiceError {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &CosyVoiceError{Kind: CosyVoiceErrorTimeout, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &CosyVoiceError{Kind: CosyVoiceErrorTimeout, Cause: err}
	}
	return &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, Cause: err}
}

func classifyCosyVoiceStatus(statusCode int) *CosyVoiceError {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return &CosyVoiceError{Kind: CosyVoiceErrorTimeout, StatusCode: statusCode}
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return &CosyVoiceError{Kind: CosyVoiceErrorUnavailable, StatusCode: statusCode}
	default:
		return &CosyVoiceError{Kind: CosyVoiceErrorBadResponse, StatusCode: statusCode}
	}
}
