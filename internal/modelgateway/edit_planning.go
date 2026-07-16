package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maximumEditPlanOutputTokens = 3072

type EditPlanCandidate struct {
	ID              string  `json:"id"`
	SourceType      string  `json:"source_type"`
	SourceInMs      int     `json:"source_in_ms"`
	SourceOutMs     int     `json:"source_out_ms"`
	SemanticSummary string  `json:"semantic_summary"`
	SemanticScore   float64 `json:"semantic_score"`
}

type EditPlanRequirement struct {
	VisualBeatID       string              `json:"visual_beat_id"`
	NarrationSegmentID string              `json:"narration_segment_id"`
	StartMs            int                 `json:"start_ms"`
	EndMs              int                 `json:"end_ms"`
	NarrationText      string              `json:"narration_text"`
	SellingPoint       string              `json:"selling_point,omitempty"`
	VisualGoal         string              `json:"visual_goal,omitempty"`
	SourceType         string              `json:"source_type"`
	Candidates         []EditPlanCandidate `json:"candidates"`
}

type EditPlanInput struct {
	ProductName  string                `json:"product_name"`
	ScriptText   string                `json:"script_text"`
	Requirements []EditPlanRequirement `json:"requirements"`
}

type EditPlanClipChoice struct {
	VisualBeatID string `json:"visual_beat_id"`
	CandidateID  string `json:"candidate_id"`
	SourceInMs   int    `json:"source_in_ms"`
	SourceOutMs  int    `json:"source_out_ms"`
	Label        string `json:"label"`
	VisualGoal   string `json:"visual_goal"`
}

type EditPlanResult struct {
	Clips []EditPlanClipChoice `json:"clips"`
}

type EditPlanner interface {
	PlanEdits(context.Context, EditPlanInput) (EditPlanResult, error)
}

type editPlannerFunc func(context.Context, EditPlanInput) (EditPlanResult, error)

func (f editPlannerFunc) PlanEdits(ctx context.Context, input EditPlanInput) (EditPlanResult, error) {
	return f(ctx, input)
}

func NewEditPlanner(cfg Config) EditPlanner {
	switch strings.TrimSpace(cfg.Provider) {
	case "openai_compatible":
		return NewOpenAICompatibleEditPlanner(cfg)
	default:
		provider := strings.TrimSpace(cfg.Provider)
		if provider == "" {
			provider = "openai_compatible"
		}
		return editPlannerFunc(func(context.Context, EditPlanInput) (EditPlanResult, error) {
			return EditPlanResult{}, NewError(
				ErrorCodeUnsupportedProvider,
				fmt.Sprintf("llm provider %q is not implemented", provider),
				false,
				nil,
			)
		})
	}
}

type OpenAICompatibleEditPlanner struct {
	baseURL   string
	apiKey    string
	model     string
	maxTokens int
	timeout   time.Duration
	client    *http.Client
	logger    *slog.Logger
}

