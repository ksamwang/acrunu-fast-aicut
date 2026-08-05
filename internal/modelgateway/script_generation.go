package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultScriptGenerationMaxTokens    = 8192
	TTSVisualSourceType                 = "visual_only"
	DefaultScriptTargetDuration         = 30
	DefaultScriptGenerationTemperature  = 0.75
	scriptSpokenCharactersPerSecond     = 5.0
	minimumUsableScriptSpokenCharacters = 30
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
	Temperature             *float64                       `json:"temperature,omitempty"`
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

type ScriptCopyVariant struct {
	VariantIndex          int      `json:"variant_index"`
	Angle                 string   `json:"angle"`
	SelectedSellingPoints []string `json:"selected_selling_points"`
	Hook                  string   `json:"hook"`
	ScriptText            string   `json:"script_text"`
}

type ScriptCopyResult struct {
	Variants []ScriptCopyVariant `json:"variants"`
}

type ScriptVisualIntentPlan struct {
	VariantIndex  int                    `json:"variant_index"`
	EditingIntent string                 `json:"editing_intent"`
	Beats         []ScriptGenerationBeat `json:"beats"`
}

type ScriptVisualIntentResult struct {
	Plans []ScriptVisualIntentPlan `json:"plans"`
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
	temperature, ok := NormalizeScriptGenerationTemperature(input.Temperature)
	if !ok {
		return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, "temperature must be between 0 and 2", false, nil)
	}
	input.Temperature = &temperature

	copyPrompt := BuildScriptGenerationPrompt(input)
	copies, err := g.requestScriptCopies(ctx, copyPrompt, "", temperature)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	initialValidationErr := ValidateScriptCopyResult(copies, input)
	initialQualityIssues := validateScriptCopyQualityIssues(copies, input)
	if initialValidationErr != nil || len(initialQualityIssues) > 0 {
		previousJSON, _ := json.Marshal(copies)
		repairIssues := append([]string{}, initialQualityIssues...)
		if initialValidationErr != nil {
			repairIssues = append(repairIssues, initialValidationErr.Error())
		}
		repairScope := "请完整重写一次并修复列出的全部问题。"
		if initialValidationErr == nil {
			repairScope = "只重写问题中列出的 variant；其余 variant 的所有字段必须原样返回。偏短时优先把已选择但尚未充分说明的真实卖点讲完整，不能加入空话或未提供的事实。"
		}
		repairInstruction := "上一次文案 JSON 需要优化。" + repairScope + "仍然只输出广告口播，不得增加镜头、beats、visual_goal、素材解说或制作指令。需要修复：" + strings.Join(repairIssues, "；") + "。上一次 JSON：" + string(previousJSON)
		repaired, repairErr := g.requestScriptCopies(ctx, copyPrompt, repairInstruction, 0.2)
		if repairErr != nil {
			if initialValidationErr != nil {
				return ScriptGenerationResult{}, repairErr
			}
		} else if repairedValidationErr := ValidateScriptCopyResult(repaired, input); repairedValidationErr != nil {
			if initialValidationErr != nil {
				return ScriptGenerationResult{}, repairedValidationErr
			}
		} else if initialValidationErr != nil {
			copies = repaired
		} else {
			copies = mergeScriptCopyQualityRepair(copies, repaired, input)
		}
	}
	sort.Slice(copies.Variants, func(i, j int) bool {
		return copies.Variants[i].VariantIndex < copies.Variants[j].VariantIndex
	})

	visualPrompt := BuildScriptVisualIntentPrompt(input, copies)
	visuals, err := g.requestScriptVisualIntents(ctx, visualPrompt, "", 0.3)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	if validationErr := ValidateScriptVisualIntentResult(visuals, copies, input); validationErr != nil {
		previousJSON, _ := json.Marshal(visuals)
		repairInstruction := "上一次视觉计划 JSON 未通过服务端校验。请完整重写一次并修复列出的全部问题，不得改写或返回已经确认的口播文案。校验错误：" + validationErr.Error() + "。上一次 JSON：" + string(previousJSON)
		repaired, repairErr := g.requestScriptVisualIntents(ctx, visualPrompt, repairInstruction, 0.15)
		if repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		if repairErr := ValidateScriptVisualIntentResult(repaired, copies, input); repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		visuals = repaired
	}

	result, err := mergeScriptGenerationResult(copies, visuals)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	if err := ValidateScriptGenerationResult(result, input); err != nil {
		return ScriptGenerationResult{}, err
	}
	return result, nil
}

