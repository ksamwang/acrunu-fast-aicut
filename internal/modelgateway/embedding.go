package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type EmbedTextInput struct {
	Text string
}

type EmbedTextResult struct {
	Embedding   []float64      `json:"embedding"`
	ModelResult map[string]any `json:"model_result,omitempty"`
}

type TextEmbedder interface {
	EmbedText(ctx context.Context, input EmbedTextInput) (EmbedTextResult, error)
}

type embedderFunc func(ctx context.Context, input EmbedTextInput) (EmbedTextResult, error)

func (f embedderFunc) EmbedText(ctx context.Context, input EmbedTextInput) (EmbedTextResult, error) {
	return f(ctx, input)
}

func NewTextEmbedder(cfg Config) TextEmbedder {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = "mock"
	}

	var base TextEmbedder
	switch provider {
	case "mock":
		base = mockTextEmbedder{dimensions: firstPositive(cfg.Dimensions, 1024)}
	case "openai_compatible":
		base = NewOpenAICompatibleEmbedder(cfg)
	default:
		base = embedderFunc(func(context.Context, EmbedTextInput) (EmbedTextResult, error) {
			return EmbedTextResult{}, NewError(
				ErrorCodeUnsupportedProvider,
				fmt.Sprintf("provider %q is configured but not implemented", provider),
				false,
				nil,
			)
		})
	}

	if cfg.Timeout > 0 {
		next := base
		base = embedderFunc(func(ctx context.Context, input EmbedTextInput) (EmbedTextResult, error) {
			runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
			result, err := next.EmbedText(runCtx, input)
			if err != nil {
				return EmbedTextResult{}, NormalizeError(err)
			}
			return result, nil
		})
	}

	return base
}

type mockTextEmbedder struct {
	dimensions int
}

func (m mockTextEmbedder) EmbedText(_ context.Context, input EmbedTextInput) (EmbedTextResult, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return EmbedTextResult{}, NewError(ErrorCodeConfiguration, "embedding text is required", false, nil)
	}
	dimensions := firstPositive(m.dimensions, 1024)
	vector := make([]float64, dimensions)
	for index, char := range []byte(text) {
		vector[index%dimensions] += float64(char) / 255
	}
	return EmbedTextResult{
		Embedding: vector,
		ModelResult: map[string]any{
			"provider":   "mock",
			"dimensions": dimensions,
		},
	}, nil
}

type OpenAICompatibleEmbedder struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

func NewOpenAICompatibleEmbedder(cfg Config) *OpenAICompatibleEmbedder {
	return &OpenAICompatibleEmbedder{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      strings.TrimSpace(cfg.Model),
		dimensions: cfg.Dimensions,
		client:     &http.Client{Timeout: firstDuration(cfg.Timeout, 120*time.Second)},
	}
}

func (e *OpenAICompatibleEmbedder) EmbedText(ctx context.Context, input EmbedTextInput) (EmbedTextResult, error) {
	if e.baseURL == "" {
		return EmbedTextResult{}, NewError(ErrorCodeConfiguration, "embedding base_url is required", false, nil)
	}
	if e.model == "" {
		return EmbedTextResult{}, NewError(ErrorCodeConfiguration, "embedding model is required", false, nil)
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return EmbedTextResult{}, NewError(ErrorCodeConfiguration, "embedding text is required", false, nil)
	}

	payload := map[string]any{
		"model": e.model,
		"input": text,
	}
	if e.dimensions > 0 {
		payload["dimensions"] = e.dimensions
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return EmbedTextResult{}, NewError(ErrorCodeInvalidResponse, "failed to encode embedding request", false, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(e.baseURL, "/v1/embeddings"), bytes.NewReader(body))
	if err != nil {
		return EmbedTextResult{}, NewError(ErrorCodeProviderFailure, "failed to create embedding request", true, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return EmbedTextResult{}, NewError(ErrorCodeProviderFailure, "failed to request embedding", true, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return EmbedTextResult{}, NewError(ErrorCodeProviderFailure, "failed to read embedding response", true, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EmbedTextResult{}, NewError(ErrorCodeProviderFailure, fmt.Sprintf("embedding endpoint returned status %d", resp.StatusCode), true, nil)
	}
	if strings.HasPrefix(strings.TrimSpace(string(respBody)), "<") {
		return EmbedTextResult{}, NewError(ErrorCodeInvalidResponse, "embedding endpoint returned HTML", false, nil)
	}

	var decoded struct {
		Model string `json:"model"`
		Data  []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return EmbedTextResult{}, NewError(ErrorCodeInvalidResponse, "failed to decode embedding response", false, err)
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return EmbedTextResult{}, NewError(ErrorCodeInvalidResponse, "embedding response is empty", false, nil)
	}

	return EmbedTextResult{
		Embedding: decoded.Data[0].Embedding,
		ModelResult: map[string]any{
			"provider":   "openai_compatible",
			"model":      firstNonEmptyString(decoded.Model, e.model),
			"dimensions": len(decoded.Data[0].Embedding),
			"usage":      decoded.Usage,
		},
	}, nil
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
