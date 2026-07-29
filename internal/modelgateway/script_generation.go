package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	defaultScriptGenerationMaxTokens = 8192
	TTSVisualSourceType              = "visual_only"
	DefaultScriptTargetDuration      = 30
	scriptSpokenCharactersPerSecond  = 5.0
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
	ProductName             string                         `json:"product_name"`
	ProductDescription      string                         `json:"product_description,omitempty"`
	ProductCategory         string                         `json:"product_category,omitempty"`
	SellingPoints           []ScriptGenerationSellingPoint `json:"selling_points"`
	AvailableVisualEvidence []string                       `json:"available_visual_evidence,omitempty"`
	VariantCount            int                            `json:"variant_count"`
	TargetDurationSeconds   int                            `json:"target_duration_seconds"`
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
	targetDuration, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, "target_duration_seconds is unsupported", false, nil)
	}
	input.TargetDurationSeconds = targetDuration

	promptBundle := BuildScriptGenerationPrompt(input)
	result, err := g.requestScripts(ctx, promptBundle, "", 0.75)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	if err := ValidateScriptGenerationResult(result, input); err == nil {
		return result, nil
	} else {
		previousJSON, _ := json.Marshal(result)
		repairInstruction := "The previous JSON failed server validation. Rewrite the complete JSON once and fix every stated issue while preserving the requested variant count, target duration, factual limits, and output schema. Validation error: " + err.Error() + ". Previous JSON: " + string(previousJSON)
		repaired, repairErr := g.requestScripts(ctx, promptBundle, repairInstruction, 0.2)
		if repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		if repairErr := ValidateScriptGenerationResult(repaired, input); repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		return repaired, nil
	}
}