func (g *OpenAICompatibleScriptGenerator) requestJSONContent(ctx context.Context, promptBundle PromptBundle, repairInstruction string, temperature float64) (string, error) {
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
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(g.baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return "", normalizeScriptGenerationRequestError(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", normalizeScriptGenerationRequestError(ctx, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return "", NewError(
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
		return "", NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode llm response failed: %v", err), false, err)
	}
	if len(chatResponse.Choices) == 0 || strings.TrimSpace(chatResponse.Choices[0].Message.Content) == "" {
		return "", NewError(ErrorCodeInvalidResponse, "llm response is empty", false, nil)
	}
	return strings.TrimSpace(chatResponse.Choices[0].Message.Content), nil
}

func (g *OpenAICompatibleScriptGenerator) requestScriptCopies(ctx context.Context, promptBundle PromptBundle, repairInstruction string, temperature float64) (ScriptCopyResult, error) {
	content, err := g.requestJSONContent(ctx, promptBundle, repairInstruction, temperature)
	if err != nil {
		return ScriptCopyResult{}, err
	}
	var result ScriptCopyResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		slog.Error("script copy JSON decode failed",
			slog.String("model", g.model),
			slog.String("prompt", promptBundle.Prompts[0].Name),
			slog.Int("content_bytes", len(content)),
			slog.String("decode_error", err.Error()),
			slog.String("raw_content", content),
		)
		return ScriptCopyResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode copy json output failed: %v", err), false, err)
	}
	return result, nil
}

func (g *OpenAICompatibleScriptGenerator) requestScriptVisualIntents(ctx context.Context, promptBundle PromptBundle, repairInstruction string, temperature float64) (ScriptVisualIntentResult, error) {
	content, err := g.requestJSONContent(ctx, promptBundle, repairInstruction, temperature)
	if err != nil {
		return ScriptVisualIntentResult{}, err
	}
	var result ScriptVisualIntentResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ScriptVisualIntentResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode visual-intent json output failed: %v", err), false, err)
	}
	return result, nil
}

func ValidateScriptCopyResult(result ScriptCopyResult, input ScriptGenerationInput) error {
	issues := validateScriptCopyResultIssues(result, input)
	if len(issues) == 0 {
		return nil
	}
	return NewError(ErrorCodeInvalidResponse, "invalid script copy: "+strings.Join(issues, "; "), false, nil)
}

