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

func TestUpdateAssetReviewInMemory(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})

	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	updated, err := service.UpdateAssetReview(asset.ID, AssetReviewUpdate{
		SceneDescription: "manual description",
		ShotSize:         "close_up",
		CameraMovement:   "static",
		Subjects:         []string{"product", "hand"},
		SceneTags:        []string{"indoor"},
		QualityTags:      []string{"soft_focus"},
		UsabilityStatus:  "needs_review",
		ReviewerNotes:    "needs crop",
		UpdatedByUserID:  "editor-1",
	})
	if err != nil {
		t.Fatalf("update asset review failed: %v", err)
	}

	if updated.SceneDescription != "manual description" {
		t.Fatalf("expected scene description updated, got %s", updated.SceneDescription)
	}
	if updated.UsabilityStatus != "needs_review" {
		t.Fatalf("expected usability_status needs_review, got %s", updated.UsabilityStatus)
	}
	if len(updated.Subjects) != 2 || updated.Subjects[0] != "product" {
		t.Fatalf("expected subjects updated, got %#v", updated.Subjects)
	}
	if updated.ReviewerNotes != "needs crop" {
		t.Fatalf("expected reviewer notes updated, got %s", updated.ReviewerNotes)
	}
	if updated.UpdatedByUserID != "editor-1" {
		t.Fatalf("expected updated_by_user_id editor-1, got %s", updated.UpdatedByUserID)
	}
}

func TestListAssetsSupportsTagDurationAudioAndSellingPointFilters(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	sellingPoint, err := service.CreateSellingPoint(product.ID, CreateSellingPointInput{
		Title:    "Auto Wake",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create selling point failed: %v", err)
	}

	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		DurationMs:        3200,
		HasAudio:          true,
		SceneDescription:  "auto wake product shot",
		SceneTags:         []string{"indoor", "demo"},
		SellingPointIDs:   []string{sellingPoint.ID},
	})
	if err != nil {
		t.Fatalf("create asset 1 failed: %v", err)
	}

	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "b.mp4",
		StorageKey:        "assets/b.mp4",
		SourceType:        "talking_head",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		DurationMs:        900,
		HasAudio:          false,
		SceneDescription:  "speaker intro",
		SceneTags:         []string{"talking_head"},
	})
	if err != nil {
		t.Fatalf("create asset 2 failed: %v", err)
	}

	minDuration := 2000
	hasAudio := true
	filtered := service.ListAssets(AssetFilters{
		SellingPointID: sellingPoint.ID,
		Tag:            "demo",
		MinDurationMs:  &minDuration,
		HasAudio:       &hasAudio,
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered asset, got %d", len(filtered))
	}
	if filtered[0].FileName != "a.mp4" {
		t.Fatalf("expected filtered asset a.mp4, got %s", filtered[0].FileName)
	}
}
