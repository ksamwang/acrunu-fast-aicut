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
		GenerationRunID:    run.ID,
		ScriptVariantID:    run.ScriptVariantID,
		VoiceoverID:        run.VoiceoverID,
		Status:             "planning",
		PromptVersion:      modelgateway.VisualPlanPromptVersion + "+" + modelgateway.EditPlanPromptVersion,
		SourceDurationMs:   work.DurationMs,
		TimelineDurationMs: work.DurationMs,
		NarrationSegments:  cloneNarrationSegments(work.NarrationSegments),
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
	visualBeats, narrationSegments, narrationPauses, timelineDurationMs, err := materializeVisualTimeline(visualResult, visualInput, work.NarrationSegments)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	basePlan.VisualBeats = visualBeats
	basePlan.NarrationSegments = narrationSegments
	basePlan.NarrationPauses = narrationPauses
	basePlan.TimelineDurationMs = timelineDurationMs
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
	requirements, err := BuildShotRequirements(visualBeats, narrationSegments)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	candidateSets, err := s.candidateService.Retrieve(ctx, run.ProductID, requirements, maxCandidatesPerNarrationSegment)
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
	plannerInput, err := buildPlannerInput(product.Name, work.ScriptText, candidateSets)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	if err := s.runs.UpdateStage(ctx, run.ID, generationRunStagePlanning, 84); err != nil {
		return EditPlan{}, err
	}

	planner, provider, model, err := s.resolvePlanner(ctx)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	s.logger.Info("generation edit planner request",
		slog.String("generation_run_id", run.ID),
		slog.Int("visual_beat_count", len(plannerInput.Requirements)),
		slog.Int("candidate_count", plannerCandidateCount(plannerInput)),
	)
	result, err := planner.PlanEdits(ctx, plannerInput)
	if err != nil {
		return EditPlan{}, s.persistPlanFailure(ctx, basePlan, err)
	}
	clips, err := materializeEditPlan(result, plannerInput)
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
	narrationSegments, safePauseBoundaries := visualPlanningNarrationSegments(work.NarrationSegments)
	narrativeBeats := make([]modelgateway.VisualPlanNarrativeBeat, 0, len(work.Beats))
	for index, beat := range work.Beats {
		beatID := strings.TrimSpace(beat.ID)
		if beatID == "" {
			beatID = fmt.Sprintf("narrative-beat-%d", index+1)
		}
		narrativeBeats = append(narrativeBeats, modelgateway.VisualPlanNarrativeBeat{
			ID:           beatID,
			Label:        beat.Label,
			SellingPoint: beat.SellingPoint,
			VisualGoal:   beat.VisualGoal,
			SourceType:   modelgateway.TTSVisualSourceType,
		})
	}
	return modelgateway.VisualPlanInput{
		ProductName:         productName,
		ScriptText:          work.ScriptText,
		EditingIntent:       work.EditingIntent,
		NarrationSegments:   narrationSegments,
		NarrativeBeats:      narrativeBeats,
		SafePauseBoundaries: safePauseBoundaries,
	}
}

func visualPlanningNarrationSegments(input []NarrationSegment) ([]modelgateway.VisualPlanNarrationSegment, []int) {
	if !hasCompleteSynthesisUnitTimeline(input) {
		segments := make([]modelgateway.VisualPlanNarrationSegment, 0, len(input))
		for _, segment := range input {
			segments = append(segments, modelgateway.VisualPlanNarrationSegment{
				ID: segment.ID, StartMs: segment.StartMs, EndMs: segment.EndMs, Text: segment.Text,
			})
		}
		return segments, nil
	}

	segments := make([]modelgateway.VisualPlanNarrationSegment, 0)
	safeBoundaries := make([]int, 0)
	for index := 0; index < len(input); {
		unitIndex := *input[index].SynthesisUnitIndex
		end := index + 1
		var text strings.Builder
		text.WriteString(input[index].Text)
		for end < len(input) && input[end].SynthesisUnitIndex != nil && *input[end].SynthesisUnitIndex == unitIndex {
			text.WriteString(input[end].Text)
			end++
		}
		segments = append(segments, modelgateway.VisualPlanNarrationSegment{
			ID:      input[index].ID,
			StartMs: input[index].StartMs,
			EndMs:   input[end-1].EndMs,
			Text:    text.String(),
		})
		safeBoundaries = append(safeBoundaries, input[end-1].EndMs)
		index = end
	}
	return segments, safeBoundaries
}