func validateScriptCopyResultIssues(result ScriptCopyResult, input ScriptGenerationInput) []string {
	issues := make([]string, 0)
	if input.VariantCount < 1 || input.VariantCount > 8 {
		return append(issues, "invalid expected variant count")
	}
	_, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Variants) != input.VariantCount {
		issues = append(issues, fmt.Sprintf("expected %d copy variants, got %d", input.VariantCount, len(result.Variants)))
	}
	allowedSellingPoints := scriptSellingPointSet(input.SellingPoints)
	seenIndexes := map[int]struct{}{}
	seenHooks := map[string]struct{}{}
	for index := range result.Variants {
		variant := &result.Variants[index]
		variant.Angle = strings.TrimSpace(variant.Angle)
		variant.Hook = strings.TrimSpace(variant.Hook)
		variant.ScriptText = strings.TrimSpace(variant.ScriptText)
		variant.SelectedSellingPoints = normalizeScriptSellingPointNames(variant.SelectedSellingPoints)
		if variant.VariantIndex < 1 || variant.VariantIndex > input.VariantCount {
			issues = append(issues, fmt.Sprintf("copy variant %d has invalid variant_index %d", index+1, variant.VariantIndex))
		} else if _, exists := seenIndexes[variant.VariantIndex]; exists {
			issues = append(issues, fmt.Sprintf("copy variant %d repeats variant_index %d", index+1, variant.VariantIndex))
		} else {
			seenIndexes[variant.VariantIndex] = struct{}{}
		}
		if variant.Angle == "" {
			issues = append(issues, fmt.Sprintf("copy variant %d angle is required", index+1))
		}
		issues = append(issues, validateScriptNarration(index+1, &variant.Hook, &variant.ScriptText, seenHooks)...)
		if len(variant.SelectedSellingPoints) < 1 {
			issues = append(issues, fmt.Sprintf("copy variant %d must select at least one selling point", index+1))
		}
		for _, sellingPoint := range variant.SelectedSellingPoints {
			if _, exists := allowedSellingPoints[sellingPoint]; !exists {
				issues = append(issues, fmt.Sprintf("copy variant %d uses unsupported selling_point %q", index+1, sellingPoint))
			}
		}
	}
	return issues
}

func ValidateScriptVisualIntentResult(result ScriptVisualIntentResult, copies ScriptCopyResult, input ScriptGenerationInput) error {
	issues := validateScriptVisualIntentResultIssues(result, copies, input)
	if len(issues) == 0 {
		return nil
	}
	return NewError(ErrorCodeInvalidResponse, "invalid script visual intent: "+strings.Join(issues, "; "), false, nil)
}

func validateScriptVisualIntentResultIssues(result ScriptVisualIntentResult, copies ScriptCopyResult, input ScriptGenerationInput) []string {
	issues := make([]string, 0)
	_, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Plans) != len(copies.Variants) {
		issues = append(issues, fmt.Sprintf("expected %d visual plans, got %d", len(copies.Variants), len(result.Plans)))
	}
	copyByIndex := make(map[int]ScriptCopyVariant, len(copies.Variants))
	for _, copyVariant := range copies.Variants {
		copyByIndex[copyVariant.VariantIndex] = copyVariant
	}
	seenIndexes := map[int]struct{}{}
	for planIndex := range result.Plans {
		plan := &result.Plans[planIndex]
		plan.EditingIntent = strings.TrimSpace(plan.EditingIntent)
		copyVariant, exists := copyByIndex[plan.VariantIndex]
		if !exists {
			issues = append(issues, fmt.Sprintf("visual plan %d has unknown variant_index %d", planIndex+1, plan.VariantIndex))
			continue
		}
		if _, duplicate := seenIndexes[plan.VariantIndex]; duplicate {
			issues = append(issues, fmt.Sprintf("visual plan %d repeats variant_index %d", planIndex+1, plan.VariantIndex))
		} else {
			seenIndexes[plan.VariantIndex] = struct{}{}
		}
		if plan.EditingIntent == "" {
			issues = append(issues, fmt.Sprintf("visual plan %d editing_intent is required", planIndex+1))
		}
		allowedSellingPoints := make(map[string]struct{}, len(copyVariant.SelectedSellingPoints))
		for _, sellingPoint := range copyVariant.SelectedSellingPoints {
			allowedSellingPoints[sellingPoint] = struct{}{}
		}
		for beatIndex := range plan.Beats {
			beat := &plan.Beats[beatIndex]
			beat.Label = strings.TrimSpace(beat.Label)
			beat.SellingPoint = strings.TrimSpace(beat.SellingPoint)
			beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
			beat.SourceType = strings.TrimSpace(beat.SourceType)
			if beat.Label == "" || beat.SellingPoint == "" || beat.VisualGoal == "" {
				issues = append(issues, fmt.Sprintf("visual plan %d beat %d is incomplete", planIndex+1, beatIndex+1))
				continue
			}
			if beat.SourceType != TTSVisualSourceType {
				issues = append(issues, fmt.Sprintf("visual plan %d beat %d has invalid source_type", planIndex+1, beatIndex+1))
			}
			if _, exists := allowedSellingPoints[beat.SellingPoint]; !exists {
				issues = append(issues, fmt.Sprintf("visual plan %d beat %d uses selling_point %q outside its approved copy", planIndex+1, beatIndex+1, beat.SellingPoint))
			}
			issues = append(issues, validateScriptVisualGoal(planIndex+1, beatIndex+1, beat.VisualGoal)...)
		}
	}
	for variantIndex := range copyByIndex {
		if _, exists := seenIndexes[variantIndex]; !exists {
			issues = append(issues, fmt.Sprintf("visual plan is missing variant_index %d", variantIndex))
		}
	}
	return issues
}

