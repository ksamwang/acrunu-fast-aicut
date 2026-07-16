package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

var (
	ErrGenerationPlanInput      = errors.New("invalid generation planning input")
	ErrNoEligibleAssetCandidate = errors.New("no eligible asset candidate")
)

const (
	editPlannerCandidatesPerVisualBeat          = 6
	maximumPlannerCandidateSemanticSummaryRunes = 120
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
	visualPlanner        modelgateway.VisualPlanner
	logger               *slog.Logger
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
		logger:               slog.Default(),
	}
}

func (s *GenerationPlanningService) WithPlanner(planner modelgateway.EditPlanner) *GenerationPlanningService {
	if planner != nil {
		s.planner = planner
	}
	return s
}

func (s *GenerationPlanningService) WithVisualPlanner(planner modelgateway.VisualPlanner) *GenerationPlanningService {
	if planner != nil {
		s.visualPlanner = planner
	}
	return s
}

func (s *GenerationPlanningService) WithLogger(logger *slog.Logger) *GenerationPlanningService {
	if logger != nil {
		s.logger = logger
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

	basePlan := EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: run.ScriptVariantID,
		VoiceoverID:     run.VoiceoverID,
		Status:          "planning",
		PromptVersion:   modelgateway.VisualPlanPromptVersion + "+" + modelgateway.EditPlanPromptVersion,
	}
	if err := updatePlanArtifacts(&basePlan, nil, nil, nil); err != nil {
		return EditPlan{}, err
	}
	if _, err := s.runs.SaveEditPlan(ctx, basePlan); err != nil {
		return EditPlan{}, err
	}
	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStagePlanning, 76); err != nil {
		return EditPlan{}, err
	}

	visualPlanner, visualProvider, visualModel, err := s.resolveVisualPlanner(ctx)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	visualInput := buildVisualPlannerInput(product.Name, work)
	visualResult, err := visualPlanner.PlanVisuals(ctx, visualInput)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	visualBeats, err := materializeVisualBeats(visualResult, visualInput)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	basePlan.VisualBeats = visualBeats
	basePlan.LLMProvider = visualProvider
	basePlan.LLMModel = visualModel
	if err := updatePlanArtifacts(&basePlan, visualBeats, nil, nil); err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	if _, err := s.runs.SaveEditPlan(ctx, basePlan); err != nil {
		return EditPlan{}, err
	}

	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStageRetrieving, 80); err != nil {
		return EditPlan{}, err
	}
	requirements, err := BuildShotRequirements(visualBeats, work.NarrationSegments)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	candidateSets, err := s.candidateService.Retrieve(ctx, run.ProductID, requirements, editPlannerCandidatesPerVisualBeat)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	if err := updatePlanArtifacts(&basePlan, visualBeats, candidateSets, nil); err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	if _, err := s.runs.SaveEditPlan(ctx, basePlan); err != nil {
		return EditPlan{}, err
	}
	if err := validateCandidateSets(candidateSets); err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStagePlanning, 84); err != nil {
		return EditPlan{}, err
	}

	planner, provider, model, err := s.resolvePlanner(ctx)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	plannerInput := buildPlannerInput(product.Name, work.ScriptText, candidateSets)
	s.logger.Info("generation edit planner request",
		slog.String("generation_run_id", run.ID),
		slog.Int("visual_beat_count", len(plannerInput.Requirements)),
		slog.Int("candidate_count", plannerCandidateCount(plannerInput)),
	)
	result, err := planner.PlanEdits(ctx, plannerInput)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	clips, err := materializeEditPlan(result, candidateSets)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	basePlan.Status = "ready"
	basePlan.Clips = clips
	basePlan.LLMProvider = provider
	basePlan.LLMModel = model
	basePlan.ErrorMessage = ""
	if err := updatePlanArtifacts(&basePlan, visualBeats, candidateSets, clips); err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
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
	planner := modelgateway.NewEditPlanner(cfg)
	if openAICompatible, ok := planner.(*modelgateway.OpenAICompatibleEditPlanner); ok {
		openAICompatible.WithLogger(s.logger)
	}
	return planner, cfg.Provider, cfg.Model, nil
}

