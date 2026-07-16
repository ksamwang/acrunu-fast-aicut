package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

var (
	ErrGenerationPlanInput      = errors.New("invalid generation planning input")
	ErrNoEligibleAssetCandidate = errors.New("no eligible asset candidate")
)

type GenerateEditPlanInput struct {
	GenerationRunID string
	ScriptVariantID string
	VoiceoverID     string
}

type GenerationPlanningService struct {
	runs                 *GenerationRunService
	voiceovers           generationWorkLoader
	productAssetService  *ProductAssetService
	candidateService     *AssetCandidateService
	systemConfigService  *SystemConfigService
	modelProviderService *ModelProviderService
	fallbackConfig       config.Config
	planner              modelgateway.EditPlanner
}

func NewGenerationPlanningService(
	runs *GenerationRunService,
	voiceovers generationWorkLoader,
	productAssets *ProductAssetService,
	candidates *AssetCandidateService,
	systemConfigs *SystemConfigService,
	modelProviders *ModelProviderService,
	fallback config.Config,
) *GenerationPlanningService {
	return &GenerationPlanningService{
		runs:                 runs,
		voiceovers:           voiceovers,
		productAssetService:  productAssets,
		candidateService:     candidates,
		systemConfigService:  systemConfigs,
		modelProviderService: modelProviders,
		fallbackConfig:       fallback,
	}
}

func (s *GenerationPlanningService) WithPlanner(planner modelgateway.EditPlanner) *GenerationPlanningService {
	if planner != nil {
		s.planner = planner
	}
	return s
}

func (s *GenerationPlanningService) Generate(ctx context.Context, input GenerateEditPlanInput) (EditPlan, error) {
	if s == nil || s.runs == nil || s.voiceovers == nil || s.productAssetService == nil || s.candidateService == nil {
		return EditPlan{}, fmt.Errorf("generation planning service is not configured")
	}
	input = normalizeGenerateEditPlanInput(input)
	if input.GenerationRunID == "" || input.ScriptVariantID == "" || input.VoiceoverID == "" {
		return EditPlan{}, fmt.Errorf("%w: run, script variant, and voiceover are required", ErrGenerationPlanInput)
	}
	run, err := s.runs.Get(ctx, input.GenerationRunID)
	if err != nil {
		return EditPlan{}, err
	}
	if run.ScriptVariantID != input.ScriptVariantID || run.VoiceoverID != input.VoiceoverID || run.VoiceoverTaskID == "" {
		return EditPlan{}, fmt.Errorf("%w: run does not match planning payload", ErrGenerationPlanInput)
	}
	product, err := s.productAssetService.GetProduct(run.ProductID)
	if err != nil {
		return EditPlan{}, err
	}
	if product.Status == "archived" {
		return EditPlan{}, ErrProductNotFound
	}
	work, err := s.voiceovers.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		return EditPlan{}, err
	}
	if len(work.NarrationSegments) == 0 {
		return EditPlan{}, fmt.Errorf("%w: narration segments are not ready", ErrGenerationPlanInput)
	}
	if err := validateNarrationTimeline(work.NarrationSegments, work.DurationMs); err != nil {
		return EditPlan{}, fmt.Errorf("%w: %v", ErrGenerationPlanInput, err)
	}

	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStageRetrieving, 78); err != nil {
		return EditPlan{}, err
	}
	requirements, err := BuildShotRequirements(work.NarrationSegments, work.Beats)
	if err != nil {
		return EditPlan{}, err
	}
	candidateSets, err := s.candidateService.Retrieve(ctx, run.ProductID, requirements, defaultCandidatesPerNarrationSegment)
	if err != nil {
		return EditPlan{}, err
	}
	if err := validateCandidateSets(candidateSets); err != nil {
		return EditPlan{}, err
	}
	candidateSnapshot, err := json.Marshal(candidateSets)
	if err != nil {
		return EditPlan{}, err
	}
	basePlan := EditPlan{
		GenerationRunID:   run.ID,
		ScriptVariantID:   run.ScriptVariantID,
		VoiceoverID:       run.VoiceoverID,
		Status:            "planning",
		CandidateSnapshot: candidateSnapshot,
		PlanJSON:          json.RawMessage("{}"),
		PromptVersion:     modelgateway.EditPlanPromptVersion,
	}
	if _, err := s.runs.SaveEditPlan(ctx, basePlan); err != nil {
		return EditPlan{}, err
	}
	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStagePlanning, 84); err != nil {
		return EditPlan{}, err
	}

	planner, provider, model, err := s.resolvePlanner(ctx)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	result, err := planner.PlanEdits(ctx, buildPlannerInput(product.Name, work.ScriptText, candidateSets))
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	clips, err := materializeEditPlan(result, candidateSets)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	planJSON, err := json.Marshal(map[string]any{
		"clips": clips,
	})
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	basePlan.Status = "ready"
	basePlan.PlanJSON = planJSON
	basePlan.Clips = clips
	basePlan.LLMProvider = provider
	basePlan.LLMModel = model
	basePlan.ErrorMessage = ""
	plan, err := s.runs.SaveEditPlan(ctx, basePlan)
	if err != nil {
		return EditPlan{}, err
	}
	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStagePlanReady, 88); err != nil {
		return EditPlan{}, err
	}
	return plan, nil
}

