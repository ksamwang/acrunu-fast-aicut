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

const defaultEditPlanMaxTokens = 8192

type EditPlanCandidate struct {
	ID              string  `json:"id"`
	SourceType      string  `json:"source_type"`
	SourceInMs      int     `json:"source_in_ms"`
	SourceOutMs     int     `json:"source_out_ms"`
	SemanticSummary string  `json:"semantic_summary"`
	SemanticScore   float64 `json:"semantic_score"`
}

type EditPlanRequirement struct {
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
	NarrationSegmentID string `json:"narration_segment_id"`
	CandidateID        string `json:"candidate_id"`
	SourceInMs         int    `json:"source_in_ms"`
	SourceOutMs        int    `json:"source_out_ms"`
	Label              string `json:"label"`
	VisualGoal         string `json:"visual_goal"`
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
	client    *http.Client
}

func NewOpenAICompatibleEditPlanner(cfg Config) *OpenAICompatibleEditPlanner {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultEditPlanMaxTokens
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAICompatibleEditPlanner{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     strings.TrimSpace(cfg.Model),
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: timeout},
	}
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
		return EditPlanResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinOpenAICompatibleURL(p.baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return EditPlanResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return EditPlanResult{}, normalizeEditPlanRequestError(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return EditPlanResult{}, normalizeEditPlanRequestError(ctx, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return EditPlanResult{}, NewError(
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
		return EditPlanResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner response failed: %v", err), false, err)
	}
	if len(chatResponse.Choices) == 0 || strings.TrimSpace(chatResponse.Choices[0].Message.Content) == "" {
		return EditPlanResult{}, NewError(ErrorCodeInvalidResponse, "edit planner response is empty", false, nil)
	}
	var result EditPlanResult
	if err := json.Unmarshal([]byte(chatResponse.Choices[0].Message.Content), &result); err != nil {
		return EditPlanResult{}, NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner JSON output failed: %v", err), false, err)
	}
	if err := ValidateEditPlanResult(result, input.Requirements); err != nil {
		return EditPlanResult{}, err
	}
	return result, nil
}

func ValidateEditPlanResult(result EditPlanResult, requirements []EditPlanRequirement) error {
	if len(requirements) == 0 {
		return NewError(ErrorCodeInvalidResponse, "edit plan requirements are required", false, nil)
	}
	if len(result.Clips) != len(requirements) {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("expected %d edit plan clips, got %d", len(requirements), len(result.Clips)), false, nil)
	}
	seenNarrationSegments := map[string]struct{}{}
	for index := range result.Clips {
		clip := &result.Clips[index]
		clip.NarrationSegmentID = strings.TrimSpace(clip.NarrationSegmentID)
		clip.CandidateID = strings.TrimSpace(clip.CandidateID)
		clip.Label = strings.TrimSpace(clip.Label)
		clip.VisualGoal = strings.TrimSpace(clip.VisualGoal)
		if clip.NarrationSegmentID == "" || clip.CandidateID == "" || clip.Label == "" || clip.VisualGoal == "" {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d is incomplete", index+1), false, nil)
		}
		if clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d source range is invalid", index+1), false, nil)
		}
		if clip.NarrationSegmentID != strings.TrimSpace(requirements[index].NarrationSegmentID) {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan clip %d is out of narration order", index+1), false, nil)
		}
		if _, exists := seenNarrationSegments[clip.NarrationSegmentID]; exists {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan narration segment %q is repeated", clip.NarrationSegmentID), false, nil)
		}
		seenNarrationSegments[clip.NarrationSegmentID] = struct{}{}
	}
	for _, requirement := range requirements {
		if _, ok := seenNarrationSegments[strings.TrimSpace(requirement.NarrationSegmentID)]; !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan does not cover narration segment %q", requirement.NarrationSegmentID), false, nil)
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
	for index, requirement := range input.Requirements {
		if strings.TrimSpace(requirement.NarrationSegmentID) == "" || strings.TrimSpace(requirement.NarrationText) == "" || requirement.EndMs <= requirement.StartMs {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d is invalid", index+1), false, nil)
		}
		if len(requirement.Candidates) == 0 {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d has no candidates", index+1), false, nil)
		}
	}
	return nil
}

func normalizeEditPlanRequestError(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
		return NewError(ErrorCodeTimeout, "edit planner request timed out", true, err)
	}
	return NewError(ErrorCodeProviderFailure, fmt.Sprintf("request edit planner failed: %v", err), true, err)
}