func mergeScriptGenerationResult(copies ScriptCopyResult, visuals ScriptVisualIntentResult) (ScriptGenerationResult, error) {
	plans := make(map[int]ScriptVisualIntentPlan, len(visuals.Plans))
	for _, plan := range visuals.Plans {
		plans[plan.VariantIndex] = plan
	}
	result := ScriptGenerationResult{Variants: make([]ScriptGenerationVariant, 0, len(copies.Variants))}
	for _, copyVariant := range copies.Variants {
		plan, exists := plans[copyVariant.VariantIndex]
		if !exists {
			return ScriptGenerationResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual plan is missing variant_index %d", copyVariant.VariantIndex), false, nil)
		}
		result.Variants = append(result.Variants, ScriptGenerationVariant{
			Hook:          copyVariant.Hook,
			ScriptText:    copyVariant.ScriptText,
			EditingIntent: plan.EditingIntent,
			Beats:         plan.Beats,
		})
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
	_, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Variants) != input.VariantCount {
		issues = append(issues, fmt.Sprintf("expected %d script variants, got %d", input.VariantCount, len(result.Variants)))
	}
	seenHooks := map[string]struct{}{}
	allowedSellingPoints := scriptSellingPointSet(input.SellingPoints)
	for index := range result.Variants {
		variant := &result.Variants[index]
		variant.Hook = strings.TrimSpace(variant.Hook)
		variant.ScriptText = strings.TrimSpace(variant.ScriptText)
		variant.EditingIntent = strings.TrimSpace(variant.EditingIntent)
		if variant.Hook == "" || variant.ScriptText == "" || variant.EditingIntent == "" {
			issues = append(issues, fmt.Sprintf("variant %d is incomplete", index+1))
			continue
		}
		issues = append(issues, validateScriptNarration(index+1, &variant.Hook, &variant.ScriptText, seenHooks)...)
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
				}
			}
			issues = append(issues, validateScriptVisualGoal(index+1, beatIndex+1, beat.VisualGoal)...)
		}
	}
	return issues
}

