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

func TestAssetCandidateServiceAppliesRequirementFilters(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	point, err := assets.CreateSellingPoint(product.ID, CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	store := &recordingCandidateStore{}
	service := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(candidateTestEmbedder{}).
		WithStore(store)

	sets, err := service.Retrieve(context.Background(), product.ID, []ShotRequirement{{
		NarrationSegmentID: "narration-1",
		StartMs:            0,
		EndMs:              1800,
		NarrationText:      "裤脚不再蹭链条。",
		SellingPoint:       "避免蹭链条，骑行更安心",
		VisualGoal:         "展示固定裤脚动作。",
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
	input := store.inputs[0]
	if input.MinimumDurationMs != 1800 || input.Limit != maxCandidatesPerNarrationSegment {
		t.Fatalf("unexpected duration or limit %#v", input)
	}
	if len(input.SourceTypes) != 2 || input.SourceTypes[0] != "visual_only" || input.SourceTypes[1] != "talking_head" {
		t.Fatalf("unexpected source types %#v", input.SourceTypes)
	}
	if len(input.SellingPointIDs) != 1 || input.SellingPointIDs[0] != point.ID {
		t.Fatalf("expected matched selling point id, got %#v", input.SellingPointIDs)
	}
}

type candidateTestEmbedder struct{}

func (candidateTestEmbedder) EmbedText(_ context.Context, _ modelgateway.EmbedTextInput) (modelgateway.EmbedTextResult, error) {
	return modelgateway.EmbedTextResult{Embedding: []float64{0.2, 0.3}}, nil
}

func TestBuildShotRequirementsMapsNarrationToBeatsInOrder(t *testing.T) {
	requirements, err := BuildShotRequirements([]NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 1000, Text: "第一句"},
		{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句"},
		{ID: "n-3", StartMs: 2000, EndMs: 3000, Text: "第三句"},
	}, []VoiceoverBeat{
		{SellingPoint: "痛点", VisualGoal: "建立问题", SourceType: "talking_head"},
		{SellingPoint: "结果", VisualGoal: "展示结果", SourceType: "visual_only"},
	})
	if err != nil {
		t.Fatalf("build requirements: %v", err)
	}
	if len(requirements) != 3 || requirements[0].SellingPoint != "痛点" || requirements[2].SellingPoint != "结果" {
		t.Fatalf("unexpected requirements %#v", requirements)
	}
}

func TestBuildShotRequirementsRejectsNonContinuousNarrationTimeline(t *testing.T) {
	_, err := BuildShotRequirements([]NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 900, Text: "第一句"},
		{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句"},
	}, nil)
	if err == nil {
		t.Fatal("expected non-continuous narration timeline to be rejected")
	}
}
