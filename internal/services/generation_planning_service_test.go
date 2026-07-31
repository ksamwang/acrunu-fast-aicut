package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type planningCandidateStore struct {
	call   int
	inputs []CandidateSearchInput
}

func (s *planningCandidateStore) SearchCandidates(_ context.Context, input CandidateSearchInput) ([]AssetCandidate, error) {
	s.call++
	s.inputs = append(s.inputs, input)
	assetIndex := 1
	if input.MinimumDurationMs >= modelgateway.MinimumActionEditPlanClipDurationMs {
		assetIndex = 2
	}
	return []AssetCandidate{{
		ID:                      fmt.Sprintf("candidate-%d", assetIndex),
		AssetID:                 fmt.Sprintf("asset-%d", assetIndex),
		ObjectType:              "shot",
		SourceType:              "visual_only",
		SourceInMs:              0,
		SourceOutMs:             10_000,
		AssetDurationMs:         10_000,
		DefaultUseOriginalAudio: assetIndex == 2,
		SemanticScore:           0.9,
	}}, nil
}

type emptyPlanningCandidateStore struct{}

func (emptyPlanningCandidateStore) SearchCandidates(_ context.Context, _ CandidateSearchInput) ([]AssetCandidate, error) {
	return nil, nil
}

type deterministicEditPlanner struct {
	invalidCandidate bool
}

func (p deterministicEditPlanner) PlanEdits(_ context.Context, input modelgateway.EditPlanInput) (modelgateway.EditPlanResult, error) {
	result := modelgateway.EditPlanResult{}
	for _, requirement := range input.Requirements {
		for _, slot := range requirement.Slots {
			candidateID := slot.Candidates[0].ID
			if p.invalidCandidate {
				candidateID = "not-allowed"
			}
			result.Clips = append(result.Clips, modelgateway.EditPlanClipChoice{
				SlotID:      slot.ID,
				CandidateID: candidateID,
			})
		}
	}
	return result, nil
}

type deterministicVisualPlanner struct {
	beats []modelgateway.VisualPlanBeat
}

func (p deterministicVisualPlanner) PlanVisuals(_ context.Context, input modelgateway.VisualPlanInput) (modelgateway.VisualPlanResult, error) {
	if len(p.beats) > 0 {
		return modelgateway.VisualPlanResult{VisualBeats: append([]modelgateway.VisualPlanBeat(nil), p.beats...)}, nil
	}
	beats := make([]modelgateway.VisualPlanBeat, 0, len(input.NarrationSegments))
	for _, segment := range input.NarrationSegments {
		durationClass := modelgateway.VisualDurationClassStandard
		if segment.EndMs-segment.StartMs < 1800 {
			durationClass = modelgateway.VisualDurationClassBrief
		}
		beats = append(beats, modelgateway.VisualPlanBeat{
			NarrationSegmentID: segment.ID,
			StartMs:            segment.StartMs,
			EndMs:              segment.EndMs,
			DurationClass:      durationClass,
			Label:              "画面展示",
			VisualGoal:         "展示产品使用动作。",
			SourceType:         "visual_only",
		})
	}
	return modelgateway.VisualPlanResult{VisualBeats: beats}, nil
}