func hasCompleteSynthesisUnitTimeline(input []NarrationSegment) bool {
	if len(input) == 0 {
		return false
	}
	previousUnitIndex := -1
	previousEndMs := input[0].StartMs
	for _, segment := range input {
		if segment.SynthesisUnitIndex == nil || *segment.SynthesisUnitIndex < previousUnitIndex || *segment.SynthesisUnitIndex > previousUnitIndex+1 || segment.StartMs != previousEndMs {
			return false
		}
		if previousUnitIndex == -1 && *segment.SynthesisUnitIndex != 0 {
			return false
		}
		previousUnitIndex = *segment.SynthesisUnitIndex
		previousEndMs = segment.EndMs
	}
	return true
}

func materializeVisualTimeline(result modelgateway.VisualPlanResult, input modelgateway.VisualPlanInput, captions []NarrationSegment) ([]VisualBeat, []NarrationSegment, []NarrationPause, int, error) {
	if err := modelgateway.ValidateVisualPlanResult(result, input); err != nil {
		return nil, nil, nil, 0, err
	}
	if len(captions) == 0 {
		captions = make([]NarrationSegment, 0, len(input.NarrationSegments))
		for _, segment := range input.NarrationSegments {
			captions = append(captions, NarrationSegment{ID: segment.ID, StartMs: segment.StartMs, EndMs: segment.EndMs, Text: segment.Text})
		}
	}
	safePauseBoundaries := make(map[int]struct{}, len(input.SafePauseBoundaries))
	for _, boundary := range input.SafePauseBoundaries {
		safePauseBoundaries[boundary] = struct{}{}
	}
	pauses := make([]NarrationPause, 0, len(result.VisualBeats))
	beats := make([]VisualBeat, 0, len(result.VisualBeats))
	cumulativePauseMs := 0
	for _, beat := range result.VisualBeats {
		originalDurationMs := beat.EndMs - beat.StartMs
		pauseMs := 0
		_, safePauseBoundary := safePauseBoundaries[beat.EndMs]
		if safePauseBoundary {
			pauseMs = visualBeatTimelinePadding(beat.DurationClass, originalDurationMs)
		}
		durationClass := beat.DurationClass
		if !safePauseBoundary && !isVisualBeatDurationValid(durationClass, originalDurationMs) {
			durationClass = unpaddedVisualBeatDurationClass(originalDurationMs)
		}
		beats = append(beats, VisualBeat{
			ID:                 uuid.NewString(),
			NarrationSegmentID: beat.NarrationSegmentID,
			NarrativeBeatID:    beat.NarrativeBeatID,
			StartMs:            beat.StartMs + cumulativePauseMs,
			EndMs:              beat.EndMs + cumulativePauseMs + pauseMs,
			DurationClass:      durationClass,
			Label:              beat.Label,
			SellingPoint:       beat.SellingPoint,
			VisualGoal:         beat.VisualGoal,
			SourceType:         beat.SourceType,
		})
		if pauseMs > 0 {
			pauses = append(pauses, NarrationPause{AfterMs: beat.EndMs, DurationMs: pauseMs})
			cumulativePauseMs += pauseMs
		}
	}
	narrationSegments := shiftNarrationSegments(captions, pauses)
	return beats, narrationSegments, pauses, input.NarrationSegments[len(input.NarrationSegments)-1].EndMs + cumulativePauseMs, nil
}

func unpaddedVisualBeatDurationClass(durationMs int) string {
	switch {
	case durationMs >= 1800:
		return VisualBeatDurationStandard
	case durationMs >= 1000:
		return VisualBeatDurationBrief
	default:
		return VisualBeatDurationLegacy
	}
}

func visualBeatTimelinePadding(durationClass string, durationMs int) int {
	minimumMs, maximumMs, basePauseMs := 0, 0, 0
	switch durationClass {
	case modelgateway.VisualDurationClassBrief:
		minimumMs, maximumMs, basePauseMs = 1000, 1800, 150
	case modelgateway.VisualDurationClassStandard:
		minimumMs, maximumMs, basePauseMs = 1800, 4500, 250
	case modelgateway.VisualDurationClassAction:
		minimumMs, maximumMs, basePauseMs = modelgateway.MinimumActionEditPlanClipDurationMs, 6000, 0
	default:
		return 0
	}
	targetMs := durationMs + basePauseMs
	if targetMs < minimumMs {
		targetMs = minimumMs
	}
	if targetMs > maximumMs {
		targetMs = maximumMs
	}
	if durationClass == modelgateway.VisualDurationClassAction && targetMs > modelgateway.MaximumEditPlanClipDurationMs && targetMs < modelgateway.MinimumActionEditPlanClipDurationMs+modelgateway.MinimumEditPlanClipDurationMs {
		targetMs = modelgateway.MinimumActionEditPlanClipDurationMs + modelgateway.MinimumEditPlanClipDurationMs
	}
	if targetMs <= durationMs {
		return 0
	}
	return targetMs - durationMs
}