func NewOpenAICompatibleEditPlanner(cfg Config) *OpenAICompatibleEditPlanner {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 || maxTokens > maximumEditPlanOutputTokens {
		maxTokens = maximumEditPlanOutputTokens
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &OpenAICompatibleEditPlanner{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     strings.TrimSpace(cfg.Model),
		maxTokens: maxTokens,
		timeout:   timeout,
		client:    &http.Client{Timeout: timeout},
		logger:    slog.Default(),
	}
}

func (p *OpenAICompatibleEditPlanner) WithLogger(logger *slog.Logger) *OpenAICompatibleEditPlanner {
	if logger != nil {
		p.logger = logger
	}
	return p
}

func (p *OpenAICompatibleEditPlanner) PlanEdits(ctx context.Context, input EditPlanInput) (EditPlanResult, error) {
	if p == nil || p.baseURL == "" {
		return EditPlanResult{}, NewError(ErrorCodeConfiguration, "openai compatible base_url is required", false, nil)
	}
	if p.model == "" {
		return EditPlanResult{}, NewError(ErrorCodeConfiguration, "llm model is required", false, nil)
	}
	if err := validateEditPlanInput(input); err != nil {
		return EditPlanResult{}, err
	}

	promptBundle := BuildEditPlanPrompt(input)
	var result EditPlanResult
	if err := p.completeJSON(ctx, promptBundle, &result); err != nil {
		return EditPlanResult{}, err
	}
	if err := ValidateEditPlanResult(result, input.Requirements); err != nil {
		return EditPlanResult{}, err
	}
	return result, nil
}

func (p *OpenAICompatibleEditPlanner) completeJSON(ctx context.Context, promptBundle PromptBundle, result any) error {
	payload := map[string]any{
		"model": p.model,
		"messages": []map[string]any{
			{"role": "system", "content": promptBundle.Prompts[0].System},
			{"role": "user", "content": promptBundle.Prompts[0].User},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      p.maxTokens,
		"temperature":     0.25,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(p.baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.logRequestFailure(promptBundle.Prompts[0].Name, len(body), startedAt, err)
		return normalizeEditPlanRequestError(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		p.logRequestFailure(promptBundle.Prompts[0].Name, len(body), startedAt, err)
		return normalizeEditPlanRequestError(ctx, err)
	}
	p.logRequestResult(promptBundle.Prompts[0].Name, len(body), len(responseBody), startedAt, resp.StatusCode)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return NewError(
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
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner response failed: %v", err), false, err)
	}
	if len(chatResponse.Choices) == 0 || strings.TrimSpace(chatResponse.Choices[0].Message.Content) == "" {
		return NewError(ErrorCodeInvalidResponse, "edit planner response is empty", false, nil)
	}
	if err := json.Unmarshal([]byte(chatResponse.Choices[0].Message.Content), result); err != nil {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner JSON output failed: %v", err), false, err)
	}
	return nil
}

func (p *OpenAICompatibleEditPlanner) logRequestResult(promptName string, requestBytes int, responseBytes int, startedAt time.Time, statusCode int) {
	logger := p.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("llm planner request completed",
		slog.String("prompt", promptName),
		slog.String("model", p.model),
		slog.Int("request_bytes", requestBytes),
		slog.Int("response_bytes", responseBytes),
		slog.Int("status_code", statusCode),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
}

func (p *OpenAICompatibleEditPlanner) logRequestFailure(promptName string, requestBytes int, startedAt time.Time, err error) {
	logger := p.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("llm planner request failed",
		slog.String("prompt", promptName),
		slog.String("model", p.model),
		slog.Int("request_bytes", requestBytes),
		slog.Int64("timeout_ms", p.timeout.Milliseconds()),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		slog.String("error", err.Error()),
	)
}

func ValidateEditPlanResult(result EditPlanResult, requirements []EditPlanRequirement) error {
	if len(requirements) == 0 {
		return NewError(ErrorCodeInvalidResponse, "edit plan requirements are required", false, nil)
	}
	if len(result.Clips) != len(requirements) {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("expected %d edit plan clips, got %d", len(requirements), len(result.Clips)), false, nil)
	}
	if err := validateEditPlanRequirements(requirements); err != nil {
		return err
	}
	seenVisualBeats := map[string]struct{}{}
	for index := range result.Clips {
		clip := &result.Clips[index]
		clip.VisualBeatID = strings.TrimSpace(clip.VisualBeatID)
		clip.CandidateID = strings.TrimSpace(clip.CandidateID)
		clip.Label = strings.TrimSpace(clip.Label)
		clip.VisualGoal = strings.TrimSpace(clip.VisualGoal)
		if clip.VisualBeatID == "" || clip.CandidateID == "" || clip.Label == "" || clip.VisualGoal == "" {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d is incomplete", index+1), false, nil)
		}
		if clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d source range is invalid", index+1), false, nil)
		}
		requirement := requirements[index]
		if clip.VisualBeatID != requirement.VisualBeatID {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d is out of visual beat order", index+1), false, nil)
		}
		if _, exists := seenVisualBeats[clip.VisualBeatID]; exists {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan visual beat %q is repeated", clip.VisualBeatID), false, nil)
		}
		if clip.SourceOutMs-clip.SourceInMs != requirement.EndMs-requirement.StartMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d source duration does not match its visual beat", index+1), false, nil)
		}
		candidate, ok := findEditPlanCandidate(requirement.Candidates, clip.CandidateID)
		if !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d selects a candidate outside the allowed set", index+1), false, nil)
		}
		if clip.SourceInMs < candidate.SourceInMs || clip.SourceOutMs > candidate.SourceOutMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d source range is outside its candidate", index+1), false, nil)
		}
		seenVisualBeats[clip.VisualBeatID] = struct{}{}
	}
	for _, requirement := range requirements {
		if _, ok := seenVisualBeats[requirement.VisualBeatID]; !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan does not cover visual beat %q", requirement.VisualBeatID), false, nil)
		}
	}
	return nil
}