func TestBuildVisualPlannerInputForcesVisualOnlyTTSMaterial(t *testing.T) {
	input := buildVisualPlannerInput("束裤带", VoiceoverWork{
		ScriptText: "固定裤脚。",
		NarrationSegments: []NarrationSegment{{
			ID: "n-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
		Beats: []VoiceoverBeat{
			{ID: "business-one", Label: "口播", SellingPoint: "卖点一", VisualGoal: "人物口播", SourceType: "talking_head"},
			{Label: "混剪", SellingPoint: "卖点二", VisualGoal: "人物与产品", SourceType: "mixed"},
		},
	})
	for _, beat := range input.NarrativeBeats {
		if beat.SourceType != modelgateway.TTSVisualSourceType {
			t.Fatalf("expected TTS visual planner input to be visual-only, got %#v", input.NarrativeBeats)
		}
	}
	if input.NarrativeBeats[0].ID != "business-one" || input.NarrativeBeats[1].ID != "narrative-beat-2" {
		t.Fatalf("expected stable and fallback narrative beat ids, got %#v", input.NarrativeBeats)
	}
}

func TestMaterializeVisualTimelinePadsEachSemanticBeforeNextNarration(t *testing.T) {
	input := modelgateway.VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "一秒快拆，大面积魔术贴，收纳方便。高弹松紧带，拉伸自如，贴合脚踝。",
		NarrationSegments: []modelgateway.VisualPlanNarrationSegment{
			{ID: "quick", StartMs: 0, EndMs: 1015, Text: "一秒快拆，"},
			{ID: "velcro", StartMs: 1015, EndMs: 2284, Text: "大面积魔术贴，"},
			{ID: "storage", StartMs: 2284, EndMs: 3250, Text: "收纳方便。"},
			{ID: "elastic", StartMs: 3250, EndMs: 4458, Text: "高弹松紧带，"},
			{ID: "stretch", StartMs: 4458, EndMs: 5430, Text: "拉伸自如，"},
			{ID: "fit", StartMs: 5430, EndMs: 6450, Text: "贴合脚踝。"},
		},
		SafePauseBoundaries: []int{1015, 2284, 3250, 4458, 5430, 6450},
	}
	result := modelgateway.VisualPlanResult{VisualBeats: []modelgateway.VisualPlanBeat{
		{NarrationSegmentID: "quick", StartMs: 0, EndMs: 1015, DurationClass: modelgateway.VisualDurationClassBrief, Label: "快拆", VisualGoal: "展示一秒快拆", SourceType: "visual_only"},
		{NarrationSegmentID: "velcro", StartMs: 1015, EndMs: 2284, DurationClass: modelgateway.VisualDurationClassStandard, Label: "魔术贴", VisualGoal: "展示大面积魔术贴", SourceType: "visual_only"},
		{NarrationSegmentID: "storage", StartMs: 2284, EndMs: 3250, DurationClass: modelgateway.VisualDurationClassAction, Label: "收纳", VisualGoal: "完整展示放入口袋", SourceType: "visual_only"},
		{NarrationSegmentID: "elastic", StartMs: 3250, EndMs: 5430, DurationClass: modelgateway.VisualDurationClassAction, Label: "拉伸", VisualGoal: "完整展示反复拉伸", SourceType: "visual_only"},
		{NarrationSegmentID: "fit", StartMs: 5430, EndMs: 6450, DurationClass: modelgateway.VisualDurationClassStandard, Label: "贴合", VisualGoal: "展示贴合脚踝", SourceType: "visual_only"},
	}}

	beats, segments, pauses, durationMs, err := materializeVisualTimeline(result, input, nil)
	if err != nil {
		t.Fatalf("materialize timeline: %v", err)
	}
	if beats[1].StartMs != 1165 || beats[1].EndMs != 2965 {
		t.Fatalf("expected velcro visual to complete before storage, got %#v", beats[1])
	}
	if beats[2].StartMs != 2965 || beats[2].EndMs != 5765 {
		t.Fatalf("expected storage visual to receive a full action window, got %#v", beats[2])
	}
	if segments[2].StartMs != 2965 || segments[2].EndMs != 3931 || segments[3].StartMs != 5765 {
		t.Fatalf("expected high-elasticity narration to wait for storage, got %#v", segments)
	}
	if len(pauses) != 5 || durationMs <= input.NarrationSegments[len(input.NarrationSegments)-1].EndMs {
		t.Fatalf("expected persisted editorial pauses, pauses=%#v duration=%d", pauses, durationMs)
	}
}

func TestGenerationPlanningServicePersistsMultiClipNarrationPlan(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	firstSynthesisUnit, secondSynthesisUnit := 0, 1
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID:            "voiceover-task-1",
		ProductID:     product.ID,
		ProductName:   product.Name,
		ScriptText:    "骑行时裤脚不再蹭链条。固定后更安心。",
		DurationMs:    3500,
		EditingIntent: "先展示痛点，再展示固定后的结果。",
		Beats: []VoiceoverBeat{
			{ID: "business-pain", Label: "痛点", SellingPoint: "避免蹭链条", VisualGoal: "展示裤脚靠近链条。", SourceType: "visual_only"},
			{ID: "business-result", Label: "结果", SellingPoint: "固定更稳", VisualGoal: "展示固定后的骑行状态。", SourceType: "visual_only"},
		},
		NarrationSegments: []NarrationSegment{
			{ID: "narration-1", StartMs: 0, EndMs: 2000, Text: "骑行时裤脚不再蹭链条。", SynthesisUnitIndex: &firstSynthesisUnit},
			{ID: "narration-2", StartMs: 2000, EndMs: 3500, Text: "固定后更安心。", SynthesisUnitIndex: &secondSynthesisUnit},
		},
	}}
	runs := NewGenerationRunService(loader)
	run, err := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: product.ID, CreatedByUserID: "user-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := runs.LinkTask(context.Background(), run.ID, loader.work.ID, generationRunTaskStageVoiceover); err != nil {
		t.Fatalf("link task: %v", err)
	}
	if err := runs.AttachVoiceoverArtifacts(context.Background(), run.ID, loader.work.ID, "script-1", "voiceover-1"); err != nil {
		t.Fatalf("attach artifacts: %v", err)
	}
	store := &planningCandidateStore{}
	candidates := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(&candidateTestEmbedder{}).
		WithStore(store)
	planning := NewGenerationPlanningService(
		runs,
		loader,
		assets,
		candidates,
		NewSystemConfigService(),
		NewModelProviderService(),
		config.Config{},
	).WithVisualPlanner(deterministicVisualPlanner{beats: []modelgateway.VisualPlanBeat{
		{NarrationSegmentID: "narration-1", NarrativeBeatID: "business-pain", StartMs: 0, EndMs: 2000, DurationClass: modelgateway.VisualDurationClassStandard, Label: "痛点", VisualGoal: "裤脚靠近链条", SourceType: "visual_only"},
		{NarrationSegmentID: "narration-2", NarrativeBeatID: "business-result", StartMs: 2000, EndMs: 3500, DurationClass: modelgateway.VisualDurationClassAction, Label: "固定结果", VisualGoal: "完整展示束裤带固定裤脚并开始骑行", SourceType: "visual_only"},
	}}).WithPlanner(deterministicEditPlanner{})

	plan, err := planning.Generate(context.Background(), GenerateEditPlanInput{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
	})
	if err != nil {
		t.Fatalf("generate plan: %v", err)
	}
	if plan.Status != "ready" || len(plan.VisualBeats) != 2 || len(plan.Clips) != 2 {
		t.Fatalf("unexpected plan %#v", plan)
	}
	if plan.VisualBeats[1].DurationClass != VisualBeatDurationAction {
		t.Fatalf("expected visual duration class to be persisted, got %#v", plan.VisualBeats)
	}
	if plan.VisualBeats[0].NarrativeBeatID != "business-pain" || plan.VisualBeats[1].NarrativeBeatID != "business-result" {
		t.Fatalf("expected business intention references to be persisted, got %#v", plan.VisualBeats)
	}
	if plan.Clips[0].NarrationSegmentID == plan.Clips[1].NarrationSegmentID || plan.Clips[0].VisualBeatID == plan.Clips[1].VisualBeatID {
		t.Fatalf("expected each visual beat to stay on complete narration semantics, got %#v", plan.Clips)
	}
	if plan.SourceDurationMs != 3500 || plan.TimelineDurationMs != 5050 || len(plan.NarrationPauses) != 2 {
		t.Fatalf("expected service-side editorial pauses, got %#v", plan)
	}
	if plan.NarrationSegments[1].StartMs != 2250 || plan.NarrationSegments[1].EndMs != 3750 {
		t.Fatalf("expected later narration to be shifted after the first visual, got %#v", plan.NarrationSegments)
	}
	if plan.Clips[0].AssetID != "asset-1" || plan.Clips[1].AssetID != "asset-2" || !plan.Clips[1].UseOriginalAudio {
		t.Fatalf("candidate mapping or audio policy was not retained %#v", plan.Clips)
	}
	if len(store.inputs) != 4 {
		t.Fatalf("expected duration-aware candidate queries, got %d", len(store.inputs))
	}
	for _, candidateInput := range store.inputs {
		if candidateInput.Limit != planningCandidateRetrievalPoolSize {
			t.Fatalf("expected retrieval pool limit %d, got %#v", planningCandidateRetrievalPoolSize, candidateInput)
		}
	}
	var artifacts struct {
		SourceDurationMs   int                `json:"source_duration_ms"`
		TimelineDurationMs int                `json:"timeline_duration_ms"`
		NarrationSegments  []NarrationSegment `json:"narration_segments"`
		NarrationPauses    []NarrationPause   `json:"narration_pauses"`
		VisualBeats        []VisualBeat       `json:"visual_beats"`
		CandidateSets      []CandidateSet     `json:"candidate_sets"`
		Clips              []EditPlanClip     `json:"clips"`
	}
	if err := json.Unmarshal(plan.PlanJSON, &artifacts); err != nil {
		t.Fatalf("decode plan artifacts: %v", err)
	}
	if len(artifacts.VisualBeats) != 2 || len(artifacts.CandidateSets) != 2 || len(artifacts.Clips) != 2 {
		t.Fatalf("plan artifact snapshot is incomplete %#v", artifacts)
	}
	if artifacts.SourceDurationMs != 3500 || artifacts.TimelineDurationMs != 5050 || len(artifacts.NarrationSegments) != 2 || len(artifacts.NarrationPauses) != 2 {
		t.Fatalf("plan artifact narration timeline is incomplete %#v", artifacts)
	}
	updatedRun, err := runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updatedRun.Status != generationRunStatusGenerating || updatedRun.Stage != generationRunStagePlanReady {
		t.Fatalf("expected generating plan-ready run, got %#v", updatedRun)
	}
}

