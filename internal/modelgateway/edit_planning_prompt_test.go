package modelgateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBuildEditPlanPromptDeduplicatesCandidateDetails(t *testing.T) {
	sharedSummary := "人物拉伸束裤带，展示松紧带回弹"
	input := EditPlanInput{
		ProductName: "束裤带",
		ScriptText:  "高弹贴合，收纳方便。",
		Requirements: []EditPlanRequirement{
			{
				DurationClass: VisualDurationClassAction,
				NarrationText: "高弹贴合。",
				Label:         "展示弹力",
				VisualGoal:    "双手拉伸束裤带并展示回弹",
				SourceType:    TTSVisualSourceType,
				Slots: []EditPlanSlot{{
					ID: "s001", DurationMs: 2800, Role: EditPlanSlotRoleActionPrimary,
					Candidates: []EditPlanCandidate{
						{ID: "m001", SourceType: TTSVisualSourceType, SemanticSummary: sharedSummary, SemanticScore: 0.91, BatchUseCount: 1},
						{ID: "m002", SourceType: TTSVisualSourceType, SemanticSummary: "束裤带固定在裤脚处", SemanticScore: 0.78, BatchUseCount: 0},
					},
				}},
			},
			{
				DurationClass: VisualDurationClassStandard,
				NarrationText: "收纳方便。",
				Label:         "方便收纳",
				VisualGoal:    "手将束裤带折叠后放入口袋",
				SourceType:    TTSVisualSourceType,
				Slots: []EditPlanSlot{{
					ID: "s002", DurationMs: 1800, Role: EditPlanSlotRolePrimary,
					Candidates: []EditPlanCandidate{
						{ID: "m001", SourceType: TTSVisualSourceType, SemanticSummary: sharedSummary, SemanticScore: 0.72, BatchUseCount: 1},
						{ID: "m003", SourceType: TTSVisualSourceType, SemanticSummary: "手将束裤带放入口袋", SemanticScore: 0.89, BatchUseCount: 0},
					},
				}},
			},
		},
	}

	compact := buildEditPlanPromptInput(input)
	if len(compact.CandidateOptions) != 3 {
		t.Fatalf("expected three global candidate options, got %d", len(compact.CandidateOptions))
	}
	if got := compact.Requirements[0].CandidateScores["m001"]; got != 0.91 {
		t.Fatalf("expected first requirement score 0.91, got %v", got)
	}
	if got := compact.Requirements[1].CandidateScores["m001"]; got != 0.72 {
		t.Fatalf("expected second requirement score 0.72, got %v", got)
	}
	if got := compact.Requirements[1].Slots[0].AllowedCandidateIDs; len(got) != 2 || got[0] != "m001" || got[1] != "m003" {
		t.Fatalf("unexpected allowed candidate ids %#v", got)
	}

	encoded, err := json.Marshal(compact)
	if err != nil {
		t.Fatalf("marshal compact prompt input: %v", err)
	}
	if strings.Count(string(encoded), sharedSummary) != 1 {
		t.Fatalf("candidate summary must be serialized once: %s", encoded)
	}
	prompt := BuildEditPlanPrompt(input).Prompts[0].User
	if strings.Contains(prompt, `"candidates":`) || !strings.Contains(prompt, `"candidate_options":`) || !strings.Contains(prompt, `"allowed_candidate_ids":`) || !strings.Contains(prompt, "candidate_indexes") {
		t.Fatalf("unexpected edit planner prompt structure: %s", prompt)
	}
}

func TestBuildEditPlanPromptCompactsRepeatedCandidatePayload(t *testing.T) {
	candidates := make([]EditPlanCandidate, 0, 6)
	for index := 1; index <= 6; index++ {
		candidates = append(candidates, EditPlanCandidate{
			ID:              fmt.Sprintf("m%03d", index),
			SourceType:      TTSVisualSourceType,
			SemanticSummary: fmt.Sprintf("素材%d：%s", index, strings.Repeat("展示产品操作动作和使用结果；", 10)),
			SemanticScore:   0.90 - float64(index)/100,
			BatchUseCount:   index % 3,
		})
	}
	input := EditPlanInput{ProductName: "产品", ScriptText: strings.Repeat("口播文案。", 20)}
	for index := 1; index <= 12; index++ {
		input.Requirements = append(input.Requirements, EditPlanRequirement{
			DurationClass: VisualDurationClassAction,
			NarrationText: fmt.Sprintf("第%d段口播", index),
			Label:         fmt.Sprintf("动作%d", index),
			VisualGoal:    fmt.Sprintf("展示第%d个产品动作", index),
			SourceType:    TTSVisualSourceType,
			Slots: []EditPlanSlot{
				{ID: fmt.Sprintf("s%03d", index*2-1), DurationMs: 2800, Role: EditPlanSlotRoleActionPrimary, Candidates: append([]EditPlanCandidate(nil), candidates...)},
				{ID: fmt.Sprintf("s%03d", index*2), DurationMs: 1000, Role: EditPlanSlotRoleSupport, Candidates: append([]EditPlanCandidate(nil), candidates...)},
			},
		})
	}
	fullJSON, err := json.Marshal(map[string]any{
		"product_name": input.ProductName,
		"script_text":  input.ScriptText,
		"requirements": input.Requirements,
	})
	if err != nil {
		t.Fatalf("marshal full prompt input: %v", err)
	}
	compactJSON, err := json.Marshal(buildEditPlanPromptInput(input))
	if err != nil {
		t.Fatalf("marshal compact prompt input: %v", err)
	}
	if len(compactJSON)*2 >= len(fullJSON) {
		t.Fatalf("expected compact payload below half size, full=%d compact=%d", len(fullJSON), len(compactJSON))
	}
}