func validateEditPlanInput(input EditPlanInput) error {
	if strings.TrimSpace(input.ProductName) == "" || strings.TrimSpace(input.ScriptText) == "" {
		return NewError(ErrorCodeConfiguration, "product name and script text are required", false, nil)
	}
	if len(input.Requirements) == 0 {
		return NewError(ErrorCodeConfiguration, "edit plan requirements are required", false, nil)
	}
	return validateEditPlanRequirements(input.Requirements)
}

func validateEditPlanRequirements(requirements []EditPlanRequirement) error {
	if len(requirements) == 0 {
		return NewError(ErrorCodeConfiguration, "edit plan requirements are required", false, nil)
	}
	if requirements[0].StartMs != 0 {
		return NewError(ErrorCodeConfiguration, "edit plan visual timeline must start at 0", false, nil)
	}
	expectedStartMs := requirements[0].StartMs
	seenVisualBeats := map[string]struct{}{}
	for index := range requirements {
		requirement := &requirements[index]
		requirement.VisualBeatID = strings.TrimSpace(requirement.VisualBeatID)
		requirement.NarrationSegmentID = strings.TrimSpace(requirement.NarrationSegmentID)
		requirement.NarrationText = strings.TrimSpace(requirement.NarrationText)
		requirement.VisualGoal = strings.TrimSpace(requirement.VisualGoal)
		requirement.SourceType = strings.TrimSpace(requirement.SourceType)
		if requirement.VisualBeatID == "" || requirement.NarrationSegmentID == "" || requirement.NarrationText == "" || requirement.VisualGoal == "" || requirement.EndMs <= requirement.StartMs {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d is invalid", index+1), false, nil)
		}
		if requirement.StartMs != expectedStartMs {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d does not continue the visual timeline", index+1), false, nil)
		}
		if requirement.SourceType != "visual_only" && requirement.SourceType != "talking_head" && requirement.SourceType != "mixed" {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d source type is invalid", index+1), false, nil)
		}
		if _, exists := seenVisualBeats[requirement.VisualBeatID]; exists {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan visual beat %q is repeated", requirement.VisualBeatID), false, nil)
		}
		if len(requirement.Candidates) == 0 {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d has no candidates", index+1), false, nil)
		}
		for candidateIndex, candidate := range requirement.Candidates {
			if strings.TrimSpace(candidate.ID) == "" || candidate.SourceInMs < 0 || candidate.SourceOutMs <= candidate.SourceInMs {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d candidate %d is invalid", index+1, candidateIndex+1), false, nil)
			}
		}
		seenVisualBeats[requirement.VisualBeatID] = struct{}{}
		expectedStartMs = requirement.EndMs
	}
	return nil
}

func findEditPlanCandidate(candidates []EditPlanCandidate, candidateID string) (EditPlanCandidate, bool) {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == candidateID {
			return candidate, true
		}
	}
	return EditPlanCandidate{}, false
}

func normalizeEditPlanRequestError(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
		return NewError(ErrorCodeTimeout, "edit planner request timed out", true, err)
	}
	return NewError(ErrorCodeProviderFailure, fmt.Sprintf("request edit planner failed: %v", err), true, err)
}