func TestGenerationPlanningServicePersistsEmptyVisualBeatCandidateSnapshot(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID:          "voiceover-task-1",
		ProductID:   product.ID,
		ProductName: product.Name,
		ScriptText:  "固定裤脚。",
		DurationMs:  1000,
		NarrationSegments: []NarrationSegment{{
			ID: "narration-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
	}}
	runs := NewGenerationRunService(loader)
	run, _ := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: product.ID})
	_ = runs.LinkTask(context.Background(), run.ID, loader.work.ID, generationRunTaskStageVoiceover)
	_ = runs.AttachVoiceoverArtifacts(context.Background(), run.ID, loader.work.ID, "script-1", "voiceover-1")
	candidates := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(&candidateTestEmbedder{}).
		WithStore(emptyPlanningCandidateStore{})
	planning := NewGenerationPlanningService(runs, loader, assets, candidates, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithVisualPlanner(deterministicVisualPlanner{})

	_, err := planning.Generate(context.Background(), GenerateEditPlanInput{GenerationRunID: run.ID, ScriptVariantID: "script-1", VoiceoverID: "voiceover-1"})
	if err == nil || !strings.Contains(err.Error(), "visual beat 1 has no eligible material") {
		t.Fatalf("expected visual beat candidate failure, got %v", err)
	}
	stored, err := runs.GetEditPlan(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("expected failed plan to be stored: %v", err)
	}
	if stored.Status != "failed" || len(stored.VisualBeats) != 1 {
		t.Fatalf("expected failed visual plan snapshot, got %#v", stored)
	}
	var artifacts struct {
		CandidateSets []CandidateSet `json:"candidate_sets"`
	}
	if err := json.Unmarshal(stored.CandidateSnapshot, &artifacts); err != nil {
		t.Fatalf("decode candidate snapshot: %v", err)
	}
	if len(artifacts.CandidateSets) != 1 || len(artifacts.CandidateSets[0].Candidates) != 0 {
		t.Fatalf("expected empty candidate set to be persisted %#v", artifacts)
	}
}

func TestGenerationPlanningServiceRejectsPlannerCandidateOutsideClosedSet(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID:          "voiceover-task-1",
		ProductID:   product.ID,
		ProductName: product.Name,
		ScriptText:  "固定裤脚。",
		DurationMs:  1000,
		NarrationSegments: []NarrationSegment{{
			ID: "narration-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
	}}
	runs := NewGenerationRunService(loader)
	run, _ := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: product.ID})
	_ = runs.LinkTask(context.Background(), run.ID, loader.work.ID, generationRunTaskStageVoiceover)
	_ = runs.AttachVoiceoverArtifacts(context.Background(), run.ID, loader.work.ID, "script-1", "voiceover-1")
	candidates := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(&candidateTestEmbedder{}).
		WithStore(&planningCandidateStore{})
	planning := NewGenerationPlanningService(runs, loader, assets, candidates, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithVisualPlanner(deterministicVisualPlanner{}).
		WithPlanner(deterministicEditPlanner{invalidCandidate: true})

	_, err := planning.Generate(context.Background(), GenerateEditPlanInput{GenerationRunID: run.ID, ScriptVariantID: "script-1", VoiceoverID: "voiceover-1"})
	if err == nil || !strings.Contains(err.Error(), "outside its allowed set") {
		t.Fatalf("expected closed candidate set validation error, got %v", err)
	}
	stored, err := runs.GetEditPlan(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("expected failed plan to be stored: %v", err)
	}
	if stored.Status != "failed" {
		t.Fatalf("expected failed persisted plan, got %#v", stored)
	}
}