func shiftNarrationSegments(input []NarrationSegment, pauses []NarrationPause) []NarrationSegment {
	result := make([]NarrationSegment, 0, len(input))
	for _, segment := range input {
		startShiftMs := 0
		endShiftMs := 0
		for _, pause := range pauses {
			if pause.AfterMs <= segment.StartMs {
				startShiftMs += pause.DurationMs
			}
			if pause.AfterMs < segment.EndMs {
				endShiftMs += pause.DurationMs
			}
		}
		result = append(result, NarrationSegment{
			ID:                 segment.ID,
			StartMs:            segment.StartMs + startShiftMs,
			EndMs:              segment.EndMs + endShiftMs,
			Text:               segment.Text,
			Confidence:         segment.Confidence,
			SynthesisUnitIndex: segment.SynthesisUnitIndex,
		})
	}
	return result
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

func buildPlannerInput(productName string, scriptText string, sets []CandidateSet) (modelgateway.EditPlanInput, error) {
	requirements := make([]modelgateway.EditPlanRequirement, 0, len(sets))
	aliasesByAssetID := make(map[string]string)
	nextMaterialAlias := 1
	nextSlotID := 1
	for setIndex, set := range sets {
		slots, err := buildDeterministicEditPlanSlots(set.Requirement, &nextSlotID)
		if err != nil {
			return modelgateway.EditPlanInput{}, fmt.Errorf("visual beat %d: %w", setIndex+1, err)
		}
		for slotIndex := range slots {
			slot := &slots[slotIndex]
			for _, candidate := range set.Candidates {
				assetID := strings.TrimSpace(candidate.AssetID)
				if assetID == "" || candidate.SourceOutMs-candidate.SourceInMs < slot.DurationMs {
					continue
				}
				alias := aliasesByAssetID[assetID]
				if alias == "" {
					alias = fmt.Sprintf("m%03d", nextMaterialAlias)
					nextMaterialAlias++
					aliasesByAssetID[assetID] = alias
				}
				slot.Candidates = append(slot.Candidates, modelgateway.EditPlanCandidate{
					ID:               alias,
					SourceType:       candidate.SourceType,
					SourceInMs:       candidate.SourceInMs,
					SourceOutMs:      candidate.SourceOutMs,
					SemanticSummary:  truncatePlannerCandidateSummary(candidate.SemanticSummary),
					SemanticScore:    candidate.SemanticScore,
					AssetID:          assetID,
					UseOriginalAudio: candidate.DefaultUseOriginalAudio,
				})
			}
			if len(slot.Candidates) == 0 {
				return modelgateway.EditPlanInput{}, fmt.Errorf(
					"%w: visual beat %d slot %d requires a %dms material for %q",
					ErrNoEligibleAssetCandidate,
					setIndex+1,
					slotIndex+1,
					slot.DurationMs,
					set.Requirement.VisualGoal,
				)
			}
		}
		requirements = append(requirements, modelgateway.EditPlanRequirement{
			VisualBeatID:        set.Requirement.VisualBeatID,
			NarrationSegmentID:  set.Requirement.NarrationSegmentID,
			NarrationSegmentIDs: append([]string(nil), set.Requirement.NarrationSegmentIDs...),
			NarrativeBeatID:     set.Requirement.NarrativeBeatID,
			StartMs:             set.Requirement.StartMs,
			EndMs:               set.Requirement.EndMs,
			DurationClass:       set.Requirement.DurationClass,
			NarrationText:       set.Requirement.NarrationText,
			Label:               set.Requirement.Label,
			SellingPoint:        set.Requirement.SellingPoint,
			VisualGoal:          set.Requirement.VisualGoal,
			SourceType:          set.Requirement.SourceType,
			Slots:               slots,
		})
	}
	input := modelgateway.EditPlanInput{
		ProductName:  productName,
		ScriptText:   scriptText,
		Requirements: requirements,
	}
	assignedMaterials, err := assignUniquePlannerMaterials(input.Requirements)
	if err != nil {
		return modelgateway.EditPlanInput{}, err
	}
	trimPlannerSlotCandidates(input.Requirements, assignedMaterials)
	return input, nil
}

func buildDeterministicEditPlanSlots(requirement ShotRequirement, nextSlotID *int) ([]modelgateway.EditPlanSlot, error) {
	durationMs := requirement.EndMs - requirement.StartMs
	if durationMs < modelgateway.MinimumEditPlanClipDurationMs {
		return nil, fmt.Errorf("duration %dms is shorter than the minimum clip duration", durationMs)
	}
	durations := []int{}
	action := requirement.DurationClass == VisualBeatDurationAction
	if durationMs <= modelgateway.MaximumEditPlanClipDurationMs {
		durations = append(durations, durationMs)
	} else if action {
		primaryDurationMs := minInt(modelgateway.MaximumEditPlanClipDurationMs, durationMs-modelgateway.MinimumEditPlanClipDurationMs)
		if primaryDurationMs < modelgateway.MinimumActionEditPlanClipDurationMs {
			return nil, fmt.Errorf("action duration %dms cannot be split into legal clips", durationMs)
		}
		durations = append(durations, primaryDurationMs)
		remainingMs := durationMs - primaryDurationMs
		for remainingMs > 0 {
			nextDurationMs := minInt(modelgateway.MaximumEditPlanClipDurationMs, remainingMs)
			if remainingMs-nextDurationMs > 0 && remainingMs-nextDurationMs < modelgateway.MinimumEditPlanClipDurationMs {
				nextDurationMs -= modelgateway.MinimumEditPlanClipDurationMs - (remainingMs - nextDurationMs)
			}
			durations = append(durations, nextDurationMs)
			remainingMs -= nextDurationMs
		}
	} else {
		clipCount := (durationMs + modelgateway.MaximumEditPlanClipDurationMs - 1) / modelgateway.MaximumEditPlanClipDurationMs
		baseDurationMs := durationMs / clipCount
		remainderMs := durationMs % clipCount
		for index := 0; index < clipCount; index++ {
			slotDurationMs := baseDurationMs
			if index < remainderMs {
				slotDurationMs++
			}
			durations = append(durations, slotDurationMs)
		}
	}
	if len(durations) == 0 || len(durations) > modelgateway.MaximumEditPlanClipsPerBeat {
		return nil, fmt.Errorf("duration %dms requires an unsupported number of clips", durationMs)
	}

	slots := make([]modelgateway.EditPlanSlot, 0, len(durations))
	startMs := requirement.StartMs
	for index, slotDurationMs := range durations {
		if slotDurationMs < modelgateway.MinimumEditPlanClipDurationMs || slotDurationMs > modelgateway.MaximumEditPlanClipDurationMs {
			return nil, fmt.Errorf("duration %dms produces an invalid %dms clip", durationMs, slotDurationMs)
		}
		role := modelgateway.EditPlanSlotRoleSupport
		if index == 0 {
			role = modelgateway.EditPlanSlotRolePrimary
			if action {
				role = modelgateway.EditPlanSlotRoleActionPrimary
			}
		}
		slots = append(slots, modelgateway.EditPlanSlot{
			ID:         fmt.Sprintf("s%03d", *nextSlotID),
			StartMs:    startMs,
			EndMs:      startMs + slotDurationMs,
			DurationMs: slotDurationMs,
			Role:       role,
		})
		(*nextSlotID)++
		startMs += slotDurationMs
	}
	return slots, nil
}

func assignUniquePlannerMaterials(requirements []modelgateway.EditPlanRequirement) (map[string]string, error) {
	slots := make([]modelgateway.EditPlanSlot, 0)
	for _, requirement := range requirements {
		slots = append(slots, requirement.Slots...)
	}
	assignments := make(map[string]int, len(slots))
	var assignSlot func(int, map[string]bool) bool
	assignSlot = func(slotIndex int, visited map[string]bool) bool {
		for _, candidate := range slots[slotIndex].Candidates {
			if visited[candidate.ID] {
				continue
			}
			visited[candidate.ID] = true
			assignedSlot, assigned := assignments[candidate.ID]
			if !assigned || assignSlot(assignedSlot, visited) {
				assignments[candidate.ID] = slotIndex
				return true
			}
		}
		return false
	}
	for slotIndex, slot := range slots {
		if !assignSlot(slotIndex, map[string]bool{}) {
			return nil, fmt.Errorf("%w: slot %q has no globally unique material assignment", ErrNoEligibleAssetCandidate, slot.ID)
		}
	}
	result := make(map[string]string, len(slots))
	for candidateID, slotIndex := range assignments {
		result[slots[slotIndex].ID] = candidateID
	}
	return result, nil
}

func trimPlannerSlotCandidates(requirements []modelgateway.EditPlanRequirement, assignedMaterials map[string]string) {
	for requirementIndex := range requirements {
		for slotIndex := range requirements[requirementIndex].Slots {
			slot := &requirements[requirementIndex].Slots[slotIndex]
			if len(slot.Candidates) <= editPlannerCandidatesPerVisualBeat {
				continue
			}
			assignedID := assignedMaterials[slot.ID]
			assignedIndex := -1
			for candidateIndex, candidate := range slot.Candidates {
				if candidate.ID == assignedID {
					assignedIndex = candidateIndex
					break
				}
			}
			if assignedIndex >= 0 && assignedIndex >= editPlannerCandidatesPerVisualBeat {
				trimmed := append([]modelgateway.EditPlanCandidate(nil), slot.Candidates[:editPlannerCandidatesPerVisualBeat-1]...)
				trimmed = append(trimmed, slot.Candidates[assignedIndex])
				slot.Candidates = trimmed
				continue
			}
			slot.Candidates = append([]modelgateway.EditPlanCandidate(nil), slot.Candidates[:editPlannerCandidatesPerVisualBeat]...)
		}
	}
}

func plannerCandidateCount(input modelgateway.EditPlanInput) int {
	total := 0
	for _, requirement := range input.Requirements {
		for _, slot := range requirement.Slots {
			total += len(slot.Candidates)
		}
	}
	return total
}

func truncatePlannerCandidateSummary(summary string) string {
	return prioritizedSemanticSummary(summary, maximumPlannerCandidateSemanticSummaryRunes)
}

func materializeEditPlan(result modelgateway.EditPlanResult, input modelgateway.EditPlanInput) ([]EditPlanClip, error) {
	if err := modelgateway.ValidateEditPlanResult(result, input.Requirements); err != nil {
		return nil, err
	}
	type slotContext struct {
		requirement modelgateway.EditPlanRequirement
		slot        modelgateway.EditPlanSlot
	}
	contexts := make(map[string]slotContext)
	for _, requirement := range input.Requirements {
		for _, slot := range requirement.Slots {
			contexts[slot.ID] = slotContext{requirement: requirement, slot: slot}
		}
	}
	clips := make([]EditPlanClip, 0, len(result.Clips))
	usedAssetIDs := make(map[string]int, len(result.Clips))
	for index, choice := range result.Clips {
		context, ok := contexts[choice.SlotID]
		if !ok {
			return nil, fmt.Errorf("planner output references unknown slot %q", choice.SlotID)
		}
		candidate, ok := findPlannerSlotCandidate(context.slot.Candidates, choice.CandidateID)
		if !ok {
			return nil, fmt.Errorf("planner selected material %q outside slot %q", choice.CandidateID, choice.SlotID)
		}
		assetID := strings.TrimSpace(candidate.AssetID)
		if assetID == "" {
			return nil, fmt.Errorf("planner candidate %q has no source asset", candidate.ID)
		}
		if previousIndex, exists := usedAssetIDs[assetID]; exists {
			return nil, fmt.Errorf("planner clip %d reuses asset %q already selected by clip %d", index+1, assetID, previousIndex+1)
		}
		usedAssetIDs[assetID] = index
		sourceInMs := candidate.SourceInMs
		sourceOutMs := sourceInMs + context.slot.DurationMs
		clips = append(clips, EditPlanClip{
			ID:                 "",
			VisualBeatID:       context.requirement.VisualBeatID,
			NarrationSegmentID: context.requirement.NarrationSegmentID,
			AssetID:            assetID,
			SourceInMs:         sourceInMs,
			SourceOutMs:        sourceOutMs,
			StartMs:            context.slot.StartMs,
			EndMs:              context.slot.EndMs,
			TimelineDurationMs: context.slot.DurationMs,
			Label:              strings.TrimSpace(context.requirement.Label),
			VisualGoal:         strings.TrimSpace(context.requirement.VisualGoal),
			SourceType:         candidate.SourceType,
			UseOriginalAudio:   candidate.UseOriginalAudio,
			AudioGainDB:        0,
		})
	}
	return clips, nil
}

func findPlannerSlotCandidate(candidates []modelgateway.EditPlanCandidate, candidateID string) (modelgateway.EditPlanCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.ID == strings.TrimSpace(candidateID) {
			return candidate, true
		}
	}
	return modelgateway.EditPlanCandidate{}, false
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
		"source_duration_ms":   plan.SourceDurationMs,
		"timeline_duration_ms": plan.TimelineDurationMs,
		"narration_segments":   plan.NarrationSegments,
		"narration_pauses":     plan.NarrationPauses,
		"visual_beats":         visualBeats,
		"candidate_sets":       candidateSets,
		"clips":                clips,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	plan.CandidateSnapshot = append(json.RawMessage(nil), encoded...)
	plan.PlanJSON = append(json.RawMessage(nil), encoded...)
	return nil
}