func (s *GenerationPlanningService) resolveVisualPlanner(ctx context.Context) (modelgateway.VisualPlanner, string, string, error) {
	if s.visualPlanner != nil {
		return s.visualPlanner, "test-provider", "test-model", nil
	}
	if err := EnsureLegacyOpenAICompatibleProvider(ctx, s.systemConfigService, s.modelProviderService); err != nil {
		return nil, "", "", err
	}
	cfg := ResolveLLMScriptConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	if strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, "", "", ErrLLMNotConfigured
	}
	planner := modelgateway.NewVisualPlanner(cfg)
	if openAICompatible, ok := planner.(*modelgateway.OpenAICompatibleEditPlanner); ok {
		openAICompatible.WithLogger(s.logger)
	}
	return planner, cfg.Provider, cfg.Model, nil
}

func (s *GenerationPlanningService) persistPlanFailure(ctx context.Context, plan EditPlan, cause error) error {
	plan.Status = "failed"
	plan.ErrorMessage = cause.Error()
	plan.Clips = nil
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

func buildVisualPlannerInput(productName string, work VoiceoverWork) modelgateway.VisualPlanInput {
	narrationSegments := make([]modelgateway.VisualPlanNarrationSegment, 0, len(work.NarrationSegments))
	for _, segment := range work.NarrationSegments {
		narrationSegments = append(narrationSegments, modelgateway.VisualPlanNarrationSegment{
			ID:      segment.ID,
			StartMs: segment.StartMs,
			EndMs:   segment.EndMs,
			Text:    segment.Text,
		})
	}
	narrativeBeats := make([]modelgateway.VisualPlanNarrativeBeat, 0, len(work.Beats))
	for _, beat := range work.Beats {
		narrativeBeats = append(narrativeBeats, modelgateway.VisualPlanNarrativeBeat{
			Label:        beat.Label,
			SellingPoint: beat.SellingPoint,
			VisualGoal:   beat.VisualGoal,
			SourceType:   modelgateway.TTSVisualSourceType,
		})
	}
	return modelgateway.VisualPlanInput{
		ProductName:       productName,
		ScriptText:        work.ScriptText,
		EditingIntent:     work.EditingIntent,
		NarrationSegments: narrationSegments,
		NarrativeBeats:    narrativeBeats,
	}
}

func materializeVisualBeats(result modelgateway.VisualPlanResult, input modelgateway.VisualPlanInput) ([]VisualBeat, error) {
	if err := modelgateway.ValidateVisualPlanResult(result, input); err != nil {
		return nil, err
	}
	beats := make([]VisualBeat, 0, len(result.VisualBeats))
	for _, beat := range result.VisualBeats {
		beats = append(beats, VisualBeat{
			ID:                 uuid.NewString(),
			NarrationSegmentID: beat.NarrationSegmentID,
			StartMs:            beat.StartMs,
			EndMs:              beat.EndMs,
			Label:              beat.Label,
			SellingPoint:       beat.SellingPoint,
			VisualGoal:         beat.VisualGoal,
			SourceType:         beat.SourceType,
		})
	}
	return beats, nil
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
			return fmt.Errorf("%w: visual beat %d has no eligible material for %q", ErrNoEligibleAssetCandidate, index+1, set.Requirement.VisualGoal)
		}
	}
	return nil
}