func TestBuildPlannerInputIncludesCandidateSemanticEvidence(t *testing.T) {
	input, err := buildPlannerInput("束裤带", "魔术贴一粘即合。", []CandidateSet{{
		Requirement: ShotRequirement{
			VisualBeatID:       "visual-1",
			NarrationSegmentID: "narration-1",
			StartMs:            0,
			EndMs:              1200,
			NarrationText:      "魔术贴一粘即合。",
			SellingPoint:       "魔术贴",
			VisualGoal:         "展示快速粘合动作。",
			SourceType:         "visual_only",
		},
		Candidates: []AssetCandidate{{
			ID:              "candidate-1",
			AssetID:         "asset-1",
			SourceType:      "visual_only",
			SourceInMs:      100,
			SourceOutMs:     1600,
			SemanticSummary: "画面描述：手部将束裤带魔术贴快速粘合。",
			SemanticScore:   0.92,
			BatchUseCount:   2,
		}},
	}})
	if err != nil {
		t.Fatalf("build planner input: %v", err)
	}

	candidate := input.Requirements[0].Slots[0].Candidates[0]
	if input.Requirements[0].VisualBeatID != "visual-1" || candidate.SemanticSummary != "画面描述：手部将束裤带魔术贴快速粘合。" || candidate.SemanticScore != 0.92 || candidate.BatchUseCount != 2 {
		t.Fatalf("candidate semantic evidence was not preserved %#v", candidate)
	}
}

func TestBuildPlannerInputBoundsCandidatesAndSemanticSummary(t *testing.T) {
	candidates := make([]AssetCandidate, 0, editPlannerCandidatesPerVisualBeat+1)
	for index := 0; index < editPlannerCandidatesPerVisualBeat+1; index++ {
		candidates = append(candidates, AssetCandidate{
			ID:              fmt.Sprintf("candidate-%d", index+1),
			AssetID:         fmt.Sprintf("asset-%d", index+1),
			SourceType:      "visual_only",
			SourceInMs:      0,
			SourceOutMs:     2000,
			SemanticSummary: strings.Repeat("镜", maximumPlannerCandidateSemanticSummaryRunes+20),
			SemanticScore:   0.80,
		})
	}
	input, err := buildPlannerInput("束裤带", "固定裤脚。", []CandidateSet{{
		Requirement: ShotRequirement{
			VisualBeatID:       "visual-1",
			NarrationSegmentID: "narration-1",
			StartMs:            0,
			EndMs:              1000,
			NarrationText:      "固定裤脚。",
			VisualGoal:         "展示固定动作",
			SourceType:         "visual_only",
		},
		Candidates: candidates,
	}})
	if err != nil {
		t.Fatalf("build planner input: %v", err)
	}
	got := input.Requirements[0].Slots[0].Candidates
	if len(got) != editPlannerCandidatesPerVisualBeat || got[len(got)-1].ID != "m006" {
		t.Fatalf("expected first %d candidates, got %#v", editPlannerCandidatesPerVisualBeat, got)
	}
	if len([]rune(got[0].SemanticSummary)) != maximumPlannerCandidateSemanticSummaryRunes {
		t.Fatalf("expected %d-rune summary, got %q", maximumPlannerCandidateSemanticSummaryRunes, got[0].SemanticSummary)
	}
}

func TestSelectPlannerAssetCandidatesAppliesDurationBeforeAbsoluteThreshold(t *testing.T) {
	candidates := []AssetCandidate{
		{AssetID: "too-short", SourceOutMs: 799, SemanticScore: 0.95},
		{AssetID: "top", SourceOutMs: 1000, SemanticScore: 0.80},
		{AssetID: "boundary", SourceOutMs: 1000, SemanticScore: 0.76},
		{AssetID: "outside", SourceOutMs: 1000, SemanticScore: 0.759},
	}
	selected := selectPlannerAssetCandidates(candidates, 800, 1)
	if len(selected) != 3 || selected[0].AssetID != "top" || selected[1].AssetID != "boundary" || selected[2].AssetID != "outside" {
		t.Fatalf("expected preferred candidates before the below-threshold fallback, got %#v", selected)
	}
}

func TestSelectPlannerAssetCandidatesKeepsRankedFallbackBelowThreshold(t *testing.T) {
	candidates := []AssetCandidate{
		{AssetID: "lower", SourceOutMs: 1000, SemanticScore: 0.72, BatchUseCount: 0},
		{AssetID: "top-b", SourceOutMs: 1000, SemanticScore: 0.74, BatchUseCount: 1},
		{AssetID: "top-a", SourceOutMs: 1000, SemanticScore: 0.74, BatchUseCount: 1},
	}
	selected := selectPlannerAssetCandidates(candidates, 800, 1)
	if len(selected) != 3 || selected[0].AssetID != "top-a" || selected[1].AssetID != "top-b" || selected[2].AssetID != "lower" {
		t.Fatalf("expected semantic-score-ordered fallback candidates, got %#v", selected)
	}
}

func TestSelectPlannerAssetCandidatesUsesBatchCountAndStableVariantRotation(t *testing.T) {
	candidates := []AssetCandidate{
		{AssetID: "used-top", SourceOutMs: 1000, SemanticScore: 0.80, BatchUseCount: 1},
		{AssetID: "fresh-high", SourceOutMs: 1000, SemanticScore: 0.79, BatchUseCount: 0},
		{AssetID: "fresh-low", SourceOutMs: 1000, SemanticScore: 0.78, BatchUseCount: 0},
	}
	first := selectPlannerAssetCandidates(candidates, 800, 1)
	second := selectPlannerAssetCandidates(candidates, 800, 2)
	if len(first) != 3 || first[0].AssetID != "fresh-high" || first[1].AssetID != "fresh-low" || first[2].AssetID != "used-top" {
		t.Fatalf("unexpected batch usage ordering %#v", first)
	}
	if len(second) != 3 || second[0].AssetID != "fresh-low" || second[1].AssetID != "fresh-high" || second[2].AssetID != "used-top" {
		t.Fatalf("unexpected stable variant rotation %#v", second)
	}
}