func (s *GenerationPlanningService) resolvePlanner(ctx context.Context) (modelgateway.EditPlanner, string, string, error) {
	if s.planner != nil {
		return s.planner, "test-provider", "test-model", nil
	}
	if err := EnsureLegacyOpenAICompatibleProvider(ctx, s.systemConfigService, s.modelProviderService); err != nil {
		return nil, "", "", err
	}
	cfg := ResolveLLMScriptConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	if strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, "", "", ErrLLMNotConfigured
	}
	return modelgateway.NewEditPlanner(cfg), cfg.Provider, cfg.Model, nil
}

func (s *GenerationPlanningService) persistPlanFailure(ctx context.Context, plan EditPlan, cause error) error {
	plan.Status = "failed"
	plan.ErrorMessage = cause.Error()
	plan.Clips = nil
	plan.PlanJSON = json.RawMessage("{}")
	if _, err := s.runs.SaveEditPlan(context.Background(), plan); err != nil {
		return fmt.Errorf("%w; persist edit plan failure: %v", cause, err)
	}
	return cause
}

func normalizeGenerateEditPlanInput(input GenerateEditPlanInput) GenerateEditPlanInput {
	input.GenerationRunID = strings.TrimSpace(input.GenerationRunID)
	input.ScriptVariantID = strings.TrimSpace(input.ScriptVariantID)
	input.VoiceoverID = strings.TrimSpace(input.VoiceoverID)
	return input
}

func validateCandidateSets(sets []CandidateSet) error {
	if len(sets) == 0 {
		return ErrNoEligibleAssetCandidate
	}
	for index, set := range sets {
		if err := validateShotRequirement(set.Requirement); err != nil {
			return err
		}
		if len(set.Candidates) == 0 {
			return fmt.Errorf("%w: narration segment %d has no eligible material", ErrNoEligibleAssetCandidate, index+1)
		}
	}
	return nil
}

