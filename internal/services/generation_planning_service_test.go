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
	call int
}

func (s *planningCandidateStore) SearchCandidates(_ context.Context, input CandidateSearchInput) ([]AssetCandidate, error) {
	s.call++
	return []AssetCandidate{{
		ID:                      fmt.Sprintf("candidate-%d", s.call),
		AssetID:                 fmt.Sprintf("asset-%d", s.call),
		ObjectType:              "shot",
		SourceType:              "visual_only",
		SourceInMs:              0,
		SourceOutMs:             input.MinimumDurationMs + 1200,
		AssetDurationMs:         input.MinimumDurationMs + 1200,
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
		beats = append(beats, modelgateway.VisualPlanBeat{
			NarrationSegmentID: segment.ID,
			StartMs:            segment.StartMs,
			EndMs:              segment.EndMs,
			Label:              "画面展示",
			VisualGoal:         "展示产品使用动作。",
			SourceType:         "visual_only",
		})
	}
	return modelgateway.VisualPlanResult{VisualBeats: beats}, nil
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
			{SellingPoint: "避免蹭链条", VisualGoal: "展示裤脚靠近链条。", SourceType: "visual_only"},
			{SellingPoint: "固定更稳", VisualGoal: "展示固定后的骑行状态。", SourceType: "visual_only"},
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
		{NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, Label: "痛点", VisualGoal: "裤脚靠近链条", SourceType: "visual_only"},
		{NarrationSegmentID: "narration-1", StartMs: 1000, EndMs: 2000, Label: "固定", VisualGoal: "展示束裤带固定裤脚", SourceType: "visual_only"},
		{NarrationSegmentID: "narration-2", StartMs: 2000, EndMs: 3500, Label: "结果", VisualGoal: "展示固定后骑行", SourceType: "visual_only"},
	}}).WithPlanner(deterministicEditPlanner{})

	plan, err := planning.Generate(context.Background(), GenerateEditPlanInput{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
	})
	if err != nil {
		t.Fatalf("generate plan: %v", err)
	}
	if plan.Status != "ready" || len(plan.VisualBeats) != 3 || len(plan.Clips) != 3 {
		t.Fatalf("unexpected plan %#v", plan)
	}
	if plan.Clips[0].NarrationSegmentID != plan.Clips[1].NarrationSegmentID || plan.Clips[0].VisualBeatID == plan.Clips[1].VisualBeatID {
		t.Fatalf("expected two clips for the first narration segment, got %#v", plan.Clips)
	}
	if plan.Clips[0].AssetID != "asset-1" || plan.Clips[1].AssetID != "asset-2" || !plan.Clips[1].UseOriginalAudio {
		t.Fatalf("candidate mapping or audio policy was not retained %#v", plan.Clips)
	}
	var artifacts struct {
		VisualBeats   []VisualBeat   `json:"visual_beats"`
		CandidateSets []CandidateSet `json:"candidate_sets"`
		Clips         []EditPlanClip `json:"clips"`
	}
	if err := json.Unmarshal(plan.PlanJSON, &artifacts); err != nil {
		t.Fatalf("decode plan artifacts: %v", err)
	}
	if len(artifacts.VisualBeats) != 3 || len(artifacts.CandidateSets) != 3 || len(artifacts.Clips) != 3 {
		t.Fatalf("plan artifact snapshot is incomplete %#v", artifacts)
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
