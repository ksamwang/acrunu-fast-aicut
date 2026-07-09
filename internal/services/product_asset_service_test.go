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

func TestArchiveAndRestoreAssetInMemory(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})

	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "failed",
		UsabilityStatus:   "needs_review",
		ManualCleanStatus: "cleaned",
		AnalysisError:     "mock provider failed",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	archived, err := service.ArchiveAsset(asset.ID, AssetArchiveUpdate{UpdatedByUserID: "editor-1"})
	if err != nil {
		t.Fatalf("archive asset failed: %v", err)
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %s", archived.Status)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("expected archived_at set")
	}

	restored, err := service.RestoreAsset(asset.ID, AssetArchiveUpdate{UpdatedByUserID: "editor-2"})
	if err != nil {
		t.Fatalf("restore asset failed: %v", err)
	}
	if restored.Status != "ready" {
		t.Fatalf("expected ready status, got %s", restored.Status)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("expected archived_at cleared")
	}
	if restored.UpdatedByUserID != "editor-2" {
		t.Fatalf("expected updated_by_user_id editor-2, got %s", restored.UpdatedByUserID)
	}
}

func TestListAndUpdateAssetSellingPointsInMemory(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	sellingPoint1, err := service.CreateSellingPoint(product.ID, CreateSellingPointInput{
		Title:    "Auto Wake",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create selling point 1 failed: %v", err)
	}
	sellingPoint2, err := service.CreateSellingPoint(product.ID, CreateSellingPointInput{
		Title:    "Battery Saver",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create selling point 2 failed: %v", err)
	}
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SellingPointIDs:   []string{sellingPoint1.ID},
		CreatedByUserID:   "creator-1",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	items, err := service.ListAssetSellingPoints(asset.ID)
	if err != nil {
		t.Fatalf("list asset selling points failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != sellingPoint1.ID {
		t.Fatalf("expected single initial selling point, got %#v", items)
	}

	updated, err := service.UpdateAssetSellingPoints(asset.ID, AssetSellingPointsUpdate{
		SellingPointIDs: []string{sellingPoint2.ID, sellingPoint2.ID},
		UpdatedByUserID: "editor-1",
	})
	if err != nil {
		t.Fatalf("update asset selling points failed: %v", err)
	}
	if len(updated) != 1 || updated[0].ID != sellingPoint2.ID {
		t.Fatalf("expected updated selling points to contain sellingPoint2 once, got %#v", updated)
	}

	reloaded, ok := service.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist after selling point update")
	}
	if reloaded.UpdatedByUserID != "editor-1" {
		t.Fatalf("expected updated_by_user_id editor-1, got %s", reloaded.UpdatedByUserID)
	}
}

func TestUpdateAssetSellingPointsRejectsWrongProductSellingPoint(t *testing.T) {
	service := NewProductAssetService()
	product1 := service.CreateProduct(CreateProductInput{Name: "P1"})
	product2 := service.CreateProduct(CreateProductInput{Name: "P2"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product1.ID,
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
	foreignSellingPoint, err := service.CreateSellingPoint(product2.ID, CreateSellingPointInput{
		Title:    "Foreign",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create foreign selling point failed: %v", err)
	}

	_, err = service.UpdateAssetSellingPoints(asset.ID, AssetSellingPointsUpdate{
		SellingPointIDs: []string{foreignSellingPoint.ID},
		UpdatedByUserID: "editor-1",
	})
	if err == nil {
		t.Fatalf("expected wrong product selling point to be rejected")
	}
	if err != ErrSellingPointNotFound {
		t.Fatalf("expected ErrSellingPointNotFound, got %v", err)
	}
}

func TestGetProductAssetStatsAndSellingPointAssetsInMemory(t *testing.T) {
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
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "needs_review",
		ManualCleanStatus: "cleaned",
		SellingPointIDs:   []string{sellingPoint.ID},
	})
	if err != nil {
		t.Fatalf("create asset 2 failed: %v", err)
	}

	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "c.mp4",
		StorageKey:        "assets/c.mp4",
		SourceType:        "visual_only",
		Status:            "archived",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SellingPointIDs:   []string{sellingPoint.ID},
	})
	if err != nil {
		t.Fatalf("create asset 3 failed: %v", err)
	}

	stats, err := service.GetProductAssetStats(product.ID)
	if err != nil {
		t.Fatalf("get product stats failed: %v", err)
	}
	if stats.AssetCount != 2 {
		t.Fatalf("expected asset count 2, got %d", stats.AssetCount)
	}
	if stats.UsableAssetCount != 1 {
		t.Fatalf("expected usable asset count 1, got %d", stats.UsableAssetCount)
	}
	if stats.PendingAnalysisCount != 1 {
		t.Fatalf("expected pending analysis count 1, got %d", stats.PendingAnalysisCount)
	}

	sellingPoints := service.ListSellingPoints(product.ID)
	if len(sellingPoints) != 1 || sellingPoints[0].AssetCount != 2 {
		t.Fatalf("expected selling point asset count 2, got %#v", sellingPoints)
	}

	assets, err := service.ListAssetsBySellingPoint(sellingPoint.ID)
	if err != nil {
		t.Fatalf("list assets by selling point failed: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 non-archived assets, got %d", len(assets))
	}
}
