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

const (
	MinimumEditPlanClipDurationMs       = 800
	MaximumEditPlanClipDurationMs       = 3500
	MinimumActionEditPlanClipDurationMs = 2800
	MaximumEditPlanClipsPerBeat         = 4
	MaximumEditPlanEarlyTransitionMs    = 300
)

type EditPlanCandidate struct {
	ID               string  `json:"id"`
	SourceType       string  `json:"source_type"`
	SourceInMs       int     `json:"-"`
	SourceOutMs      int     `json:"-"`
	SemanticSummary  string  `json:"semantic_summary"`
	SemanticScore    float64 `json:"semantic_score"`
	BatchUseCount    int     `json:"batch_use_count"`
	AssetID          string  `json:"-"`
	UseOriginalAudio bool    `json:"-"`
}

const (
	EditPlanSlotRolePrimary       = "primary"
	EditPlanSlotRoleActionPrimary = "action_primary"
	EditPlanSlotRoleSupport       = "support"
)

type EditPlanSlot struct {
	ID                        string              `json:"id"`
	StartMs                   int                 `json:"-"`
	EndMs                     int                 `json:"-"`
	DurationMs                int                 `json:"duration_ms"`
	Role                      string              `json:"role"`
	MaximumEarlyEndMs         int                 `json:"-"`
	MaximumLeadingExtensionMs int                 `json:"-"`
	Candidates                []EditPlanCandidate `json:"candidates"`
}

type EditPlanRequirement struct {
	VisualBeatID        string         `json:"-"`
	NarrationSegmentID  string         `json:"-"`
	NarrationSegmentIDs []string       `json:"-"`
	NarrativeBeatID     string         `json:"-"`
	StartMs             int            `json:"-"`
	EndMs               int            `json:"-"`
	DurationClass       string         `json:"duration_class"`
	NarrationText       string         `json:"narration_text"`
	Label               string         `json:"label"`
	SellingPoint        string         `json:"selling_point,omitempty"`
	VisualGoal          string         `json:"visual_goal,omitempty"`
	SourceType          string         `json:"source_type"`
	Slots               []EditPlanSlot `json:"slots"`
}

type EditPlanInput struct {
	ProductName  string                `json:"product_name"`
	ScriptText   string                `json:"script_text"`
	Requirements []EditPlanRequirement `json:"requirements"`
}

type EditPlanClipChoice struct {
	SlotID      string `json:"slot_id"`
	CandidateID string `json:"candidate_id"`
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
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &OpenAICompatibleEditPlanner{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     strings.TrimSpace(cfg.Model),
		maxTokens: cfg.MaxTokens,
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
		"temperature":     0.25,
	}
	if p.maxTokens > 0 {
		payload["max_tokens"] = p.maxTokens
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
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner response failed: %v", err), false, err)
	}
	finishReason := ""
	content := ""
	if len(chatResponse.Choices) > 0 {
		finishReason = strings.TrimSpace(chatResponse.Choices[0].FinishReason)
		content = chatResponse.Choices[0].Message.Content
	}
	p.logResponseMetadata(
		promptBundle.Prompts[0].Name,
		firstNonEmptyString(chatResponse.Model, p.model),
		len(chatResponse.Choices),
		finishReason,
		len([]byte(content)),
	)
	if strings.TrimSpace(content) == "" {
		return NewError(
			ErrorCodeInvalidResponse,
			fmt.Sprintf("edit planner response is empty (choices=%d, finish_reason=%q)", len(chatResponse.Choices), finishReason),
			false,
			nil,
		)
	}
	if err := json.Unmarshal([]byte(content), result); err != nil {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("decode edit planner JSON output failed: %v", err), false, err)
	}
	return nil
}

