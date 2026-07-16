package services

import (
	"context"
	"errors"
	"fmt"
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
			NarrationSegmentID: requirement.NarrationSegmentID,
			CandidateID:        candidateID,
			SourceInMs:         requirement.Candidates[0].SourceInMs,
			SourceOutMs:        requirement.Candidates[0].SourceInMs + (requirement.EndMs - requirement.StartMs),
			Label:              "镜头展示",
			VisualGoal:         "匹配旁白表达。",
		})
	}
	return result, nil
}

func TestGenerationPlanningServicePersistsCandidateBoundPlan(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID:          "voiceover-task-1",
		ProductID:   product.ID,
		ProductName: product.Name,
		ScriptText:  "骑行时裤脚不再蹭链条，固定后更安心。",
		Beats: []VoiceoverBeat{
			{SellingPoint: "避免蹭链条", VisualGoal: "展示裤脚靠近链条。", SourceType: "visual_only"},
			{SellingPoint: "固定更稳", VisualGoal: "展示固定后的骑行状态。", SourceType: "visual_only"},
		},
		NarrationSegments: []NarrationSegment{
			{ID: "narration-1", StartMs: 0, EndMs: 1500, Text: "骑行时裤脚不再蹭链条。"},
			{ID: "narration-2", StartMs: 1500, EndMs: 3000, Text: "固定后更安心。"},
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
		WithEmbedder(candidateTestEmbedder{}).
		WithStore(store)
	planning := NewGenerationPlanningService(
		runs,
		loader,
		assets,
		candidates,
		NewSystemConfigService(),
		NewModelProviderService(),
		config.Config{},
	).WithPlanner(deterministicEditPlanner{})

	plan, err := planning.Generate(context.Background(), GenerateEditPlanInput{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
	})
	if err != nil {
		t.Fatalf("generate plan: %v", err)
	}
	if plan.Status != "ready" || len(plan.Clips) != 2 {
		t.Fatalf("unexpected plan %#v", plan)
	}
	if plan.Clips[0].AssetID != "asset-1" || plan.Clips[1].AssetID != "asset-2" || !plan.Clips[1].UseOriginalAudio {
		t.Fatalf("candidate mapping or audio policy was not retained %#v", plan.Clips)
	}
	updatedRun, err := runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updatedRun.Status != generationRunStatusGenerating || updatedRun.Stage != generationRunStagePlanReady {
		t.Fatalf("expected generating plan-ready run, got %#v", updatedRun)
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
		NarrationSegments: []NarrationSegment{{
			ID: "narration-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
	}}
	runs := NewGenerationRunService(loader)
	run, _ := runs.Create(context.Background(), CreateGenerationRunInput{ProductID: product.ID})
	_ = runs.LinkTask(context.Background(), run.ID, loader.work.ID, generationRunTaskStageVoiceover)
	_ = runs.AttachVoiceoverArtifacts(context.Background(), run.ID, loader.work.ID, "script-1", "voiceover-1")
	candidates := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(candidateTestEmbedder{}).
		WithStore(&planningCandidateStore{})
	planning := NewGenerationPlanningService(runs, loader, assets, candidates, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithPlanner(deterministicEditPlanner{invalidCandidate: true})

	_, err := planning.Generate(context.Background(), GenerateEditPlanInput{GenerationRunID: run.ID, ScriptVariantID: "script-1", VoiceoverID: "voiceover-1"})
	if err == nil || !stringsContains(err.Error(), "outside the allowed set") {
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
	if candidate.SemanticSummary != "画面描述：手部将束裤带魔术贴快速粘合。" || candidate.SemanticScore != 0.92 {
		t.Fatalf("candidate semantic evidence was not preserved %#v", candidate)
	}
}

func stringsContains(value string, expected string) bool {
	return len(value) >= len(expected) && (value == expected || containsString(value, expected))
}

func containsString(value string, expected string) bool {
	for index := 0; index+len(expected) <= len(value); index++ {
		if value[index:index+len(expected)] == expected {
			return true
		}
	}
	return false
}

var _ = errors.Is
