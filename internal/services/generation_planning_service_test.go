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
	return []AssetCandidate{{
		ID:                      fmt.Sprintf("candidate-%d", s.call),
		AssetID:                 fmt.Sprintf("asset-%d", s.call),
		ObjectType:              "shot",
		SourceType:              "visual_only",
		SourceInMs:              0,
		SourceOutMs:             10_000,
		AssetDurationMs:         10_000,
		DefaultUseOriginalAudio: s.call == 2,
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
	result := modelgateway.EditPlanResult{Clips: make([]modelgateway.EditPlanClipChoice, 0, len(input.Requirements))}
	for _, requirement := range input.Requirements {
		candidateID := requirement.Candidates[0].ID
		if p.invalidCandidate {
			candidateID = "not-allowed"
		}
		result.Clips = append(result.Clips, modelgateway.EditPlanClipChoice{
			VisualBeatID: requirement.VisualBeatID,
			CandidateID:  candidateID,
			StartMs:      requirement.StartMs,
			EndMs:        requirement.EndMs,
			SourceInMs:   requirement.Candidates[0].SourceInMs,
			SourceOutMs:  requirement.Candidates[0].SourceInMs + (requirement.EndMs - requirement.StartMs),
			Label:        "镜头展示",
			VisualGoal:   "匹配旁白表达。",
		})
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
	}
	result := modelgateway.VisualPlanResult{VisualBeats: []modelgateway.VisualPlanBeat{
		{NarrationSegmentID: "quick", StartMs: 0, EndMs: 1015, DurationClass: modelgateway.VisualDurationClassBrief, Label: "快拆", VisualGoal: "展示一秒快拆", SourceType: "visual_only"},
		{NarrationSegmentID: "velcro", StartMs: 1015, EndMs: 2284, DurationClass: modelgateway.VisualDurationClassStandard, Label: "魔术贴", VisualGoal: "展示大面积魔术贴", SourceType: "visual_only"},
		{NarrationSegmentID: "storage", StartMs: 2284, EndMs: 3250, DurationClass: modelgateway.VisualDurationClassAction, Label: "收纳", VisualGoal: "完整展示放入口袋", SourceType: "visual_only"},
		{NarrationSegmentID: "elastic", StartMs: 3250, EndMs: 5430, DurationClass: modelgateway.VisualDurationClassAction, Label: "拉伸", VisualGoal: "完整展示反复拉伸", SourceType: "visual_only"},
		{NarrationSegmentID: "fit", StartMs: 5430, EndMs: 6450, DurationClass: modelgateway.VisualDurationClassStandard, Label: "贴合", VisualGoal: "展示贴合脚踝", SourceType: "visual_only"},
	}}

	beats, segments, pauses, durationMs, err := materializeVisualTimeline(result, input)
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
			{ID: "narration-1", StartMs: 0, EndMs: 2000, Text: "骑行时裤脚不再蹭链条。"},
			{ID: "narration-2", StartMs: 2000, EndMs: 3500, Text: "固定后更安心。"},
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
	if len(store.inputs) != 2 {
		t.Fatalf("expected one candidate query per visual beat, got %d", len(store.inputs))
	}
	for _, candidateInput := range store.inputs {
		if candidateInput.Limit != editPlannerCandidatesPerVisualBeat {
			t.Fatalf("expected bounded candidate limit %d, got %#v", editPlannerCandidatesPerVisualBeat, candidateInput)
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
	if err == nil || !strings.Contains(err.Error(), "outside the allowed set") {
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
	input := buildPlannerInput("束裤带", "魔术贴一粘即合。", []CandidateSet{{
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
			SourceType:      "visual_only",
			SourceInMs:      100,
			SourceOutMs:     1600,
			SemanticSummary: "画面描述：手部将束裤带魔术贴快速粘合。",
			SemanticScore:   0.92,
		}},
	}})

	candidate := input.Requirements[0].Candidates[0]
	if input.Requirements[0].VisualBeatID != "visual-1" || candidate.SemanticSummary != "画面描述：手部将束裤带魔术贴快速粘合。" || candidate.SemanticScore != 0.92 {
		t.Fatalf("candidate semantic evidence was not preserved %#v", candidate)
	}
}

func TestBuildPlannerInputBoundsCandidatesAndSemanticSummary(t *testing.T) {
	candidates := make([]AssetCandidate, 0, editPlannerCandidatesPerVisualBeat+1)
	for index := 0; index < editPlannerCandidatesPerVisualBeat+1; index++ {
		candidates = append(candidates, AssetCandidate{
			ID:              fmt.Sprintf("candidate-%d", index+1),
			SourceType:      "visual_only",
			SourceInMs:      0,
			SourceOutMs:     2000,
			SemanticSummary: strings.Repeat("镜", maximumPlannerCandidateSemanticSummaryRunes+20),
		})
	}
	input := buildPlannerInput("束裤带", "固定裤脚。", []CandidateSet{{
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
	got := input.Requirements[0].Candidates
	if len(got) != editPlannerCandidatesPerVisualBeat || got[len(got)-1].ID != "candidate-6" {
		t.Fatalf("expected first %d candidates, got %#v", editPlannerCandidatesPerVisualBeat, got)
	}
	if len([]rune(got[0].SemanticSummary)) != maximumPlannerCandidateSemanticSummaryRunes {
		t.Fatalf("expected %d-rune summary, got %q", maximumPlannerCandidateSemanticSummaryRunes, got[0].SemanticSummary)
	}
}

func TestMaterializeEditPlanAllowsMultipleClipsForVisualBeat(t *testing.T) {
	sets := []CandidateSet{{
		Requirement: ShotRequirement{
			VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", StartMs: 0, EndMs: 3440,
			NarrationText: "小小一个，放口袋里完全没负担。", VisualGoal: "展示束裤带小巧，放入口袋", SourceType: "visual_only",
		},
		Candidates: []AssetCandidate{
			{ID: "candidate-detail", AssetID: "asset-detail", SourceType: "visual_only", SourceInMs: 0, SourceOutMs: 1800},
			{ID: "candidate-pocket", AssetID: "asset-pocket", SourceType: "visual_only", SourceInMs: 0, SourceOutMs: 2500},
		},
	}}
	clips, err := materializeEditPlan(modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-detail", StartMs: 0, EndMs: 940, SourceInMs: 200, SourceOutMs: 1140, Label: "产品特写", VisualGoal: "展示束裤带小巧"},
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-pocket", StartMs: 940, EndMs: 3440, SourceInMs: 0, SourceOutMs: 2500, Label: "放入口袋", VisualGoal: "完整展示放入口袋动作"},
	}}, sets)
	if err != nil {
		t.Fatalf("materialize multi-clip edit plan: %v", err)
	}
	if len(clips) != 2 || clips[0].AssetID != "asset-detail" || clips[1].AssetID != "asset-pocket" || clips[1].StartMs != clips[0].EndMs {
		t.Fatalf("unexpected materialized clips %#v", clips)
	}
}