func TestPlanningCandidateMinimumDurationsIncludesTransitionThresholds(t *testing.T) {
	thresholds, err := planningCandidateMinimumDurations([]ShotRequirement{
		{VisualBeatID: "action", StartMs: 0, EndMs: 2800, DurationClass: VisualBeatDurationAction},
		{VisualBeatID: "result", StartMs: 2800, EndMs: 4800, DurationClass: VisualBeatDurationStandard},
	})
	if err != nil {
		t.Fatalf("build duration thresholds: %v", err)
	}
	if fmt.Sprint(thresholds["action"]) != "[2500 2800]" || fmt.Sprint(thresholds["result"]) != "[2000 2300]" {
		t.Fatalf("unexpected duration thresholds %#v", thresholds)
	}
}

func TestMaterializeEditPlanAbsorbsShortMaterialAtNextVisualBeat(t *testing.T) {
	sets := earlyTransitionCandidateSets(2300)
	input, err := buildPlannerInputWithOptions("束裤带", "展示动作和结果。", sets, plannerBuildOptions{VariantIndex: 1, AllowEarlyTransitions: true})
	if err != nil {
		t.Fatalf("build planner input: %v", err)
	}
	outgoing := input.Requirements[0].Slots[0]
	incoming := input.Requirements[1].Slots[0]
	if outgoing.MaximumEarlyEndMs != 100 || incoming.MaximumLeadingExtensionMs != 100 {
		t.Fatalf("expected paired 100ms transition allowance, got %#v %#v", outgoing, incoming)
	}
	shortAlias := plannerCandidateAliasForAsset(outgoing.Candidates, "asset-short")
	incomingAlias := plannerCandidateAliasForAsset(incoming.Candidates, "asset-next")
	clips, err := materializeEditPlan(modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{
		{SlotID: outgoing.ID, CandidateID: shortAlias},
		{SlotID: incoming.ID, CandidateID: incomingAlias},
	}}, input)
	if err != nil {
		t.Fatalf("materialize early transition: %v", err)
	}
	if len(clips) != 2 || clips[0].StartMs != 0 || clips[0].EndMs != 2700 || clips[0].TimelineDurationMs != 2700 ||
		clips[1].StartMs != 2700 || clips[1].EndMs != 4800 || clips[1].TimelineDurationMs != 2100 || clips[1].SourceOutMs-clips[1].SourceInMs != 2100 {
		t.Fatalf("unexpected early-transition clips %#v", clips)
	}
}

func TestBuildPlannerInputAllowsShortOnlyMaterialWithAbsorber(t *testing.T) {
	sets := earlyTransitionCandidateSets(2300)
	sets[0].Candidates = sets[0].Candidates[1:]
	input, err := buildPlannerInputWithOptions("束裤带", "展示动作和结果。", sets, plannerBuildOptions{VariantIndex: 1, AllowEarlyTransitions: true})
	if err != nil {
		t.Fatalf("expected safe early transition without an exact outgoing candidate: %v", err)
	}
	if input.Requirements[0].Slots[0].MaximumEarlyEndMs != 100 || input.Requirements[1].Slots[0].MaximumLeadingExtensionMs != 100 {
		t.Fatalf("expected paired 100ms transition allowance, got %#v", input.Requirements)
	}
}

func TestBuildPlannerInputFallsBackToExactDurationWithoutAbsorber(t *testing.T) {
	input, err := buildPlannerInputWithOptions("束裤带", "展示动作和结果。", earlyTransitionCandidateSets(2000), plannerBuildOptions{
		VariantIndex:          1,
		AllowEarlyTransitions: true,
	})
	if err != nil {
		t.Fatalf("build strict fallback: %v", err)
	}
	outgoing := input.Requirements[0].Slots[0]
	incoming := input.Requirements[1].Slots[0]
	if outgoing.MaximumEarlyEndMs != 0 || incoming.MaximumLeadingExtensionMs != 0 {
		t.Fatalf("expected exact-duration fallback, got %#v %#v", outgoing, incoming)
	}
	if plannerCandidateAliasForAsset(outgoing.Candidates, "asset-short") != "" || plannerCandidateAliasForAsset(outgoing.Candidates, "asset-exact") == "" {
		t.Fatalf("expected only complete outgoing material, got %#v", outgoing.Candidates)
	}
}

