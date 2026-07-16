package services

import (
	"context"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type recordingCandidateStore struct {
	inputs []CandidateSearchInput
}

func (s *recordingCandidateStore) SearchCandidates(_ context.Context, input CandidateSearchInput) ([]AssetCandidate, error) {
	s.inputs = append(s.inputs, input)
	return []AssetCandidate{{
		ID:                      "candidate-1",
		AssetID:                 "asset-1",
		ObjectType:              "shot",
		SourceType:              "visual_only",
		SourceInMs:              0,
		SourceOutMs:             5000,
		AssetDurationMs:         5000,
		DefaultUseOriginalAudio: false,
		SemanticScore:           0.93,
	}}, nil
}

type candidateTestEmbedder struct {
	inputs []modelgateway.EmbedTextInput
}

func (e *candidateTestEmbedder) EmbedText(_ context.Context, input modelgateway.EmbedTextInput) (modelgateway.EmbedTextResult, error) {
	e.inputs = append(e.inputs, input)
	return modelgateway.EmbedTextResult{Embedding: []float64{0.2, 0.3}}, nil
}

func TestAssetCandidateServiceUsesVisualGoalWithoutSellingPointRelation(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	store := &recordingCandidateStore{}
	embedder := &candidateTestEmbedder{}
	service := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(embedder).
		WithStore(store)

	sets, err := service.Retrieve(context.Background(), product.ID, []ShotRequirement{{
		VisualBeatID:       "visual-1",
		NarrationSegmentID: "narration-1",
		StartMs:            0,
		EndMs:              1800,
		NarrationText:      "裤脚不再蹭链条。",
		SellingPoint:       "避免蹭链条，骑行更安心",
		VisualGoal:         "展示固定裤脚和远离链条的动作。",
		SourceType:         "mixed",
	}}, 20)
	if err != nil {
		t.Fatalf("retrieve candidates: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Candidates) != 1 {
		t.Fatalf("unexpected candidate sets %#v", sets)
	}
	if len(store.inputs) != 1 {
		t.Fatalf("expected one store request, got %d", len(store.inputs))
	}
	if len(embedder.inputs) != 1 || embedder.inputs[0].Text != "展示固定裤脚和远离链条的动作。" {
		t.Fatalf("expected visual_goal-only embedding query, got %#v", embedder.inputs)
	}
	input := store.inputs[0]
	if input.MinimumDurationMs != 1800 || input.Limit != maxCandidatesPerNarrationSegment {
		t.Fatalf("unexpected duration or limit %#v", input)
	}
	if len(input.SourceTypes) != 2 || input.SourceTypes[0] != "visual_only" || input.SourceTypes[1] != "talking_head" {
		t.Fatalf("unexpected source types %#v", input.SourceTypes)
	}
}

func TestBuildShotRequirementsAllowsMultipleVisualBeatsForOneNarrationSentence(t *testing.T) {
	requirements, err := BuildShotRequirements([]VisualBeat{
		{ID: "visual-1", NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, SellingPoint: "痛点", VisualGoal: "展示裤脚靠近链条", SourceType: "talking_head"},
		{ID: "visual-2", NarrationSegmentID: "n-1", StartMs: 1000, EndMs: 2000, SellingPoint: "解决方案", VisualGoal: "展示束裤带固定动作", SourceType: "visual_only"},
		{ID: "visual-3", NarrationSegmentID: "n-2", StartMs: 2000, EndMs: 3000, SellingPoint: "结果", VisualGoal: "展示固定后的骑行状态", SourceType: "visual_only"},
	}, []NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 2000, Text: "第一句"},
		{ID: "n-2", StartMs: 2000, EndMs: 3000, Text: "第二句"},
	})
	if err != nil {
		t.Fatalf("build requirements: %v", err)
	}
	if len(requirements) != 3 || requirements[0].NarrationSegmentID != "n-1" || requirements[1].NarrationSegmentID != "n-1" || requirements[2].VisualBeatID != "visual-3" {
		t.Fatalf("unexpected requirements %#v", requirements)
	}
}

func TestBuildShotRequirementsRejectsNonContinuousNarrationTimeline(t *testing.T) {
	_, err := BuildShotRequirements([]VisualBeat{{
		ID: "visual-1", NarrationSegmentID: "n-1", StartMs: 0, EndMs: 900, VisualGoal: "展示动作", SourceType: "visual_only",
	}}, []NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 900, Text: "第一句"},
		{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句"},
	})
	if err == nil {
		t.Fatal("expected non-continuous narration timeline to be rejected")
	}
}
