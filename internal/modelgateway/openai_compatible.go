package modelgateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OpenAICompatibleAnalyzer struct {
	baseURL   string
	apiKey    string
	model     string
	maxTokens int
	client    *http.Client
}

func NewOpenAICompatibleAnalyzer(cfg Config) *OpenAICompatibleAnalyzer {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAICompatibleAnalyzer{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     strings.TrimSpace(cfg.Model),
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: timeout},
	}
}

func (a *OpenAICompatibleAnalyzer) AnalyzeAsset(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
	if a.baseURL == "" {
		return AnalyzeAssetResult{}, NewError(ErrorCodeConfiguration, "openai compatible base_url is required", false, nil)
	}
	if a.model == "" {
		return AnalyzeAssetResult{}, NewError(ErrorCodeConfiguration, "vlm model is required", false, nil)
	}

	promptBundle := BuildPromptBundle(input)
	userContent := []map[string]any{
		{
			"type": "text",
			"text": promptBundle.Prompts[0].User,
		},
	}
	for _, frame := range input.FrameSnapshots {
		dataURL, err := imageDataURL(frame.StorageKey)
		if err != nil {
			return AnalyzeAssetResult{}, NewError(ErrorCodeConfiguration, fmt.Sprintf("read frame failed: %v", err), false, err)
		}
		userContent = append(userContent, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": dataURL,
			},
		})
	}
	if input.ProductReferenceImage != nil && input.ProductReferenceImage.StorageKey != "" {
		dataURL, err := imageDataURL(input.ProductReferenceImage.StorageKey)
		if err != nil {
			return AnalyzeAssetResult{}, NewError(ErrorCodeConfiguration, fmt.Sprintf("read product reference image failed: %v", err), false, err)
		}
		userContent = append(userContent, map[string]any{
			"type": "text",
			"text": "Product reference image follows. Use it only as identification reference, not as a video frame.",
		})
		userContent = append(userContent, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": dataURL,
			},
		})
	}

	payload := map[string]any{
		"model": a.model,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": promptBundle.Prompts[0].System,
			},
			{
				"role":    "user",
				"content": userContent,
			},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      a.maxTokens,
		"temperature":     0,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return AnalyzeAssetResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(a.baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return AnalyzeAssetResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return AnalyzeAssetResult{}, NewError(ErrorCodeProviderFailure, fmt.Sprintf("request vlm failed: %v", err), true, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return AnalyzeAssetResult{}, NewError(ErrorCodeProviderFailure, fmt.Sprintf("read vlm response failed: %v", err), true, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AnalyzeAssetResult{}, NewError(ErrorCodeProviderFailure, fmt.Sprintf("vlm endpoint returned status %d: %s", resp.StatusCode, truncateString(string(respBody), 500)), resp.StatusCode >= 500, nil)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return AnalyzeAssetResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode vlm response failed: %v", err), false, err)
	}
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return AnalyzeAssetResult{}, NewError(ErrorCodeInvalidResponse, "vlm response is empty", false, nil)
	}

	result, err := decodeAnalyzeAssetResult(chatResp.Choices[0].Message.Content)
	if err != nil {
		return AnalyzeAssetResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode vlm json output failed: %v", err), false, err)
	}
	if result.ModelResult == nil {
		result.ModelResult = map[string]any{}
	}
	result.ModelResult["provider"] = "openai_compatible"
	result.ModelResult["model"] = a.model
	result.ModelResult["max_tokens"] = a.maxTokens
	result.ModelResult["has_product_reference_image"] = input.ProductReferenceImage != nil && input.ProductReferenceImage.StorageKey != ""
	return result, nil
}

func decodeAnalyzeAssetResult(content string) (AnalyzeAssetResult, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return AnalyzeAssetResult{}, err
	}
	return AnalyzeAssetResult{
		SceneDescription:  stringFromRaw(raw["scene_description"]),
		ShotSize:          stringFromRaw(raw["shot_size"]),
		CameraMovement:    stringFromRaw(raw["camera_movement"]),
		VisualTags:        stringSliceFromRaw(raw["visual_tags"]),
		QualityTags:       stringSliceFromRaw(raw["quality_tags"]),
		VisibleProduct:    boolFromRaw(raw["visible_product"]),
		ProductPosition:   stringFromRaw(raw["product_position"]),
		SceneContext:      stringFromRaw(raw["scene_context"]),
		ActionDescription: stringFromRaw(raw["action_description"]),
		PeoplePresence:    boolFromRaw(raw["people_presence"]),
		FaceVisible:       boolFromRaw(raw["face_visible"]),
		LightingCondition: stringFromRaw(raw["lighting_condition"]),
		ModelResult:       mapFromRaw(raw["model_result"]),
	}, nil
}

func stringFromRaw(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringSliceFromRaw(value any) []string {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringFromRaw(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		parts := strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '\n'
		})
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		if text := stringFromRaw(value); text != "" {
			return []string{text}
		}
		return nil
	}
}

func boolFromRaw(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "y", "1", "visible", "present", "有", "是", "可见", "出现", "有人", "露脸":
			return true
		default:
			return false
		}
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func mapFromRaw(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func imageDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mimeType := "image/jpeg"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mimeType = "image/png"
	case ".webp":
		mimeType = "image/webp"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func joinOpenAICompatibleURL(base string, suffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	suffix = "/" + strings.TrimLeft(suffix, "/")
	if strings.HasSuffix(base, suffix) {
		return base
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(suffix, "/v1/") {
		return base + strings.TrimPrefix(suffix, "/v1")
	}
	return base + suffix
}

func truncateString(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
