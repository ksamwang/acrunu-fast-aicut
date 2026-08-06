package modelgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxFunASRResponseBytes = 4 << 20

type FunASRErrorKind string

const (
	FunASRErrorUnavailable FunASRErrorKind = "unavailable"
	FunASRErrorTimeout     FunASRErrorKind = "timeout"
	FunASRErrorBadResponse FunASRErrorKind = "bad_response"
)

type FunASRError struct {
	Kind       FunASRErrorKind
	StatusCode int
	Cause      error
}

func (e *FunASRError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("funasr %s (status %d)", e.Kind, e.StatusCode)
	}
	if e.Cause != nil {
		return fmt.Sprintf("funasr %s: %v", e.Kind, e.Cause)
	}
	return "funasr " + string(e.Kind)
}

func (e *FunASRError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type FunASRTranscriptionInput struct {
	Filename   string
	Audio      io.Reader
	DurationMs int
}

type ASRTranscriptSegment struct {
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
}

type ASRTranscriptToken struct {
	Text    string `json:"text"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

type ASRTranscriptionResult struct {
	Text     string                 `json:"text"`
	Segments []ASRTranscriptSegment `json:"segments"`
	Tokens   []ASRTranscriptToken   `json:"tokens"`
}

type FunASRClient struct {
	baseURL string
	client  *http.Client
}

type funASRResponse struct {
	Text         string           `json:"text"`
	Timestamp    [][]int          `json:"timestamp"`
	SentenceInfo []funASRSentence `json:"sentence_info"`
}

type funASRSentence struct {
	Text      string  `json:"text"`
	Start     int     `json:"start"`
	End       int     `json:"end"`
	Timestamp [][]int `json:"timestamp"`
}

func NewFunASRClient(baseURL string, timeout time.Duration) *FunASRClient {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &FunASRClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *FunASRClient) Transcribe(ctx context.Context, input FunASRTranscriptionInput) (ASRTranscriptionResult, error) {
	if c.baseURL == "" {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorUnavailable, Cause: errors.New("base URL is required")}
	}
	if input.Audio == nil {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorBadResponse, Cause: errors.New("audio is required")}
	}
	if input.DurationMs <= 0 {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorBadResponse, Cause: errors.New("duration must be positive")}
	}

	pipeReader, pipeWriter := io.Pipe()
	formWriter := multipart.NewWriter(pipeWriter)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/transcriptions", pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorUnavailable, Cause: err}
	}
	req.Header.Set("Content-Type", formWriter.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	writeDone := make(chan error, 1)
	go func() {
		part, writeErr := formWriter.CreateFormFile("file", safeMultipartFilename(input.Filename))
		if writeErr == nil {
			_, writeErr = io.Copy(part, input.Audio)
		}
		if closeErr := formWriter.Close(); writeErr == nil {
			writeErr = closeErr
		}
		_ = pipeWriter.CloseWithError(writeErr)
		writeDone <- writeErr
	}()

	resp, requestErr := c.client.Do(req)
	if requestErr != nil {
		_ = pipeReader.CloseWithError(requestErr)
		<-writeDone
		return ASRTranscriptionResult{}, classifyFunASRRequestError(ctx, requestErr)
	}
	defer resp.Body.Close()

	writeErr := <-writeDone
	if writeErr != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorUnavailable, Cause: writeErr}
	}

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxFunASRResponseBytes+1))
	if readErr != nil {
		return ASRTranscriptionResult{}, classifyFunASRRequestError(ctx, readErr)
	}
	if len(responseBody) > maxFunASRResponseBytes {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorBadResponse, StatusCode: resp.StatusCode, Cause: errors.New("response is too large")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ASRTranscriptionResult{}, classifyFunASRStatus(resp.StatusCode)
	}

	var decoded funASRResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return ASRTranscriptionResult{}, &FunASRError{Kind: FunASRErrorBadResponse, StatusCode: resp.StatusCode, Cause: err}
	}
	return normalizeFunASRResponse(decoded, input.DurationMs), nil
}

func normalizeFunASRResponse(input funASRResponse, durationMs int) ASRTranscriptionResult {
	segments := make([]ASRTranscriptSegment, 0, len(input.SentenceInfo))
	tokens := make([]ASRTranscriptToken, 0, len(input.Timestamp))
	for _, sentence := range input.SentenceInfo {
		text := strings.TrimSpace(sentence.Text)
		if text == "" {
			continue
		}
		startMs, endMs := sentence.Start, sentence.End
		if endMs <= startMs {
			if timestampStart, timestampEnd, ok := timestampBounds(sentence.Timestamp); ok {
				startMs, endMs = timestampStart, timestampEnd
			}
		}
		startMs, endMs, ok := normalizeSegmentRange(startMs, endMs, durationMs)
		if !ok {
			continue
		}
		segments = append(segments, ASRTranscriptSegment{StartMs: startMs, EndMs: endMs, Text: text})
		tokens = append(tokens, transcriptTokens(text, sentence.Timestamp, durationMs)...)
	}
	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].StartMs < segments[j].StartMs
	})

	text := strings.TrimSpace(input.Text)
	if text == "" && len(segments) > 0 {
		parts := make([]string, 0, len(segments))
		for _, segment := range segments {
			parts = append(parts, segment.Text)
		}
		text = strings.Join(parts, "")
	}
	if text != "" && len(segments) == 0 {
		startMs, endMs := 0, durationMs
		if timestampStart, timestampEnd, ok := timestampBounds(input.Timestamp); ok {
			startMs, endMs = timestampStart, timestampEnd
		}
		if startMs, endMs, ok := normalizeSegmentRange(startMs, endMs, durationMs); ok {
			segments = append(segments, ASRTranscriptSegment{StartMs: startMs, EndMs: endMs, Text: text})
		}
	}
	if topLevelTokens := transcriptTokens(text, input.Timestamp, durationMs); len(topLevelTokens) > len(tokens) {
		tokens = topLevelTokens
	}
	sort.SliceStable(tokens, func(i, j int) bool {
		return tokens[i].StartMs < tokens[j].StartMs
	})

	return ASRTranscriptionResult{Text: text, Segments: segments, Tokens: tokens}
}

func transcriptTokens(text string, timestamps [][]int, durationMs int) []ASRTranscriptToken {
	runes := make([]rune, 0, len([]rune(text)))
	for _, value := range []rune(text) {
		if strings.TrimSpace(string(value)) == "" {
			continue
		}
		runes = append(runes, value)
	}
	if len(runes) == 0 || len(timestamps) == 0 {
		return nil
	}

	// FunASR omits punctuation timestamps in some models. Prefer an exact
	// rune mapping, then fall back to non-punctuation runes.
	if len(runes) != len(timestamps) {
		content := make([]rune, 0, len(runes))
		for _, value := range runes {
			if !strings.ContainsRune("，。！？、；：,.!?;:\"'“”‘’（）()《》<>—…·-", value) {
				content = append(content, value)
			}
		}
		if len(content) != len(timestamps) {
			return interpolateTranscriptTokens(content, timestamps, durationMs)
		}
		runes = content
	}

	result := make([]ASRTranscriptToken, 0, len(runes))
	for index, value := range runes {
		if len(timestamps[index]) < 2 {
			continue
		}
		startMs, endMs, ok := normalizeSegmentRange(timestamps[index][0], timestamps[index][1], durationMs)
		if !ok {
			continue
		}
		result = append(result, ASRTranscriptToken{Text: string(value), StartMs: startMs, EndMs: endMs})
	}
	return result
}

func interpolateTranscriptTokens(runes []rune, timestamps [][]int, durationMs int) []ASRTranscriptToken {
	if len(runes) == 0 || len(timestamps) == 0 {
		return nil
	}
	result := make([]ASRTranscriptToken, 0, len(runes))
	for runeIndex, value := range runes {
		timestampIndex := runeIndex * len(timestamps) / len(runes)
		if timestampIndex >= len(timestamps) || len(timestamps[timestampIndex]) < 2 {
			continue
		}
		groupStart := (timestampIndex*len(runes) + len(timestamps) - 1) / len(timestamps)
		groupEnd := ((timestampIndex+1)*len(runes) + len(timestamps) - 1) / len(timestamps)
		if groupEnd <= groupStart {
			groupEnd = groupStart + 1
		}
		rawStart, rawEnd := timestamps[timestampIndex][0], timestamps[timestampIndex][1]
		startMs := rawStart + (rawEnd-rawStart)*(runeIndex-groupStart)/(groupEnd-groupStart)
		endMs := rawStart + (rawEnd-rawStart)*(runeIndex-groupStart+1)/(groupEnd-groupStart)
		startMs, endMs, ok := normalizeSegmentRange(startMs, endMs, durationMs)
		if !ok {
			continue
		}
		result = append(result, ASRTranscriptToken{Text: string(value), StartMs: startMs, EndMs: endMs})
	}
	return result
}

func timestampBounds(timestamps [][]int) (int, int, bool) {
	startMs, endMs := 0, 0
	found := false
	for _, timestamp := range timestamps {
		if len(timestamp) < 2 || timestamp[1] <= timestamp[0] {
			continue
		}
		if !found || timestamp[0] < startMs {
			startMs = timestamp[0]
		}
		if !found || timestamp[1] > endMs {
			endMs = timestamp[1]
		}
		found = true
	}
	return startMs, endMs, found
}

func normalizeSegmentRange(startMs int, endMs int, durationMs int) (int, int, bool) {
	if startMs < 0 {
		startMs = 0
	}
	if durationMs <= 0 || startMs >= durationMs {
		return 0, 0, false
	}
	if endMs > durationMs {
		endMs = durationMs
	}
	if endMs <= startMs {
		return 0, 0, false
	}
	return startMs, endMs, true
}

func safeMultipartFilename(filename string) string {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	if slash := strings.LastIndex(filename, "/"); slash >= 0 {
		filename = filename[slash+1:]
	}
	if filename == "" || filename == "." || filename == ".." {
		return "audio.wav"
	}
	return filename
}

func classifyFunASRRequestError(ctx context.Context, err error) *FunASRError {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &FunASRError{Kind: FunASRErrorTimeout, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &FunASRError{Kind: FunASRErrorTimeout, Cause: err}
	}
	return &FunASRError{Kind: FunASRErrorUnavailable, Cause: err}
}

func classifyFunASRStatus(statusCode int) *FunASRError {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return &FunASRError{Kind: FunASRErrorTimeout, StatusCode: statusCode}
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return &FunASRError{Kind: FunASRErrorUnavailable, StatusCode: statusCode}
	default:
		return &FunASRError{Kind: FunASRErrorBadResponse, StatusCode: statusCode}
	}
}