func TestBuildPlannerInputUsesFallbackToPreserveUniqueAssignmentWithTolerance(t *testing.T) {
	sets := []CandidateSet{
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-action", NarrationSegmentID: "narration-action", StartMs: 0, EndMs: 2800, DurationClass: VisualBeatDurationAction, NarrationText: "完整动作。", VisualGoal: "完整展示动作", SourceType: "visual_only"},
			Candidates: []AssetCandidate{
				{AssetID: "asset-shared", SourceType: "visual_only", SourceOutMs: 2700, SemanticScore: 0.90},
				{AssetID: "asset-exact", SourceType: "visual_only", SourceOutMs: 2800, SemanticScore: 0.70},
			},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-result", NarrationSegmentID: "narration-result", StartMs: 2800, EndMs: 4800, DurationClass: VisualBeatDurationStandard, NarrationText: "展示结果。", VisualGoal: "展示结果", SourceType: "visual_only"},
			Candidates: []AssetCandidate{
				{AssetID: "asset-shared", SourceType: "visual_only", SourceOutMs: 2700, SemanticScore: 0.90},
				{AssetID: "asset-result", SourceType: "visual_only", SourceOutMs: 2000, SemanticScore: 0.90},
			},
		},
	}
	input, err := buildPlannerInputWithOptions("束裤带", "展示动作和结果。", sets, plannerBuildOptions{VariantIndex: 1, AllowEarlyTransitions: true})
	if err != nil {
		t.Fatalf("expected strict unique-assignment fallback: %v", err)
	}
	if input.Requirements[0].Slots[0].MaximumEarlyEndMs != 100 || input.Requirements[1].Slots[0].MaximumLeadingExtensionMs != 100 {
		t.Fatalf("expected the fallback assignment to preserve the safe transition allowance, got %#v", input.Requirements)
	}
	if plannerCandidateAliasForAsset(input.Requirements[0].Slots[0].Candidates, "asset-exact") == "" ||
		plannerCandidateAliasForAsset(input.Requirements[0].Slots[0].Candidates, "asset-shared") != "" {
		t.Fatalf("expected exact outgoing material after fallback, got %#v", input.Requirements[0].Slots[0].Candidates)
	}
}

func TestBuildPlannerInputDoesNotConfigureAdjacentEarlyTransitions(t *testing.T) {
	sets := []CandidateSet{
		{
			Requirement: ShotRequirement{VisualBeatID: "v1", NarrationSegmentID: "n1", StartMs: 0, EndMs: 1000, NarrationText: "第一段", VisualGoal: "第一段", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{AssetID: "v1-exact", SourceOutMs: 1000, SemanticScore: 0.8}, {AssetID: "v1-short", SourceOutMs: 900, SemanticScore: 0.8}},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "v2", NarrationSegmentID: "n2", StartMs: 1000, EndMs: 2000, NarrationText: "第二段", VisualGoal: "第二段", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{AssetID: "v2-long", SourceOutMs: 1300, SemanticScore: 0.8}, {AssetID: "v2-short", SourceOutMs: 900, SemanticScore: 0.8}},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "v3", NarrationSegmentID: "n3", StartMs: 2000, EndMs: 3000, NarrationText: "第三段", VisualGoal: "第三段", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{AssetID: "v3-long", SourceOutMs: 1300, SemanticScore: 0.8}},
		},
	}
	input, err := buildPlannerInputWithOptions("束裤带", "三段。", sets, plannerBuildOptions{VariantIndex: 1, AllowEarlyTransitions: true})
	if err != nil {
		t.Fatalf("build planner input: %v", err)
	}
	if input.Requirements[0].Slots[0].MaximumEarlyEndMs != 100 || input.Requirements[1].Slots[0].MaximumLeadingExtensionMs != 100 {
		t.Fatalf("expected first boundary tolerance, got %#v", input.Requirements)
	}
	if input.Requirements[1].Slots[0].MaximumEarlyEndMs != 0 || input.Requirements[2].Slots[0].MaximumLeadingExtensionMs != 0 {
		t.Fatalf("adjacent boundary must remain exact, got %#v", input.Requirements)
	}
}

func earlyTransitionCandidateSets(nextDurationMs int) []CandidateSet {
	return []CandidateSet{
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-action", NarrationSegmentID: "narration-action", StartMs: 0, EndMs: 2800, DurationClass: VisualBeatDurationAction, NarrationText: "完整动作。", VisualGoal: "完整展示动作", SourceType: "visual_only"},
			Candidates: []AssetCandidate{
				{AssetID: "asset-exact", SourceType: "visual_only", SourceOutMs: 2800, SemanticScore: 0.80},
				{AssetID: "asset-short", SourceType: "visual_only", SourceOutMs: 2700, SemanticScore: 0.80},
			},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-result", NarrationSegmentID: "narration-result", StartMs: 2800, EndMs: 4800, DurationClass: VisualBeatDurationStandard, NarrationText: "展示结果。", VisualGoal: "展示动作结果", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{AssetID: "asset-next", SourceType: "visual_only", SourceOutMs: nextDurationMs, SemanticScore: 0.80}},
		},
	}
}

func plannerCandidateAliasForAsset(candidates []modelgateway.EditPlanCandidate, assetID string) string {
	for _, candidate := range candidates {
		if candidate.AssetID == assetID {
			return candidate.ID
		}
	}
	return ""
}

func TestPlannerCandidateSummaryKeepsActionBeforeTruncation(t *testing.T) {
	summary := "产品：束裤带；关联卖点：防卷链条；素材类型：纯画面；景别：近景；运镜：固定；主体：人物、自行车；场景标签：" + strings.Repeat("户外、", 30) + "；画面描述：束裤带收紧固定裤脚；动作：骑行踩踏时束裤带持续固定裤脚"
	compact := truncatePlannerCandidateSummary(summary)
	if len([]rune(compact)) > maximumPlannerCandidateSemanticSummaryRunes {
		t.Fatalf("planner summary exceeds limit: %s", compact)
	}
	for _, expected := range []string{"画面描述：束裤带收紧固定裤脚", "动作：骑行踩踏时束裤带持续固定裤脚"} {
		if !strings.Contains(compact, expected) {
			t.Fatalf("expected planner summary to retain %q, got %s", expected, compact)
		}
	}
}

