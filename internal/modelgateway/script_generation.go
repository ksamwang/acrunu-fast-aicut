package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultScriptGenerationMaxTokens = 8192
	TTSVisualSourceType              = "visual_only"
)

var allowedScriptSourceTypes = map[string]struct{}{
	TTSVisualSourceType: {},
}

type ScriptGenerationSellingPoint struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsCustom    bool   `json:"is_custom,omitempty"`
}

type ScriptGenerationInput struct {
	ProductName        string                         `json:"product_name"`
	ProductDescription string                         `json:"product_description,omitempty"`
	ProductCategory    string                         `json:"product_category,omitempty"`
	SellingPoints      []ScriptGenerationSellingPoint `json:"selling_points"`
	VariantCount       int                            `json:"variant_count"`
}

type ScriptGenerationBeat struct {
	Label        string `json:"label"`
	SellingPoint string `json:"selling_point"`
	VisualGoal   string `json:"visual_goal"`
	SourceType   string `json:"source_type"`
}

type ScriptGenerationVariant struct {
	Hook          string                 `json:"hook"`
	ScriptText    string                 `json:"script_text"`
	EditingIntent string                 `json:"editing_intent"`
	Beats         []ScriptGenerationBeat `json:"beats"`
}

type ScriptGenerationResult struct {
	Variants []ScriptGenerationVariant `json:"variants"`
}

type ScriptGenerator interface {
	GenerateScripts(context.Context, ScriptGenerationInput) (ScriptGenerationResult, error)
}

type scriptGeneratorFunc func(context.Context, ScriptGenerationInput) (ScriptGenerationResult, error)

func (f scriptGeneratorFunc) GenerateScripts(ctx context.Context, input ScriptGenerationInput) (ScriptGenerationResult, error) {
	return f(ctx, input)
}

func NewScriptGenerator(cfg Config) ScriptGenerator {
	switch strings.TrimSpace(cfg.Provider) {
	case "openai_compatible":
		return NewOpenAICompatibleScriptGenerator(cfg)
	default:
		provider := strings.TrimSpace(cfg.Provider)
		if provider == "" {
			provider = "openai_compatible"
		}
		return scriptGeneratorFunc(func(context.Context, ScriptGenerationInput) (ScriptGenerationResult, error) {
			return ScriptGenerationResult{}, NewError(
				ErrorCodeUnsupportedProvider,
				fmt.Sprintf("llm provider %q is not implemented", provider),
				false,
				nil,
			)
		})
	}
}

type OpenAICompatibleScriptGenerator struct {
	baseURL   string
	apiKey    string
	model     string
	maxTokens int
	client    *http.Client
}

func NewOpenAICompatibleScriptGenerator(cfg Config) *OpenAICompatibleScriptGenerator {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultScriptGenerationMaxTokens
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAICompatibleScriptGenerator{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     strings.TrimSpace(cfg.Model),
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: timeout},
	}
}

func (g *OpenAICompatibleScriptGenerator) GenerateScripts(ctx context.Context, input ScriptGenerationInput) (ScriptGenerationResult, error) {
	if g == nil || g.baseURL == "" {
		return ScriptGenerationResult{}, NewError(ErrorCodeConfiguration, "openai compatible base_url is required", false, nil)
	}
	if g.model == "" {
		return ScriptGenerationResult{}, NewError(ErrorCodeConfiguration, "llm model is required", false, nil)
	}
	if input.VariantCount < 1 || input.VariantCount > 8 {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, "variant_count must be between 1 and 8", false, nil)
	}

	promptBundle := BuildScriptGenerationPrompt(input)
	payload := map[string]any{
		"model": g.model,
		"messages": []map[string]any{
			{"role": "system", "content": promptBundle.Prompts[0].System},
			{"role": "user", "content": promptBundle.Prompts[0].User},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      g.maxTokens,
		"temperature":     0.7,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(g.baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return ScriptGenerationResult{}, normalizeScriptGenerationRequestError(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return ScriptGenerationResult{}, normalizeScriptGenerationRequestError(ctx, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return ScriptGenerationResult{}, NewError(
			ErrorCodeProviderFailure,
			fmt.Sprintf("llm endpoint returned status %d: %s", resp.StatusCode, truncateString(string(responseBody), 500)),
			retryable,
			nil,
		)
	}

	var chatResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode llm response failed: %v", err), false, err)
	}
	if len(chatResponse.Choices) == 0 || strings.TrimSpace(chatResponse.Choices[0].Message.Content) == "" {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, "llm response is empty", false, nil)
	}
	result, err := decodeScriptGenerationResult(chatResponse.Choices[0].Message.Content)
	if err != nil {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode llm json output failed: %v", err), false, err)
	}
	if err := ValidateScriptGenerationResult(result, input.VariantCount); err != nil {
		return ScriptGenerationResult{}, err
	}
	return result, nil
}

func decodeScriptGenerationResult(content string) (ScriptGenerationResult, error) {
	var result ScriptGenerationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ScriptGenerationResult{}, err
	}
	return result, nil
}

func ValidateScriptGenerationResult(result ScriptGenerationResult, expectedVariantCount int) error {
	if expectedVariantCount < 1 || expectedVariantCount > 8 {
		return NewError(ErrorCodeInvalidResponse, "invalid expected variant count", false, nil)
	}
	if len(result.Variants) != expectedVariantCount {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("expected %d script variants, got %d", expectedVariantCount, len(result.Variants)), false, nil)
	}
	for index := range result.Variants {
		variant := &result.Variants[index]
		variant.Hook = strings.TrimSpace(variant.Hook)
		variant.ScriptText = strings.TrimSpace(variant.ScriptText)
		variant.EditingIntent = strings.TrimSpace(variant.EditingIntent)
		if variant.Hook == "" || variant.ScriptText == "" || variant.EditingIntent == "" {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("variant %d is incomplete", index+1), false, nil)
		}
		if len([]rune(variant.ScriptText)) > 1600 {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("variant %d script_text is too long", index+1), false, nil)
		}
		if len(variant.Beats) < 3 || len(variant.Beats) > 5 {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("variant %d must contain 3 to 5 beats", index+1), false, nil)
		}
		for beatIndex := range variant.Beats {
			beat := &variant.Beats[beatIndex]
			beat.Label = strings.TrimSpace(beat.Label)
			beat.SellingPoint = strings.TrimSpace(beat.SellingPoint)
			beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
			beat.SourceType = strings.TrimSpace(beat.SourceType)
			if beat.Label == "" || beat.SellingPoint == "" || beat.VisualGoal == "" {
				return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("variant %d beat %d is incomplete", index+1, beatIndex+1), false, nil)
			}
			if _, ok := allowedScriptSourceTypes[beat.SourceType]; !ok {
				return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("variant %d beat %d has invalid source_type", index+1, beatIndex+1), false, nil)
			}
		}
	}
	return nil
}

func normalizeScriptGenerationRequestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return NewError(ErrorCodeTimeout, "llm request timed out", true, err)
	}
	return NewError(ErrorCodeProviderFailure, fmt.Sprintf("request llm failed: %v", err), true, err)
}