func (p *OpenAICompatibleEditPlanner) logResponseMetadata(promptName string, responseModel string, choiceCount int, finishReason string, contentBytes int) {
	logger := p.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("llm planner response metadata",
		slog.String("prompt", promptName),
		slog.String("response_model", responseModel),
		slog.Int("choice_count", choiceCount),
		slog.String("finish_reason", finishReason),
		slog.Int("content_bytes", contentBytes),
	)
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
	if err := validateEditPlanRequirements(requirements); err != nil {
		return err
	}
	slots := flattenEditPlanSlots(requirements)
	if len(result.Clips) != len(slots) {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("expected exactly %d edit plan selections, got %d", len(slots), len(result.Clips)), false, nil)
	}
	usedCandidateIDs := make(map[string]int, len(result.Clips))
	for index := range result.Clips {
		clip := &result.Clips[index]
		clip.SlotID = strings.TrimSpace(clip.SlotID)
		clip.CandidateID = strings.TrimSpace(clip.CandidateID)
		if clip.SlotID == "" || clip.CandidateID == "" {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan selection %d is incomplete", index+1), false, nil)
		}
		slot := slots[index]
		if clip.SlotID != slot.ID {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan selection %d is out of slot order: expected %q, got %q", index+1, slot.ID, clip.SlotID), false, nil)
		}
		_, ok := findEditPlanCandidate(slot.Candidates, clip.CandidateID)
		if !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan slot %q selects material %q outside its allowed set", slot.ID, clip.CandidateID), false, nil)
		}
		if previousIndex, exists := usedCandidateIDs[clip.CandidateID]; exists {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("edit plan slot %q reuses material %q already selected by slot %q", slot.ID, clip.CandidateID, slots[previousIndex].ID), false, nil)
		}
		usedCandidateIDs[clip.CandidateID] = index
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
	seenSlots := map[string]struct{}{}
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
		if requirement.SourceType != TTSVisualSourceType {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d source type is invalid", index+1), false, nil)
		}
		if _, exists := seenVisualBeats[requirement.VisualBeatID]; exists {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan visual beat %q is repeated", requirement.VisualBeatID), false, nil)
		}
		if len(requirement.Slots) == 0 || len(requirement.Slots) > MaximumEditPlanClipsPerBeat {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d has an invalid slot count", index+1), false, nil)
		}
		expectedSlotStartMs := requirement.StartMs
		hasActionPrimary := false
		for slotIndex := range requirement.Slots {
			slot := &requirement.Slots[slotIndex]
			slot.ID = strings.TrimSpace(slot.ID)
			slot.Role = strings.TrimSpace(slot.Role)
			if slot.ID == "" || slot.StartMs != expectedSlotStartMs || slot.EndMs <= slot.StartMs || slot.EndMs > requirement.EndMs || slot.DurationMs != slot.EndMs-slot.StartMs {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d slot %d is invalid", index+1, slotIndex+1), false, nil)
			}
			if slot.DurationMs < MinimumEditPlanClipDurationMs || slot.DurationMs > MaximumEditPlanClipDurationMs {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q duration is outside %d-%dms", slot.ID, MinimumEditPlanClipDurationMs, MaximumEditPlanClipDurationMs), false, nil)
			}
			if slot.MaximumEarlyEndMs < 0 || slot.MaximumEarlyEndMs > MaximumEditPlanEarlyTransitionMs ||
				slot.MaximumLeadingExtensionMs < 0 || slot.MaximumLeadingExtensionMs > MaximumEditPlanEarlyTransitionMs ||
				(slot.MaximumEarlyEndMs > 0 && slot.MaximumLeadingExtensionMs > 0) {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q early-transition allowance is invalid", slot.ID), false, nil)
			}
			if slot.MaximumEarlyEndMs > 0 && (index == len(requirements)-1 || slotIndex != len(requirement.Slots)-1 || slot.DurationMs-slot.MaximumEarlyEndMs < MinimumEditPlanClipDurationMs) {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q cannot end early", slot.ID), false, nil)
			}
			if slot.MaximumLeadingExtensionMs > 0 && (index == 0 || slotIndex != 0 || slot.DurationMs+slot.MaximumLeadingExtensionMs > MaximumEditPlanClipDurationMs) {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q cannot absorb an early transition", slot.ID), false, nil)
			}
			if slot.Role != EditPlanSlotRolePrimary && slot.Role != EditPlanSlotRoleActionPrimary && slot.Role != EditPlanSlotRoleSupport {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q role is invalid", slot.ID), false, nil)
			}
			if _, exists := seenSlots[slot.ID]; exists {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q is repeated", slot.ID), false, nil)
			}
			if len(slot.Candidates) == 0 {
				return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q has no candidates", slot.ID), false, nil)
			}
			seenCandidates := map[string]struct{}{}
			for candidateIndex, candidate := range slot.Candidates {
				candidateID := strings.TrimSpace(candidate.ID)
				requiredDurationMs := slot.DurationMs - slot.MaximumEarlyEndMs + slot.MaximumLeadingExtensionMs
				if candidateID == "" || candidate.SourceInMs < 0 || candidate.SourceOutMs-candidate.SourceInMs < requiredDurationMs {
					return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q candidate %d cannot cover its duration", slot.ID, candidateIndex+1), false, nil)
				}
				if _, exists := seenCandidates[candidateID]; exists {
					return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan slot %q candidate %q is repeated", slot.ID, candidateID), false, nil)
				}
				seenCandidates[candidateID] = struct{}{}
			}
			if slot.Role == EditPlanSlotRoleActionPrimary {
				hasActionPrimary = slot.DurationMs >= actionVisualBeatMinimumMs
			}
			seenSlots[slot.ID] = struct{}{}
			expectedSlotStartMs = slot.EndMs
		}
		if expectedSlotStartMs != requirement.EndMs {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan requirement %d slots do not cover its timeline", index+1), false, nil)
		}
		if requirement.DurationClass == VisualDurationClassAction && !hasActionPrimary {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan action beat %q has no complete action slot", requirement.VisualBeatID), false, nil)
		}
		seenVisualBeats[requirement.VisualBeatID] = struct{}{}
		expectedStartMs = requirement.EndMs
	}
	previousBoundaryUsesTolerance := false
	for index := 0; index < len(requirements)-1; index++ {
		outgoing := requirements[index].Slots[len(requirements[index].Slots)-1]
		incoming := requirements[index+1].Slots[0]
		usesTolerance := outgoing.MaximumEarlyEndMs > 0 || incoming.MaximumLeadingExtensionMs > 0
		if usesTolerance && (outgoing.MaximumEarlyEndMs == 0 || outgoing.MaximumEarlyEndMs != incoming.MaximumLeadingExtensionMs) {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("edit plan boundary after visual beat %q has unmatched early-transition allowances", requirements[index].VisualBeatID), false, nil)
		}
		if usesTolerance && previousBoundaryUsesTolerance {
			return NewError(ErrorCodeConfiguration, "edit plan cannot use early transitions at adjacent visual beat boundaries", false, nil)
		}
		previousBoundaryUsesTolerance = usesTolerance
	}
	return nil
}

func flattenEditPlanSlots(requirements []EditPlanRequirement) []EditPlanSlot {
	total := 0
	for _, requirement := range requirements {
		total += len(requirement.Slots)
	}
	result := make([]EditPlanSlot, 0, total)
	for _, requirement := range requirements {
		result = append(result, requirement.Slots...)
	}
	return result
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
