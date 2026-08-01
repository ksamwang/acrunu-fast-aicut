package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func TestCandidateSemanticSummaryPrioritizesSceneAndAction(t *testing.T) {
	input := "产品：束裤带；关联卖点：防卷链条；素材类型：纯画面；景别：近景；运镜：固定；主体：人物、自行车；场景标签：户外、草地、骑行；质量标签：清晰、稳定；场景：户外草地；画面描述：束裤带环绕脚踝并收紧裤脚；动作：骑行踩踏时束裤带持续固定裤脚"
	summary := candidateSemanticSummary(input)
	for _, expected := range []string{"产品：束裤带", "画面描述：束裤带环绕脚踝并收紧裤脚", "动作：骑行踩踏时束裤带持续固定裤脚"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("expected prioritized summary to contain %q, got %s", expected, summary)
		}
	}
	if strings.Index(summary, "动作：") > strings.Index(summary, "关联卖点：") {
		t.Fatalf("expected action before secondary metadata, got %s", summary)
	}
}

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
	if input.MinimumDurationMs != modelgateway.MinimumEditPlanClipDurationMs || input.Limit != maxCandidatesPerNarrationSegment {
		t.Fatalf("unexpected duration or limit %#v", input)
	}
	if len(input.SourceTypes) != 2 || input.SourceTypes[0] != "visual_only" || input.SourceTypes[1] != "talking_head" {
		t.Fatalf("unexpected source types %#v", input.SourceTypes)
	}
}

func TestMinimumCandidateDurationKeepsShortSupportMaterial(t *testing.T) {
	for _, durationClass := range []string{VisualBeatDurationBrief, VisualBeatDurationStandard, VisualBeatDurationAction} {
		if got := minimumCandidateDuration(durationClass); got != modelgateway.MinimumEditPlanClipDurationMs {
			t.Fatalf("duration class %q must retrieve from %dms, got %d", durationClass, modelgateway.MinimumEditPlanClipDurationMs, got)
		}
	}
}

func TestAssetCandidateServicePlanningRetrievalMergesDurationQueries(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	store := &recordingCandidateStore{}
	embedder := &candidateTestEmbedder{}
	service := NewAssetCandidateService(nil, assets, NewSystemConfigService(), NewModelProviderService(), config.Config{}).
		WithEmbedder(embedder).
		WithStore(store)

	sets, err := service.RetrieveForPlanning(context.Background(), product.ID, []ShotRequirement{{
		VisualBeatID:       "visual-1",
		NarrationSegmentID: "narration-1",
		StartMs:            0,
		EndMs:              1800,
		NarrationText:      "固定裤脚。",
		VisualGoal:         "展示固定裤脚",
		SourceType:         "visual_only",
	}}, maxCandidatesPerNarrationSegment, PlanningCandidateRetrievalOptions{
		MinimumDurationsByVisualBeat: map[string][]int{"visual-1": {1200, 800, 1200}},
	})
	if err != nil {
		t.Fatalf("retrieve planning candidates: %v", err)
	}
	if len(embedder.inputs) != 1 {
		t.Fatalf("expected one embedding for one visual goal, got %d", len(embedder.inputs))
	}
	if len(store.inputs) != 2 || store.inputs[0].MinimumDurationMs != 800 || store.inputs[1].MinimumDurationMs != 1200 {
		t.Fatalf("unexpected duration-aware searches %#v", store.inputs)
	}
	for _, input := range store.inputs {
		if input.Limit != planningCandidateRetrievalPoolSize {
			t.Fatalf("expected internal pool size %d, got %#v", planningCandidateRetrievalPoolSize, input)
		}
	}
	if len(sets) != 1 || len(sets[0].Candidates) != 1 {
		t.Fatalf("expected one merged candidate, got %#v", sets)
	}
}

func TestBuildShotRequirementsCombinesNarrationAcrossVisualBeat(t *testing.T) {
	requirements, err := BuildShotRequirements([]VisualBeat{
		{ID: "visual-1", NarrationSegmentID: "n-1", NarrativeBeatID: "business-action", StartMs: 0, EndMs: 3000, DurationClass: VisualBeatDurationAction, SellingPoint: "快拆收纳", VisualGoal: "完整展示快拆和收纳", SourceType: "visual_only"},
		{ID: "visual-2", NarrationSegmentID: "n-2", StartMs: 3000, EndMs: 5000, DurationClass: VisualBeatDurationStandard, SellingPoint: "结果", VisualGoal: "展示收纳结果", SourceType: "visual_only"},
	}, []NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 2000, Text: "第一句"},
		{ID: "n-2", StartMs: 2000, EndMs: 5000, Text: "第二句"},
	})
	if err != nil {
		t.Fatalf("build requirements: %v", err)
	}
	if len(requirements) != 2 || requirements[0].NarrationSegmentID != "n-1" || requirements[1].NarrationSegmentID != "n-2" {
		t.Fatalf("unexpected requirements %#v", requirements)
	}
	if len(requirements[0].NarrationSegmentIDs) != 2 || requirements[0].NarrationText != "第一句第二句" || requirements[0].DurationClass != VisualBeatDurationAction {
		t.Fatalf("expected cross-subtitle narration context, got %#v", requirements[0])
	}
	if requirements[0].NarrativeBeatID != "business-action" {
		t.Fatalf("expected business intention reference in shot requirement, got %#v", requirements[0])
	}
}

func TestBuildShotRequirementsAllowsEditorialPauseBetweenNarrationSegments(t *testing.T) {
	requirements, err := BuildShotRequirements([]VisualBeat{
		{ID: "visual-1", NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, VisualGoal: "展示动作", SourceType: "visual_only"},
		{ID: "visual-2", NarrationSegmentID: "n-2", StartMs: 1000, EndMs: 2000, VisualGoal: "展示结果", SourceType: "visual_only"},
	}, []NarrationSegment{
		{ID: "n-1", StartMs: 0, EndMs: 900, Text: "第一句"},
		{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句"},
	})
	if err != nil || len(requirements) != 2 {
		t.Fatalf("expected narration gaps to represent editorial pauses, got %#v, err=%v", requirements, err)
	}
}
