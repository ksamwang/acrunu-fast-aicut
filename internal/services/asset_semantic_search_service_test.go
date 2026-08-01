package services

import (
	"strings"
	"testing"
)

func TestBuildSemanticAssetSearchConditionsKeepsStructuredFilters(t *testing.T) {
	minimumDuration := 1200
	hasAudio := true
	conditions, args := buildSemanticAssetSearchConditions(semanticAssetSearchStoreInput{
		ProviderID: "1b59ca45-6cda-42db-a844-427510a78a29",
		Model:      indexedEmbeddingModel,
		Dimension:  indexedEmbeddingDimension,
		Filters: AssetFilters{
			ProductID:        "3bd5219b-00c0-42d2-8daf-ce157fb523e3",
			SourceType:       "visual_only",
			SellingPointID:   "ee6ce252-8dee-4e50-8b86-ee97994659d4",
			Tag:              "魔术贴",
			MinDurationMs:    &minimumDuration,
			HasAudio:         &hasAudio,
			ExcludeDiscarded: true,
		},
	}, nil)
	joined := strings.Join(conditions, "\n")
	for _, expected := range []string{
		"e.object_type = 'shot'",
		"a.product_id = $4::uuid",
		"a.source_type = $5",
		"asset_selling_points",
		"COALESCE(a.duration_ms, 0) >=",
		"a.has_audio =",
		"a.usability_status <> 'discarded'",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected condition %q in %s", expected, joined)
		}
	}
	if len(args) != 10 {
		t.Fatalf("expected 10 bound arguments, got %#v", args)
	}
}

func TestVectorCosineDistanceSQLUsesDefaultHNSWExpression(t *testing.T) {
	indexed := vectorCosineDistanceSQL("e.embedding", "$1", indexedEmbeddingModel, indexedEmbeddingDimension)
	if indexed != "e.embedding::vector(1024) <=> $1::vector(1024)" {
		t.Fatalf("unexpected indexed expression %q", indexed)
	}
	fallback := vectorCosineDistanceSQL("e.embedding", "$1", "other-model", 768)
	if fallback != "e.embedding <=> $1::vector" {
		t.Fatalf("unexpected fallback expression %q", fallback)
	}
}

func TestSemanticAssetKeywordUsesEffectiveActionDescription(t *testing.T) {
	conditions, args := buildSemanticAssetFilterConditions("a", AssetFilters{Keyword: "拉伸"}, nil)
	joined := strings.Join(conditions, "\n")
	if strings.Contains(joined, "a.action_description") {
		t.Fatalf("keyword filter references a non-existent action_description column: %s", joined)
	}
	for _, expected := range []string{"a.model_labels", "a.review_overrides", "->> 'action_description'"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected effective action expression %q in %s", expected, joined)
		}
	}
	if len(args) != 1 || args[0] != "%拉伸%" {
		t.Fatalf("unexpected keyword arguments: %#v", args)
	}
}