func TestMaterializeEditPlanAllowsMultipleClipsForVisualBeat(t *testing.T) {
	sets := []CandidateSet{{
		Requirement: ShotRequirement{
			VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", StartMs: 0, EndMs: 4000,
			NarrationText: "小小一个，放口袋里完全没负担。", Label: "便携收纳", VisualGoal: "手将束裤带折叠后放入口袋", SourceType: "visual_only",
		},
		Candidates: []AssetCandidate{
			{ID: "candidate-detail", AssetID: "asset-detail", SourceType: "visual_only", SourceInMs: 0, SourceOutMs: 2500, SemanticScore: 0.80},
			{ID: "candidate-pocket", AssetID: "asset-pocket", SourceType: "visual_only", SourceInMs: 0, SourceOutMs: 2500, SemanticScore: 0.80},
		},
	}}
	input, err := buildPlannerInput("束裤带", "小小一个，放口袋里完全没负担。", sets)
	if err != nil {
		t.Fatalf("build planner input: %v", err)
	}
	clips, err := materializeEditPlan(modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{
		{SlotID: input.Requirements[0].Slots[0].ID, CandidateID: "m001"},
		{SlotID: input.Requirements[0].Slots[1].ID, CandidateID: "m002"},
	}}, input)
	if err != nil {
		t.Fatalf("materialize edit plan: %v", err)
	}
	if len(clips) != 2 || clips[0].AssetID != "asset-detail" || clips[1].AssetID != "asset-pocket" || clips[0].SourceOutMs != 2000 || clips[1].StartMs != 2000 {
		t.Fatalf("unexpected materialized clips %#v", clips)
	}
}

func TestBuildPlannerInputRequiresEnoughUniqueAssets(t *testing.T) {
	sets := []CandidateSet{
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "展示外观。", VisualGoal: "展示外观", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{ID: "candidate-1", AssetID: "asset-shared", SourceOutMs: 2000}},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1000, EndMs: 2000, NarrationText: "展示使用。", VisualGoal: "展示使用", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{ID: "candidate-1", AssetID: "asset-shared", SourceOutMs: 2000}},
		},
	}
	if _, err := buildPlannerInput("束裤带", "展示。", sets); err == nil || !strings.Contains(err.Error(), "no globally unique material assignment") {
		t.Fatalf("expected insufficient unique candidate capacity, got %v", err)
	}
}

func TestBuildPlannerInputFindsGlobalUniqueAssignment(t *testing.T) {
	sets := []CandidateSet{
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "展示外观。", VisualGoal: "展示外观", SourceType: "visual_only"},
			Candidates: []AssetCandidate{
				{ID: "candidate-a", AssetID: "asset-a", SourceOutMs: 2000, SemanticScore: 0.80},
				{ID: "candidate-b", AssetID: "asset-b", SourceOutMs: 2000, SemanticScore: 0.80},
			},
		},
		{
			Requirement: ShotRequirement{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1000, EndMs: 2000, NarrationText: "展示使用。", VisualGoal: "展示使用", SourceType: "visual_only"},
			Candidates:  []AssetCandidate{{ID: "candidate-a", AssetID: "asset-a", SourceOutMs: 2000, SemanticScore: 0.80}},
		},
	}
	input, err := buildPlannerInput("束裤带", "展示。", sets)
	if err != nil {
		t.Fatalf("expected a global unique candidate assignment, got %v", err)
	}
	firstCandidates := input.Requirements[0].Slots[0].Candidates
	secondCandidates := input.Requirements[1].Slots[0].Candidates
	if len(firstCandidates) != 1 || len(secondCandidates) != 1 || firstCandidates[0].AssetID != "asset-b" || secondCandidates[0].AssetID != "asset-a" {
		t.Fatalf("expected the singleton material to be reserved for its required slot, got %#v", input.Requirements)
	}
}

func TestBuildPlannerInputPreservesFeasibleMaterialBeyondTopSix(t *testing.T) {
	sets := make([]CandidateSet, 0, 7)
	for requirementIndex := 0; requirementIndex < 7; requirementIndex++ {
		candidates := make([]AssetCandidate, 0, 7)
		for candidateIndex := 0; candidateIndex < 7; candidateIndex++ {
			candidates = append(candidates, AssetCandidate{
				ID:          fmt.Sprintf("candidate-%d", candidateIndex+1),
				AssetID:     fmt.Sprintf("asset-%d", candidateIndex+1),
				SourceOutMs: 2000,
			})
		}
		sets = append(sets, CandidateSet{
			Requirement: ShotRequirement{
				VisualBeatID: fmt.Sprintf("visual-%d", requirementIndex+1), NarrationSegmentID: fmt.Sprintf("narration-%d", requirementIndex+1),
				StartMs: requirementIndex * 1000, EndMs: (requirementIndex + 1) * 1000,
				NarrationText: "展示产品。", VisualGoal: "展示产品", SourceType: "visual_only",
			},
			Candidates: candidates,
		})
	}
	input, err := buildPlannerInput("束裤带", "展示产品。", sets)
	if err != nil {
		t.Fatalf("expected top-12 matching to preserve a global assignment: %v", err)
	}
	foundSeventhMaterial := false
	for _, requirement := range input.Requirements {
		if len(requirement.Slots[0].Candidates) > editPlannerCandidatesPerVisualBeat {
			t.Fatalf("planner slot exceeds candidate limit: %#v", requirement.Slots[0])
		}
		for _, candidate := range requirement.Slots[0].Candidates {
			if candidate.ID == "m007" {
				foundSeventhMaterial = true
			}
		}
	}
	if !foundSeventhMaterial {
		t.Fatal("expected the globally required seventh material to survive per-slot trimming")
	}
}

func TestBuildPlannerInputRequiresDistinctAssetsForLongVisualBeat(t *testing.T) {
	sets := []CandidateSet{{
		Requirement: ShotRequirement{VisualBeatID: "visual-long", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 4000, NarrationText: "完整展示动作。", VisualGoal: "完整展示动作", SourceType: "visual_only"},
		Candidates:  []AssetCandidate{{ID: "candidate-1", AssetID: "asset-1", SourceOutMs: 5000}},
	}}
	if _, err := buildPlannerInput("束裤带", "完整展示动作。", sets); err == nil || !strings.Contains(err.Error(), "no globally unique material assignment") {
		t.Fatalf("expected long visual beat to require two assets, got %v", err)
	}
}

