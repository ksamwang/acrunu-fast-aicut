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
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultScriptGenerationMaxTokens     = 8192
	TTSVisualSourceType                  = "visual_only"
	DefaultScriptTargetDuration          = 30
	scriptSpokenCharactersPerSecond      = 5.0
	scriptRecommendedCharactersPerSecond = 4.5
	scriptMinimumCharacterRatio          = 0.9
	scriptMaximumCharacterRatio          = 1.1
	scriptMinimumDurationRatio           = 0.9
	scriptMaximumDurationRatio           = 1.12
	maximumScriptClauseCharacters        = 42
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

	copyPrompt := BuildScriptGenerationPrompt(input)
	copies, err := g.requestScriptCopies(ctx, copyPrompt, "", 0.75)
	if err != nil {
		return ScriptGenerationResult{}, err
	}
	if validationErr := ValidateScriptCopyResult(copies, input); validationErr != nil {
		previousJSON, _ := json.Marshal(copies)
		repairInstruction := "上一次文案 JSON 未通过服务端校验。请完整重写一次并修复列出的全部问题，仍然只输出广告口播，不得增加镜头、beats、visual_goal、素材解说或制作指令。校验错误：" + validationErr.Error() + "。上一次 JSON：" + string(previousJSON)
		repaired, repairErr := g.requestScriptCopies(ctx, copyPrompt, repairInstruction, 0.2)
		if repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		if repairErr := ValidateScriptCopyResult(repaired, input); repairErr != nil {
			return ScriptGenerationResult{}, repairErr
		}
		copies = repaired
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
	targetDuration, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Variants) != input.VariantCount {
		issues = append(issues, fmt.Sprintf("expected %d copy variants, got %d", input.VariantCount, len(result.Variants)))
	}
	allowedSellingPoints := scriptSellingPointSet(input.SellingPoints)
	coveredSellingPoints := make(map[string]struct{}, len(allowedSellingPoints))
	seenIndexes := map[int]struct{}{}
	seenAngles := map[string]struct{}{}
	seenHooks := map[string]struct{}{}
	_, maximumSellingPoints := ScriptSellingPointCountRange(targetDuration)
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
		} else {
			normalizedAngle := normalizeScriptComparisonText(variant.Angle)
			if _, exists := seenAngles[normalizedAngle]; exists {
				issues = append(issues, fmt.Sprintf("copy variant %d repeats another advertising angle", index+1))
			}
			seenAngles[normalizedAngle] = struct{}{}
		}
		issues = append(issues, validateScriptNarration(index+1, &variant.Hook, &variant.ScriptText, targetDuration, seenHooks)...)
		if len(variant.SelectedSellingPoints) < 1 || len(variant.SelectedSellingPoints) > maximumSellingPoints {
			issues = append(issues, fmt.Sprintf("copy variant %d must select 1 to %d selling points", index+1, maximumSellingPoints))
		}
		for _, sellingPoint := range variant.SelectedSellingPoints {
			if _, exists := allowedSellingPoints[sellingPoint]; !exists {
				issues = append(issues, fmt.Sprintf("copy variant %d uses unsupported selling_point %q", index+1, sellingPoint))
				continue
			}
			coveredSellingPoints[sellingPoint] = struct{}{}
		}
	}
	for sellingPoint := range allowedSellingPoints {
		if _, exists := coveredSellingPoints[sellingPoint]; !exists {
			issues = append(issues, fmt.Sprintf("copy variants do not cover selling_point %q", sellingPoint))
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
	targetDuration, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Plans) != len(copies.Variants) {
		issues = append(issues, fmt.Sprintf("expected %d visual plans, got %d", len(copies.Variants), len(result.Plans)))
	}
	minimumBeats, maximumBeats := ScriptBeatCountRange(targetDuration)
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
		if len(plan.Beats) < minimumBeats || len(plan.Beats) > maximumBeats {
			issues = append(issues, fmt.Sprintf("visual plan %d must contain %d to %d beats", planIndex+1, minimumBeats, maximumBeats))
		}
		allowedSellingPoints := make(map[string]struct{}, len(copyVariant.SelectedSellingPoints))
		for _, sellingPoint := range copyVariant.SelectedSellingPoints {
			allowedSellingPoints[sellingPoint] = struct{}{}
		}
		coveredSellingPoints := make(map[string]struct{}, len(allowedSellingPoints))
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
			} else {
				coveredSellingPoints[beat.SellingPoint] = struct{}{}
			}
			issues = append(issues, validateScriptVisualGoal(planIndex+1, beatIndex+1, beat.VisualGoal)...)
		}
		for sellingPoint := range allowedSellingPoints {
			if _, exists := coveredSellingPoints[sellingPoint]; !exists {
				issues = append(issues, fmt.Sprintf("visual plan %d does not cover selling_point %q", planIndex+1, sellingPoint))
			}
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
	targetDuration, ok := NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return append(issues, "target_duration_seconds is unsupported")
	}
	if len(result.Variants) != input.VariantCount {
		issues = append(issues, fmt.Sprintf("expected %d script variants, got %d", input.VariantCount, len(result.Variants)))
	}
	minimumBeats, maximumBeats := ScriptBeatCountRange(targetDuration)
	seenHooks := map[string]struct{}{}
	allowedSellingPoints := scriptSellingPointSet(input.SellingPoints)
	coveredSellingPoints := make(map[string]struct{}, len(input.SellingPoints))
	for index := range result.Variants {
		variant := &result.Variants[index]
		variant.Hook = strings.TrimSpace(variant.Hook)
		variant.ScriptText = strings.TrimSpace(variant.ScriptText)
		variant.EditingIntent = strings.TrimSpace(variant.EditingIntent)
		if variant.Hook == "" || variant.ScriptText == "" || variant.EditingIntent == "" {
			issues = append(issues, fmt.Sprintf("variant %d is incomplete", index+1))
			continue
		}
		issues = append(issues, validateScriptNarration(index+1, &variant.Hook, &variant.ScriptText, targetDuration, seenHooks)...)
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
			issues = append(issues, validateScriptVisualGoal(index+1, beatIndex+1, beat.VisualGoal)...)
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

var scriptProductionDirectionPhrases = []string{
	"画面里", "镜头中", "镜头里", "转到暗光", "转到夜间", "双手回到车把",
	"袋口回到闭合状态", "最后拉上拉链", "最后合上拉链", "不用靠描述", "直接看清",
	"俯拍", "镜头切换", "运镜",
}

var abstractVisualGoalPhrases = []string{
	"展示产品优势", "展示产品特点", "突出核心卖点", "体现便利性", "营造氛围", "增强视觉吸引力",
	"特写", "俯拍", "镜头切换", "运镜",
}

func validateScriptNarration(index int, hook *string, scriptText *string, targetDuration int, seenHooks map[string]struct{}) []string {
	issues := make([]string, 0)
	*hook = strings.TrimSpace(*hook)
	*scriptText = strings.TrimSpace(*scriptText)
	if *hook == "" || *scriptText == "" {
		return append(issues, fmt.Sprintf("variant %d hook and script_text are required", index))
	}
	minimumDurationMs, maximumDurationMs := ScriptEstimatedDurationRangeMs(targetDuration)
	estimatedDurationMs := EstimateScriptDurationMs(*scriptText)
	if estimatedDurationMs < minimumDurationMs || estimatedDurationMs > maximumDurationMs {
		issues = append(issues, fmt.Sprintf("variant %d estimated duration is %.1fs; target %ds requires %.1fs to %.1fs", index, float64(estimatedDurationMs)/1000, targetDuration, float64(minimumDurationMs)/1000, float64(maximumDurationMs)/1000))
	}
	if phrase := firstScriptPhrase(*scriptText, informationFeedClichePhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("variant %d contains generic advertising phrase %q", index, phrase))
	}
	if phrase := firstScriptPhrase(*scriptText, scriptProductionDirectionPhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("variant %d contains production-direction phrase %q", index, phrase))
	}
	clauses := scriptSemanticClauses(*scriptText)
	minimumClauses, maximumClauses := ScriptClauseCountRange(targetDuration)
	if len(clauses) < minimumClauses || len(clauses) > maximumClauses {
		issues = append(issues, fmt.Sprintf("variant %d has %d semantic clauses; target %ds requires %d to %d", index, len(clauses), targetDuration, minimumClauses, maximumClauses))
	}
	for clauseIndex, clause := range clauses {
		if CountScriptSpokenCharacters(clause) > maximumScriptClauseCharacters {
			issues = append(issues, fmt.Sprintf("variant %d clause %d is too dense; split it with natural punctuation", index, clauseIndex+1))
		}
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

func ScriptSpokenCharacterRange(targetDurationSeconds int) (int, int) {
	targetDurationSeconds, ok := NormalizeScriptTargetDuration(targetDurationSeconds)
	if !ok {
		return 0, 0
	}
	target := float64(targetDurationSeconds) * scriptRecommendedCharactersPerSecond
	return int(math.Ceil(target * scriptMinimumCharacterRatio)), int(math.Floor(target * scriptMaximumCharacterRatio))
}

func ScriptEstimatedDurationRangeMs(targetDurationSeconds int) (int, int) {
	targetDurationSeconds, ok := NormalizeScriptTargetDuration(targetDurationSeconds)
	if !ok {
		return 0, 0
	}
	targetMs := float64(targetDurationSeconds * 1000)
	return int(math.Ceil(targetMs * scriptMinimumDurationRatio)), int(math.Floor(targetMs * scriptMaximumDurationRatio))
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

func ScriptClauseCountRange(targetDurationSeconds int) (int, int) {
	switch targetDurationSeconds {
	case 15:
		return 3, 7
	case 20:
		return 4, 8
	case 45:
		return 8, 14
	case 60:
		return 10, 18
	default:
		return 6, 10
	}
}

func ScriptSellingPointCountRange(targetDurationSeconds int) (int, int) {
	switch targetDurationSeconds {
	case 15:
		return 1, 2
	case 20:
		return 1, 3
	case 45:
		return 1, 5
	case 60:
		return 1, 6
	default:
		return 1, 4
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
