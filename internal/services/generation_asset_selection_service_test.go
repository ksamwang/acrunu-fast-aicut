package services

import (
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func TestSelectDiversePlannerResultAvoidsMaterialUsedByBatch(t *testing.T) {
	input := modelgateway.EditPlanInput{Requirements: []modelgateway.EditPlanRequirement{
		selectionTestRequirement("visual-1", "s001", 0, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", DiversityScore: 0.92},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", DiversityScore: 0.90},
			{ID: "m003", AssetID: "asset-c", ReuseKey: "asset-c", DiversityScore: 0.89},
		}),
		selectionTestRequirement("visual-2", "s002", 1000, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", DiversityScore: 0.91},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", DiversityScore: 0.90},
			{ID: "m003", AssetID: "asset-c", ReuseKey: "asset-c", DiversityScore: 0.88},
		}),
	}}
	preferred := modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{
		{SlotID: "s001", CandidateID: "m001"},
		{SlotID: "s002", CandidateID: "m002"},
	}}

	result, err := selectDiversePlannerResult(input, preferred, map[string]int{"asset-a": 1}, map[string]int{}, map[string]bool{})
	if err != nil {
		t.Fatalf("select diverse result: %v", err)
	}
	if result.Clips[0].CandidateID != "m003" || result.Clips[1].CandidateID != "m002" {
		t.Fatalf("expected unused globally unique assignment, got %#v", result.Clips)
	}
}

func TestSelectDiversePlannerResultFallsBackToLeastUsedRelevantMaterial(t *testing.T) {
	input := modelgateway.EditPlanInput{Requirements: []modelgateway.EditPlanRequirement{
		selectionTestRequirement("visual-1", "s001", 0, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", DiversityScore: 0.91},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", DiversityScore: 0.92},
		}),
	}}
	preferred := modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{{SlotID: "s001", CandidateID: "m002"}}}

	result, err := selectDiversePlannerResult(input, preferred, map[string]int{"asset-a": 1, "asset-b": 2}, map[string]int{}, map[string]bool{})
	if err != nil {
		t.Fatalf("select fallback result: %v", err)
	}
	if result.Clips[0].CandidateID != "m001" {
		t.Fatalf("expected least-used relevant material, got %#v", result.Clips)
	}
}

func TestSelectDiversePlannerResultAvoidsPreviousRegenerationMaterial(t *testing.T) {
	input := modelgateway.EditPlanInput{Requirements: []modelgateway.EditPlanRequirement{
		selectionTestRequirement("visual-1", "s001", 0, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", DiversityScore: 0.92},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", DiversityScore: 0.90},
		}),
	}}
	preferred := modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{{SlotID: "s001", CandidateID: "m001"}}}

	result, err := selectDiversePlannerResult(input, preferred, map[string]int{}, map[string]int{}, map[string]bool{"asset-a": true})
	if err != nil {
		t.Fatalf("select regenerated result: %v", err)
	}
	if result.Clips[0].CandidateID != "m002" {
		t.Fatalf("expected regeneration to avoid its previous material, got %#v", result.Clips)
	}
}

func TestSelectDiversePlannerResultKeepsSemanticQualityAheadOfRecentUse(t *testing.T) {
	input := modelgateway.EditPlanInput{Requirements: []modelgateway.EditPlanRequirement{
		selectionTestRequirement("visual-1", "s001", 0, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", SemanticScore: 0.92},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", SemanticScore: 0.86},
		}),
	}}
	preferred := modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{{SlotID: "s001", CandidateID: "m001"}}}

	result, err := selectDiversePlannerResult(input, preferred, map[string]int{}, map[string]int{"asset-a": 1}, map[string]bool{})
	if err != nil {
		t.Fatalf("select semantically stronger result: %v", err)
	}
	if result.Clips[0].CandidateID != "m001" {
		t.Fatalf("expected recent usage to remain a soft penalty, got %#v", result.Clips)
	}
}

func TestSelectDiversePlannerResultReusesQualifiedBeforeUnusedFallback(t *testing.T) {
	input := modelgateway.EditPlanInput{Requirements: []modelgateway.EditPlanRequirement{
		selectionTestRequirement("visual-1", "s001", 0, []modelgateway.EditPlanCandidate{
			{ID: "m001", AssetID: "asset-a", ReuseKey: "asset-a", SemanticScore: 0.91, SemanticQualified: true},
			{ID: "m002", AssetID: "asset-b", ReuseKey: "asset-b", SemanticScore: 0.70, SemanticQualified: false},
		}),
	}}
	preferred := modelgateway.EditPlanResult{Clips: []modelgateway.EditPlanClipChoice{{SlotID: "s001", CandidateID: "m002"}}}

	result, err := selectDiversePlannerResult(input, preferred, map[string]int{"asset-a": 1}, map[string]int{}, map[string]bool{})
	if err != nil {
		t.Fatalf("select qualified result: %v", err)
	}
	if result.Clips[0].CandidateID != "m001" {
		t.Fatalf("expected qualified material reuse before an unrelated fallback, got %#v", result.Clips)
	}
}

func selectionTestRequirement(beatID string, slotID string, startMs int, candidates []modelgateway.EditPlanCandidate) modelgateway.EditPlanRequirement {
	for index := range candidates {
		candidates[index].SourceInMs = 0
		candidates[index].SourceOutMs = 3000
		candidates[index].SourceType = modelgateway.TTSVisualSourceType
	}
	return modelgateway.EditPlanRequirement{
		VisualBeatID:       beatID,
		NarrationSegmentID: "narration-" + beatID,
		StartMs:            startMs,
		EndMs:              startMs + 1000,
		NarrationText:      "展示产品",
		VisualGoal:         "展示产品动作",
		SourceType:         modelgateway.TTSVisualSourceType,
		Slots: []modelgateway.EditPlanSlot{{
			ID:         slotID,
			StartMs:    startMs,
			EndMs:      startMs + 1000,
			DurationMs: 1000,
			Role:       modelgateway.EditPlanSlotRolePrimary,
			Candidates: candidates,
		}},
	}
}