func buildPlannerInput(productName string, scriptText string, sets []CandidateSet) modelgateway.EditPlanInput {
	requirements := make([]modelgateway.EditPlanRequirement, 0, len(sets))
	for _, set := range sets {
		candidateCount := len(set.Candidates)
		if candidateCount > editPlannerCandidatesPerVisualBeat {
			candidateCount = editPlannerCandidatesPerVisualBeat
		}
		candidates := make([]modelgateway.EditPlanCandidate, 0, candidateCount)
		for _, candidate := range set.Candidates[:candidateCount] {
			candidates = append(candidates, modelgateway.EditPlanCandidate{
				ID:              candidate.ID,
				SourceType:      candidate.SourceType,
				SourceInMs:      candidate.SourceInMs,
				SourceOutMs:     candidate.SourceOutMs,
				SemanticSummary: truncatePlannerCandidateSummary(candidate.SemanticSummary),
				SemanticScore:   candidate.SemanticScore,
			})
		}
		requirements = append(requirements, modelgateway.EditPlanRequirement{
			VisualBeatID:       set.Requirement.VisualBeatID,
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

func plannerCandidateCount(input modelgateway.EditPlanInput) int {
	total := 0
	for _, requirement := range input.Requirements {
		total += len(requirement.Candidates)
	}
	return total
}

func truncatePlannerCandidateSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	runes := []rune(summary)
	if len(runes) <= maximumPlannerCandidateSemanticSummaryRunes {
		return summary
	}
	return string(runes[:maximumPlannerCandidateSemanticSummaryRunes-3]) + "..."
}

func materializeEditPlan(result modelgateway.EditPlanResult, sets []CandidateSet) ([]EditPlanClip, error) {
	input := buildPlannerInput("validated product", "validated script", sets)
	if err := modelgateway.ValidateEditPlanResult(result, input.Requirements); err != nil {
		return nil, err
	}
	candidatesByVisualBeat := map[string]map[string]AssetCandidate{}
	for _, set := range sets {
		candidateMap := map[string]AssetCandidate{}
		for _, candidate := range set.Candidates {
			candidateMap[candidate.ID] = candidate
		}
		candidatesByVisualBeat[set.Requirement.VisualBeatID] = candidateMap
	}
	choices := make(map[string]modelgateway.EditPlanClipChoice, len(result.Clips))
	for _, choice := range result.Clips {
		choices[choice.VisualBeatID] = choice
	}
	clips := make([]EditPlanClip, 0, len(sets))
	for _, set := range sets {
		requirement := set.Requirement
		choice, ok := choices[requirement.VisualBeatID]
		if !ok {
			return nil, fmt.Errorf("planner output is missing visual beat %q", requirement.VisualBeatID)
		}
		candidate, ok := candidatesByVisualBeat[requirement.VisualBeatID][choice.CandidateID]
		if !ok {
			return nil, fmt.Errorf("planner selected candidate %q outside the allowed set", choice.CandidateID)
		}
		if choice.SourceInMs < candidate.SourceInMs || choice.SourceOutMs > candidate.SourceOutMs || choice.SourceOutMs <= choice.SourceInMs {
			return nil, fmt.Errorf("planner source range is outside candidate %q", candidate.ID)
		}
		durationMs := requirement.EndMs - requirement.StartMs
		if choice.SourceOutMs-choice.SourceInMs != durationMs {
			return nil, fmt.Errorf("planner source range must match visual beat %q", requirement.VisualBeatID)
		}
		clips = append(clips, EditPlanClip{
			ID:                 "",
			VisualBeatID:       requirement.VisualBeatID,
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

func updatePlanArtifacts(plan *EditPlan, visualBeats []VisualBeat, candidateSets []CandidateSet, clips []EditPlanClip) error {
	if plan == nil {
		return fmt.Errorf("edit plan is required")
	}
	if visualBeats == nil {
		visualBeats = []VisualBeat{}
	}
	if candidateSets == nil {
		candidateSets = []CandidateSet{}
	}
	if clips == nil {
		clips = []EditPlanClip{}
	}
	payload := map[string]any{
		"visual_beats":   visualBeats,
		"candidate_sets": candidateSets,
		"clips":          clips,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	plan.CandidateSnapshot = append(json.RawMessage(nil), encoded...)
	plan.PlanJSON = append(json.RawMessage(nil), encoded...)
	return nil
}