func buildPlannerInput(productName string, scriptText string, sets []CandidateSet) modelgateway.EditPlanInput {
	requirements := make([]modelgateway.EditPlanRequirement, 0, len(sets))
	for _, set := range sets {
		candidates := make([]modelgateway.EditPlanCandidate, 0, len(set.Candidates))
		for _, candidate := range set.Candidates {
			candidates = append(candidates, modelgateway.EditPlanCandidate{
				ID:              candidate.ID,
				SourceType:      candidate.SourceType,
				SourceInMs:      candidate.SourceInMs,
				SourceOutMs:     candidate.SourceOutMs,
				SemanticSummary: candidate.SemanticSummary,
				SemanticScore:   candidate.SemanticScore,
			})
		}
		requirements = append(requirements, modelgateway.EditPlanRequirement{
			NarrationSegmentID: set.Requirement.NarrationSegmentID,
			StartMs:            set.Requirement.StartMs,
			EndMs:              set.Requirement.EndMs,
			NarrationText:      set.Requirement.NarrationText,
			SellingPoint:       set.Requirement.SellingPoint,
			VisualGoal:         set.Requirement.VisualGoal,
			SourceType:         set.Requirement.SourceType,
			Candidates:         candidates,
		})
	}
	return modelgateway.EditPlanInput{
		ProductName:  productName,
		ScriptText:   scriptText,
		Requirements: requirements,
	}
}

func materializeEditPlan(result modelgateway.EditPlanResult, sets []CandidateSet) ([]EditPlanClip, error) {
	requirements := make([]modelgateway.EditPlanRequirement, 0, len(sets))
	candidatesByNarration := map[string]map[string]AssetCandidate{}
	for _, set := range sets {
		requirements = append(requirements, modelgateway.EditPlanRequirement{
			NarrationSegmentID: set.Requirement.NarrationSegmentID,
			StartMs:            set.Requirement.StartMs,
			EndMs:              set.Requirement.EndMs,
			NarrationText:      set.Requirement.NarrationText,
			Candidates:         []modelgateway.EditPlanCandidate{{ID: "validated"}},
		})
		candidateMap := map[string]AssetCandidate{}
		for _, candidate := range set.Candidates {
			candidateMap[candidate.ID] = candidate
		}
		candidatesByNarration[set.Requirement.NarrationSegmentID] = candidateMap
	}
	if err := modelgateway.ValidateEditPlanResult(result, requirements); err != nil {
		return nil, err
	}
	choices := make(map[string]modelgateway.EditPlanClipChoice, len(result.Clips))
	for _, choice := range result.Clips {
		choices[choice.NarrationSegmentID] = choice
	}
	clips := make([]EditPlanClip, 0, len(sets))
	for _, set := range sets {
		requirement := set.Requirement
		choice, ok := choices[requirement.NarrationSegmentID]
		if !ok {
			return nil, fmt.Errorf("planner output is missing narration segment %q", requirement.NarrationSegmentID)
		}
		candidate, ok := candidatesByNarration[requirement.NarrationSegmentID][choice.CandidateID]
		if !ok {
			return nil, fmt.Errorf("planner selected candidate %q outside the allowed set", choice.CandidateID)
		}
		if choice.SourceInMs < candidate.SourceInMs || choice.SourceOutMs > candidate.SourceOutMs || choice.SourceOutMs <= choice.SourceInMs {
			return nil, fmt.Errorf("planner source range is outside candidate %q", candidate.ID)
		}
		durationMs := requirement.EndMs - requirement.StartMs
		if choice.SourceOutMs-choice.SourceInMs != durationMs {
			return nil, fmt.Errorf("planner source range must match narration segment %q", requirement.NarrationSegmentID)
		}
		clips = append(clips, EditPlanClip{
			ID:                 "",
			NarrationSegmentID: requirement.NarrationSegmentID,
			AssetID:            candidate.AssetID,
			SpeechSegmentID:    candidate.SpeechSegmentID,
			SourceInMs:         choice.SourceInMs,
			SourceOutMs:        choice.SourceOutMs,
			StartMs:            requirement.StartMs,
			EndMs:              requirement.EndMs,
			TimelineDurationMs: durationMs,
			Label:              strings.TrimSpace(choice.Label),
			VisualGoal:         strings.TrimSpace(choice.VisualGoal),
			SourceType:         candidate.SourceType,
			UseOriginalAudio:   candidate.DefaultUseOriginalAudio,
			AudioGainDB:        0,
		})
	}
	return clips, nil
}
