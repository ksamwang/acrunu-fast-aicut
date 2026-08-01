package services

import (
	"math"
	"strings"
	"testing"
)

func TestProjectSemanticEmbeddingsIsDeterministicAndBounded(t *testing.T) {
	sources := []semanticProjectionSource{
		{AssetID: "a", Embedding: []float32{1, 0, 0, 0, 0, 0}},
		{AssetID: "b", Embedding: []float32{0.9, 0.1, 0, 0, 0, 0}},
		{AssetID: "c", Embedding: []float32{0, 0, 1, 0, 0, 0}},
		{AssetID: "d", Embedding: []float32{0, 0, 0.9, 0.1, 0, 0}},
		{AssetID: "e", Embedding: []float32{0, 0, 0, 0, 1, 0}},
	}

	first, err := projectSemanticEmbeddings(sources)
	if err != nil {
		t.Fatalf("project embeddings failed: %v", err)
	}
	second, err := projectSemanticEmbeddings(sources)
	if err != nil {
		t.Fatalf("project embeddings again failed: %v", err)
	}
	if len(first) != len(sources) || len(second) != len(sources) {
		t.Fatalf("unexpected projection sizes: %d and %d", len(first), len(second))
	}
	for index := range first {
		left := first[index]
		right := second[index]
		if left != right {
			t.Fatalf("projection is not deterministic at %d: %+v != %+v", index, left, right)
		}
		for _, value := range []float64{left.X2, left.Y2, left.X3, left.Y3, left.Z3} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
				t.Fatalf("projection coordinate is invalid: %v", value)
			}
		}
		if left.X2 != left.X3 || left.Y2 != left.Y3 {
			t.Fatalf("2D projection does not use the first two PCA components: %+v", left)
		}
	}
	if first[0].X2 == first[2].X2 && first[0].Y2 == first[2].Y2 {
		t.Fatalf("distinct semantic sources collapsed to the same coordinate")
	}
}

func TestProjectSemanticEmbeddingsHandlesSinglePoint(t *testing.T) {
	points, err := projectSemanticEmbeddings([]semanticProjectionSource{{AssetID: "only", Embedding: []float32{1, 2, 3}}})
	if err != nil {
		t.Fatalf("project single embedding failed: %v", err)
	}
	if len(points) != 1 || points[0].X2 != 0 || points[0].Y2 != 0 || points[0].Z3 != 0 {
		t.Fatalf("unexpected single point projection: %+v", points)
	}
}

func TestBuildSemanticAssetFilterConditionsSupportsProjectionAlias(t *testing.T) {
	minimumDuration := 900
	conditions, args := buildSemanticAssetFilterConditions("asset", AssetFilters{
		ProductID:     "3bd5219b-00c0-42d2-8daf-ce157fb523e3",
		Tag:           "拉伸",
		MinDurationMs: &minimumDuration,
	}, []any{"projection-id"})
	joined := strings.Join(conditions, "\n")
	for _, expected := range []string{"asset.product_id = $2::uuid", "asset.scene_description", "asset.duration_ms"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in %s", expected, joined)
		}
	}
	if len(args) != 5 {
		t.Fatalf("expected five arguments including the projection id, got %#v", args)
	}
}

func TestEffectiveAssetActionDescriptionSQLMergesReviewOverrides(t *testing.T) {
	expression := effectiveAssetActionDescriptionSQL("asset")
	if strings.Contains(expression, "asset.action_description") {
		t.Fatalf("effective action expression references a non-existent column: %s", expression)
	}
	for _, expected := range []string{"asset.model_labels", "asset.review_overrides", "->> 'action_description'"} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("expected %q in %s", expected, expression)
		}
	}
}
