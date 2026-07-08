package services

import "testing"

func TestListAssetsAppliesFiltersInMemory(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})

	_, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset 1 failed: %v", err)
	}

	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "b.mp4",
		StorageKey:        "assets/b.mp4",
		SourceType:        "talking_head",
		Status:            "failed",
		AnalysisStatus:    "failed",
		UsabilityStatus:   "needs_review",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset 2 failed: %v", err)
	}

	filtered := service.ListAssets(AssetFilters{
		SourceType:      "talking_head",
		Status:          "failed",
		AnalysisStatus:  "failed",
		UsabilityStatus: "needs_review",
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered asset, got %d", len(filtered))
	}
	if filtered[0].FileName != "b.mp4" {
		t.Fatalf("expected filtered asset b.mp4, got %s", filtered[0].FileName)
	}
}
