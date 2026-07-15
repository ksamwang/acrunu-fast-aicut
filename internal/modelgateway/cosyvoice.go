package modelgateway

import (
	"bytes"
	"context"
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

const maxCosyVoiceResponseBytes = 128 << 20

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
}

type CosyVoiceSynthesisResult struct {
	Audio      []byte
	Model      string
	SampleRate int
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

	return CosyVoiceSynthesisResult{
		Audio:      audio,
		Model:      strings.TrimSpace(response.Header.Get("X-AICUT-TTS-Model")),
		SampleRate: parseCosyVoiceSampleRate(response.Header.Get("X-AICUT-TTS-Sample-Rate")),
	}, nil
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