func (g *OpenAICompatibleScriptGenerator) requestScripts(ctx context.Context, promptBundle PromptBundle, repairInstruction string, temperature float64) (ScriptGenerationResult, error) {
	userPrompt := promptBundle.Prompts[0].User
	if strings.TrimSpace(repairInstruction) != "" {
		userPrompt += "\n\n" + repairInstruction
	}
	payload := map[string]any{
		"model": g.model,
		"messages": []map[string]any{
			{"role": "system", "content": promptBundle.Prompts[0].System},
			{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      g.maxTokens,
		"temperature":     temperature,
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
	return result, nil
}

func decodeScriptGenerationResult(content string) (ScriptGenerationResult, error) {
	var result ScriptGenerationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ScriptGenerationResult{}, err
	}
	return result, nil
}

func ValidateScriptGenerationResult(result ScriptGenerationResult, input ScriptGenerationInput) error {
	issues := validateScriptGenerationResultIssues(result, input)
	if len(issues) == 0 {
		return nil
	}
	return NewError(ErrorCodeInvalidResponse, "invalid script generation: "+strings.Join(issues, "; "), false, nil)
}

func validateScriptGenerationResultIssues(result ScriptGenerationResult, input ScriptGenerationInput) []string {
	issues := make([]string, 0)
	if input.VariantCount < 1 || input.VariantCount > 8 {
		return append(issues, "invalid expected variant count")
	}
	targetDuration, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Variants) != input.VariantCount {
		issues = append(issues, fmt.Sprintf("expected %d script variants, got %d", input.VariantCount, len(result.Variants)))
	}
	minimumCharacters, maximumCharacters := ScriptSpokenCharacterRange(targetDuration)
	minimumBeats, maximumBeats := ScriptBeatCountRange(targetDuration)
	seenHooks := map[string]struct{}{}
	allowedSellingPoints := make(map[string]struct{}, len(input.SellingPoints))
	coveredSellingPoints := make(map[string]struct{}, len(input.SellingPoints))
	for _, sellingPoint := range input.SellingPoints {
		if name := strings.TrimSpace(sellingPoint.Name); name != "" {
			allowedSellingPoints[name] = struct{}{}
		}
	}
	for index := range result.Variants {
		variant := &result.Variants[index]
		variant.Hook = strings.TrimSpace(variant.Hook)
		variant.ScriptText = strings.TrimSpace(variant.ScriptText)
		variant.EditingIntent = strings.TrimSpace(variant.EditingIntent)
		if variant.Hook == "" || variant.ScriptText == "" || variant.EditingIntent == "" {
			issues = append(issues, fmt.Sprintf("variant %d is incomplete", index+1))
			continue
		}
		spokenCharacters := CountScriptSpokenCharacters(variant.ScriptText)
		if spokenCharacters < minimumCharacters || spokenCharacters > maximumCharacters {
			issues = append(issues, fmt.Sprintf("variant %d has %d spoken characters; target %ds requires %d to %d", index+1, spokenCharacters, targetDuration, minimumCharacters, maximumCharacters))
		}
		if phrase := firstScriptPhrase(variant.ScriptText, informationFeedClichePhrases); phrase != "" {
			issues = append(issues, fmt.Sprintf("variant %d contains generic advertising phrase %q", index+1, phrase))
		}
		clauses := scriptSemanticClauses(variant.ScriptText)
		minimumClauses := minimumScriptClauseCount(targetDuration)
		if len(clauses) < minimumClauses {
			issues = append(issues, fmt.Sprintf("variant %d has %d punctuated semantic clauses; target %ds requires at least %d", index+1, len(clauses), targetDuration, minimumClauses))
		}
		for clauseIndex, clause := range clauses {
			if CountScriptSpokenCharacters(clause) > 30 {
				issues = append(issues, fmt.Sprintf("variant %d clause %d is too dense; split it with natural punctuation", index+1, clauseIndex+1))
			}
		}
		normalizedHook := normalizeScriptComparisonText(variant.Hook)
		if normalizedHook != "" {
			if _, exists := seenHooks[normalizedHook]; exists {
				issues = append(issues, fmt.Sprintf("variant %d repeats another variant hook", index+1))
			}
			seenHooks[normalizedHook] = struct{}{}
			if !strings.HasPrefix(normalizeScriptComparisonText(variant.ScriptText), normalizedHook) {
				issues = append(issues, fmt.Sprintf("variant %d hook must be the opening words of script_text", index+1))
			}
		}
		if len(variant.Beats) < minimumBeats || len(variant.Beats) > maximumBeats {
			issues = append(issues, fmt.Sprintf("variant %d must contain %d to %d beats for %ds", index+1, minimumBeats, maximumBeats, targetDuration))
		}
		for beatIndex := range variant.Beats {
			beat := &variant.Beats[beatIndex]
			beat.Label = strings.TrimSpace(beat.Label)
			beat.SellingPoint = strings.TrimSpace(beat.SellingPoint)
			beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
			beat.SourceType = strings.TrimSpace(beat.SourceType)
			if beat.Label == "" || beat.SellingPoint == "" || beat.VisualGoal == "" {
				issues = append(issues, fmt.Sprintf("variant %d beat %d is incomplete", index+1, beatIndex+1))
				continue
			}
			if _, ok := allowedScriptSourceTypes[beat.SourceType]; !ok {
				issues = append(issues, fmt.Sprintf("variant %d beat %d has invalid source_type", index+1, beatIndex+1))
			}
			if len(allowedSellingPoints) > 0 {
				if _, ok := allowedSellingPoints[beat.SellingPoint]; !ok {
					issues = append(issues, fmt.Sprintf("variant %d beat %d uses unsupported selling_point %q", index+1, beatIndex+1, beat.SellingPoint))
				} else {
					coveredSellingPoints[beat.SellingPoint] = struct{}{}
				}
			}
			if len([]rune(beat.VisualGoal)) < 6 {
				issues = append(issues, fmt.Sprintf("variant %d beat %d visual_goal is too vague", index+1, beatIndex+1))
			}
			if phrase := firstScriptPhrase(beat.VisualGoal, abstractVisualGoalPhrases); phrase != "" {
				issues = append(issues, fmt.Sprintf("variant %d beat %d visual_goal contains abstract wording %q", index+1, beatIndex+1, phrase))
			}
		}
	}
	for sellingPoint := range allowedSellingPoints {
		if _, ok := coveredSellingPoints[sellingPoint]; !ok {
			issues = append(issues, fmt.Sprintf("response does not cover selling_point %q", sellingPoint))
		}
	}
	return issues
}

var informationFeedClichePhrases = []string{
	"今天给大家推荐", "实用神器", "不容错过", "赶紧入手", "闭眼入",
	"快来试试吧", "赶快试试吧", "值得拥有",
}

var abstractVisualGoalPhrases = []string{
	"展示产品优势", "展示产品特点", "突出核心卖点", "体现便利性", "营造氛围", "增强视觉吸引力",
}

func NormalizeScriptTargetDuration(value int) (int, bool) {
	if value == 0 {
		return DefaultScriptTargetDuration, true
	}
	switch value {
	case 15, 20, 30, 45, 60:
		return value, true
	default:
		return 0, false
	}
}

func ScriptSpokenCharacterRange(targetDurationSeconds int) (int, int) {
	targetDurationSeconds, ok := NormalizeScriptTargetDuration(targetDurationSeconds)
	if !ok {
		return 0, 0
	}
	target := float64(targetDurationSeconds) * scriptSpokenCharactersPerSecond
	return int(math.Ceil(target * 0.9)), int(math.Floor(target * 1.15))
}

func CountScriptSpokenCharacters(text string) int {
	count := 0
	for _, value := range text {
		if unicode.IsSpace(value) || unicode.IsPunct(value) || unicode.IsSymbol(value) {
			continue
		}
		count++
	}
	return count
}

func EstimateScriptDurationMs(text string) int {
	durationMs := int(math.Round(float64(CountScriptSpokenCharacters(text)) / scriptSpokenCharactersPerSecond * 1000))
	for _, value := range text {
		switch value {
		case '。', '！', '？', '.', '!', '?', '；', ';':
			durationMs += 260
		case '，', ',', '、', '：', ':':
			durationMs += 140
		}
	}
	if durationMs < 8000 {
		return 8000
	}
	return durationMs
}

func ScriptBeatCountRange(targetDurationSeconds int) (int, int) {
	switch targetDurationSeconds {
	case 15, 20:
		return 3, 5
	case 45:
		return 5, 7
	case 60:
		return 6, 8
	default:
		return 4, 6
	}
}

func minimumScriptClauseCount(targetDurationSeconds int) int {
	switch targetDurationSeconds {
	case 15:
		return 4
	case 20:
		return 5
	case 45:
		return 8
	case 60:
		return 10
	default:
		return 6
	}
}

func scriptSemanticClauses(text string) []string {
	parts := strings.FieldsFunc(text, func(value rune) bool {
		switch value {
		case '。', '！', '？', '.', '!', '?', '；', ';', '，', ',', '、', '：', ':':
			return true
		default:
			return false
		}
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}
	return result
}

func firstScriptPhrase(text string, phrases []string) string {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return phrase
		}
	}
	return ""
}

func normalizeScriptComparisonText(text string) string {
	return strings.Map(func(value rune) rune {
		if unicode.IsSpace(value) || unicode.IsPunct(value) || unicode.IsSymbol(value) {
			return -1
		}
		return value
	}, strings.TrimSpace(text))
}

func normalizeScriptGenerationRequestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return NewError(ErrorCodeTimeout, "llm request timed out", true, err)
	}
	return NewError(ErrorCodeProviderFailure, fmt.Sprintf("request llm failed: %v", err), true, err)
}