func TestBuildPlannerInputUsesDistinctFallbackAssetsBelowThreshold(t *testing.T) {
	sets := []CandidateSet{{
		Requirement: ShotRequirement{VisualBeatID: "visual-long", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 4000, NarrationText: "展示夜骑反光。", VisualGoal: "夜骑中反光条清晰可见", SourceType: "visual_only"},
		Candidates: []AssetCandidate{
			{ID: "candidate-top", AssetID: "asset-top", SourceOutMs: 5000, SemanticScore: 0.74},
			{ID: "candidate-next", AssetID: "asset-next", SourceOutMs: 5000, SemanticScore: 0.72},
		},
	}}
	input, err := buildPlannerInput("束裤带", "展示夜骑反光。", sets)
	if err != nil {
		t.Fatalf("expected below-threshold fallback materials to satisfy unique slots: %v", err)
	}
	slots := input.Requirements[0].Slots
	if len(slots) != 2 || len(slots[0].Candidates) != 1 || len(slots[1].Candidates) != 1 || slots[0].Candidates[0].AssetID == slots[1].Candidates[0].AssetID {
		t.Fatalf("expected each slot to expose its distinct assigned fallback, got %#v", slots)
	}
}

func TestBuildDeterministicSlotsAvoidsActionDeadZone(t *testing.T) {
	if padding := visualBeatTimelinePadding(modelgateway.VisualDurationClassAction, 3180); padding != 0 {
		t.Fatalf("expected complete 3180ms action narration to need no artificial padding, padding=%d", padding)
	}
	if padding := visualBeatTimelinePadding(modelgateway.VisualDurationClassAction, 3190); padding != 0 {
		t.Fatalf("expected complete 3190ms action narration to need no artificial padding, padding=%d", padding)
	}
	if padding := visualBeatTimelinePadding(modelgateway.VisualDurationClassAction, 2700); padding != 100 {
		t.Fatalf("expected a short action to be padded only to 2800ms, padding=%d", padding)
	}
	if padding := visualBeatTimelinePadding(modelgateway.VisualDurationClassAction, 3540); padding != 60 {
		t.Fatalf("expected a native dead-zone action to normalize to 3600ms, padding=%d", padding)
	}

	nextSlotID := 1
	slots, err := buildDeterministicEditPlanSlots(ShotRequirement{
		StartMs: 0, EndMs: 3500, DurationClass: VisualBeatDurationAction,
	}, &nextSlotID)
	if err != nil {
		t.Fatalf("build action slots: %v", err)
	}
	if len(slots) != 1 || slots[0].DurationMs != 3500 || slots[0].Role != modelgateway.EditPlanSlotRoleActionPrimary {
		t.Fatalf("expected one complete action slot, got %#v", slots)
	}

	nextSlotID = 1
	slots, err = buildDeterministicEditPlanSlots(ShotRequirement{
		StartMs: 0, EndMs: 4070, DurationClass: VisualBeatDurationAction,
	}, &nextSlotID)
	if err != nil {
		t.Fatalf("build split action slots: %v", err)
	}
	if len(slots) != 2 || slots[0].DurationMs != 3270 || slots[1].DurationMs != 800 {
		t.Fatalf("expected deterministic 3270ms + 800ms slots, got %#v", slots)
	}
}

func TestGenerationPlanningLLMConcurrencyUsesRuntimeSetting(t *testing.T) {
	settings := NewSystemConfigService()
	if _, err := settings.Upsert(SystemConfig{Key: "llm.max_concurrency", Value: 3, Type: "number"}); err != nil {
		t.Fatalf("set LLM concurrency failed: %v", err)
	}
	planning := NewGenerationPlanningService(nil, nil, nil, nil, settings, nil, config.Config{})
	if got := planning.llmMaxConcurrency(context.Background()); got != 3 {
		t.Fatalf("expected runtime LLM concurrency 3, got %d", got)
	}
}

func TestReserveSingletonPlannerMaterialsPropagatesRequiredChoices(t *testing.T) {
	requirements := []modelgateway.EditPlanRequirement{
		{Slots: []modelgateway.EditPlanSlot{{ID: "s001", Candidates: []modelgateway.EditPlanCandidate{{ID: "m001"}, {ID: "m002"}}}}},
		{Slots: []modelgateway.EditPlanSlot{{ID: "s002", Candidates: []modelgateway.EditPlanCandidate{{ID: "m002"}, {ID: "m003"}}}}},
		{Slots: []modelgateway.EditPlanSlot{{ID: "s003", Candidates: []modelgateway.EditPlanCandidate{{ID: "m001"}}}}},
	}
	if err := reserveSingletonPlannerMaterials(requirements); err != nil {
		t.Fatalf("reserve singleton materials: %v", err)
	}
	for index, expected := range []string{"m002", "m003", "m001"} {
		candidates := requirements[index].Slots[0].Candidates
		if len(candidates) != 1 || candidates[0].ID != expected {
			t.Fatalf("slot %d expected reserved material %s, got %#v", index+1, expected, candidates)
		}
	}
}

func TestReserveSingletonPlannerMaterialsRejectsForcedConflict(t *testing.T) {
	requirements := []modelgateway.EditPlanRequirement{
		{Slots: []modelgateway.EditPlanSlot{{ID: "s001", Candidates: []modelgateway.EditPlanCandidate{{ID: "m001"}}}}},
		{Slots: []modelgateway.EditPlanSlot{{ID: "s002", Candidates: []modelgateway.EditPlanCandidate{{ID: "m001"}}}}},
	}
	err := reserveSingletonPlannerMaterials(requirements)
	if err == nil || !strings.Contains(err.Error(), "require the same material") {
		t.Fatalf("expected forced material conflict, got %v", err)
	}
}