func NormalizeScriptGenerationTemperature(value *float64) (float64, bool) {
	if value == nil {
		return DefaultScriptGenerationTemperature, true
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 2 {
		return 0, false
	}
	return *value, true
}

var scriptProductionDirectionPhrases = []string{
	"画面里", "镜头中", "镜头里", "转到暗光", "转到夜间", "双手回到车把",
	"袋口回到闭合状态", "最后拉上拉链", "最后合上拉链", "不用靠描述", "直接看清",
	"俯拍", "镜头切换", "运镜",
}

var abstractVisualGoalPhrases = []string{
	"展示产品优势", "展示产品特点", "突出核心卖点", "体现便利性", "营造氛围", "增强视觉吸引力",
	"特写", "俯拍", "镜头切换", "运镜",
}

func validateScriptNarration(index int, hook *string, scriptText *string, seenHooks map[string]struct{}) []string {
	issues := make([]string, 0)
	*hook = strings.TrimSpace(*hook)
	*scriptText = strings.TrimSpace(*scriptText)
	if *hook == "" || *scriptText == "" {
		return append(issues, fmt.Sprintf("variant %d hook and script_text are required", index))
	}
	spokenCharacters := CountScriptSpokenCharacters(*scriptText)
	if spokenCharacters < minimumUsableScriptSpokenCharacters {
		issues = append(issues, fmt.Sprintf("variant %d has %d spoken characters; minimum usable copy is %d", index, spokenCharacters, minimumUsableScriptSpokenCharacters))
	}
	normalizedHook := normalizeScriptComparisonText(*hook)
	if normalizedHook != "" {
		if _, exists := seenHooks[normalizedHook]; exists {
			issues = append(issues, fmt.Sprintf("variant %d repeats another variant hook", index))
		}
		seenHooks[normalizedHook] = struct{}{}
		if !strings.HasPrefix(normalizeScriptComparisonText(*scriptText), normalizedHook) {
			issues = append(issues, fmt.Sprintf("variant %d hook must be the opening words of script_text", index))
		}
	}
	return issues
}

func validateScriptCopyQualityIssues(result ScriptCopyResult, input ScriptGenerationInput) []string {
	issues := make([]string, 0)
	for index, variant := range result.Variants {
		issues = append(issues, validateScriptCopyVariantQualityIssues(index+1, variant, input)...)
	}
	return issues
}

func validateScriptCopyVariantQualityIssues(index int, variant ScriptCopyVariant, _ ScriptGenerationInput) []string {
	scriptText := strings.TrimSpace(variant.ScriptText)
	if scriptText == "" {
		return nil
	}
	issues := make([]string, 0, 1)
	if phrase := firstScriptPhrase(scriptText, scriptProductionDirectionPhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("variant %d contains production-direction phrase %q", index, phrase))
	}
	return issues
}

func mergeScriptCopyQualityRepair(original ScriptCopyResult, repaired ScriptCopyResult, input ScriptGenerationInput) ScriptCopyResult {
	repairedByIndex := make(map[int]ScriptCopyVariant, len(repaired.Variants))
	for _, variant := range repaired.Variants {
		repairedByIndex[variant.VariantIndex] = variant
	}
	merged := ScriptCopyResult{Variants: append([]ScriptCopyVariant(nil), original.Variants...)}
	for index, originalVariant := range merged.Variants {
		originalIssues := validateScriptCopyVariantQualityIssues(index+1, originalVariant, input)
		if len(originalIssues) == 0 {
			continue
		}
		repairedVariant, exists := repairedByIndex[originalVariant.VariantIndex]
		if !exists {
			continue
		}
		repairedIssues := validateScriptCopyVariantQualityIssues(index+1, repairedVariant, input)
		if len(repairedIssues) < len(originalIssues) {
			merged.Variants[index] = repairedVariant
		}
	}
	if err := ValidateScriptCopyResult(merged, input); err != nil {
		return original
	}
	return merged
}

func validateScriptVisualGoal(variantIndex int, beatIndex int, visualGoal string) []string {
	issues := make([]string, 0)
	if len([]rune(visualGoal)) < 6 {
		issues = append(issues, fmt.Sprintf("variant %d beat %d visual_goal is too vague", variantIndex, beatIndex))
	}
	if phrase := firstScriptPhrase(visualGoal, abstractVisualGoalPhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("variant %d beat %d visual_goal contains abstract or production wording %q", variantIndex, beatIndex, phrase))
	}
	return issues
}

func scriptSellingPointSet(sellingPoints []ScriptGenerationSellingPoint) map[string]struct{} {
	result := make(map[string]struct{}, len(sellingPoints))
	for _, sellingPoint := range sellingPoints {
		if name := strings.TrimSpace(sellingPoint.Name); name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func normalizeScriptSellingPointNames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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
