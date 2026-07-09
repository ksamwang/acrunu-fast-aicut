package services

import (
	"testing"
	"time"
)

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

	if err := service.UpdateAssetAnalysis(asset.ID, AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "model description",
		ShotSize:         "medium_shot",
		CameraMovement:   "static",
		Subjects:         []string{"product"},
		SceneTags:        []string{"demo"},
		QualityTags:      []string{},
		ModelResult: map[string]any{
			"provider": "mock",
			"score":    0.92,
		},
		AnalyzedAt:      time.Now(),
		UpdatedByUserID: "analyzer-1",
	}); err != nil {
		t.Fatalf("seed asset analysis failed: %v", err)
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
	if updated.ModelLabels["scene_description"] != "model description" {
		t.Fatalf("expected model labels preserved, got %#v", updated.ModelLabels)
	}
	if updated.ReviewOverrides["scene_description"] != "manual description" {
		t.Fatalf("expected review overrides stored, got %#v", updated.ReviewOverrides)
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
	if provider, ok := updated.ModelResult["provider"].(string); !ok || provider != "mock" {
		t.Fatalf("expected model result provider preserved, got %#v", updated.ModelResult)
	}
}

func TestUpdateAssetAnalysisReappliesReviewOverridesInMemory(t *testing.T) {
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

	if err := service.UpdateAssetAnalysis(asset.ID, AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "model v1",
		ShotSize:         "medium_shot",
		CameraMovement:   "static",
		Subjects:         []string{"product"},
		SceneTags:        []string{"demo"},
		QualityTags:      []string{},
		ModelResult:      map[string]any{"provider": "mock", "version": 1},
		UpdatedByUserID:  "analyzer-1",
	}); err != nil {
		t.Fatalf("seed analysis failed: %v", err)
	}

	if _, err := service.UpdateAssetReview(asset.ID, AssetReviewUpdate{
		SceneDescription: "manual final",
		ShotSize:         "close_up",
		CameraMovement:   "static",
		Subjects:         []string{"product", "hand"},
		SceneTags:        []string{"indoor"},
		QualityTags:      []string{"soft_focus"},
		UsabilityStatus:  "needs_review",
		ReviewerNotes:    "keep manual choice",
		UpdatedByUserID:  "editor-1",
	}); err != nil {
		t.Fatalf("review update failed: %v", err)
	}

	if err := service.UpdateAssetAnalysis(asset.ID, AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "model v2",
		ShotSize:         "wide_shot",
		CameraMovement:   "pan",
		Subjects:         []string{"vehicle"},
		SceneTags:        []string{"outdoor"},
		QualityTags:      []string{"noise"},
		ModelResult:      map[string]any{"provider": "mock", "version": 2},
		UpdatedByUserID:  "analyzer-2",
	}); err != nil {
		t.Fatalf("re-analysis failed: %v", err)
	}

	reloaded, ok := service.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if reloaded.SceneDescription != "manual final" || reloaded.ShotSize != "close_up" {
		t.Fatalf("expected manual overrides to stay effective, got %#v", reloaded)
	}
	if reloaded.ModelLabels["scene_description"] != "model v2" {
		t.Fatalf("expected model labels refreshed, got %#v", reloaded.ModelLabels)
	}
	if reloaded.ReviewOverrides["scene_description"] != "manual final" {
		t.Fatalf("expected review overrides retained, got %#v", reloaded.ReviewOverrides)
	}
	if got, ok := reloaded.ModelResult["version"].(int); !ok || got != 2 {
		t.Fatalf("expected latest model result retained, got %#v", reloaded.ModelResult)
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

func TestListSpeechSegmentsByAssetInMemory(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "talking_head",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	if _, err := service.CreateSpeechSegment(CreateSpeechSegmentInput{
		AssetID:         asset.ID,
		StartMs:         3000,
		EndMs:           4500,
		Transcript:      "第二句",
		Source:          "local-agent",
		Status:          "ready",
		CreatedByUserID: "editor-1",
	}); err != nil {
		t.Fatalf("create speech segment 1 failed: %v", err)
	}
	if _, err := service.CreateSpeechSegment(CreateSpeechSegmentInput{
		AssetID:         asset.ID,
		StartMs:         0,
		EndMs:           2000,
		Transcript:      "第一句",
		Source:          "local-agent",
		Status:          "ready",
		CreatedByUserID: "editor-1",
	}); err != nil {
		t.Fatalf("create speech segment 2 failed: %v", err)
	}

	items, err := service.ListSpeechSegmentsByAsset(asset.ID)
	if err != nil {
		t.Fatalf("list speech segments failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 speech segments, got %d", len(items))
	}
	if items[0].Transcript != "第一句" || items[1].Transcript != "第二句" {
		t.Fatalf("expected speech segments ordered by start time, got %#v", items)
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

func TestListAssetsSupportsExcludeDiscardedKeywordAndSorting(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	assetA, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SceneDescription:  "stable product close-up",
	})
	if err != nil {
		t.Fatalf("create asset a failed: %v", err)
	}
	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "b.mp4",
		StorageKey:        "assets/b.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "discarded",
		ManualCleanStatus: "cleaned",
		SceneDescription:  "speaker intro clip",
	})
	if err != nil {
		t.Fatalf("create asset b failed: %v", err)
	}
	assetC, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "c.mp4",
		StorageKey:        "assets/c.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SceneDescription:  "garage walkaround",
	})
	if err != nil {
		t.Fatalf("create asset c failed: %v", err)
	}

	if _, err := service.UpdateAssetReview(assetA.ID, AssetReviewUpdate{
		SceneDescription: assetA.SceneDescription,
		UpdatedByUserID:  "editor-a",
	}); err != nil {
		t.Fatalf("update asset a review failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := service.UpdateAssetReview(assetC.ID, AssetReviewUpdate{
		SceneDescription: assetC.SceneDescription,
		UpdatedByUserID:  "editor-c",
	}); err != nil {
		t.Fatalf("update asset c review failed: %v", err)
	}

	now := time.Now()
	if err := service.UpdateAssetAnalysis(assetA.ID, AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "stable product close-up",
		AnalyzedAt:       now.Add(-2 * time.Minute),
		UpdatedByUserID:  "analyzer-a",
	}); err != nil {
		t.Fatalf("update asset a analysis failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := service.UpdateAssetAnalysis(assetC.ID, AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "garage walkaround",
		AnalyzedAt:       now.Add(-1 * time.Minute),
		UpdatedByUserID:  "analyzer-c",
	}); err != nil {
		t.Fatalf("update asset c analysis failed: %v", err)
	}

	filtered := service.ListAssets(AssetFilters{
		ExcludeDiscarded: true,
		Keyword:          "stable product",
	})
	if len(filtered) != 1 || filtered[0].ID != assetA.ID {
		t.Fatalf("expected only asset a after keyword + exclude filter, got %#v", filtered)
	}

	updatedSorted := service.ListAssets(AssetFilters{
		SortBy:           "updated_at_desc",
		ExcludeDiscarded: true,
	})
	if len(updatedSorted) < 2 || updatedSorted[0].ID != assetC.ID {
		t.Fatalf("expected asset c first by updated_at_desc, got %#v", updatedSorted)
	}

	analyzedSorted := service.ListAssets(AssetFilters{SortBy: "analyzed_at_desc"})
	if len(analyzedSorted) < 2 || analyzedSorted[0].ID != assetC.ID {
		t.Fatalf("expected asset c first by analyzed_at_desc, got %#v", analyzedSorted)
	}
}

func TestFindDuplicateAssetsByChecksum(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	assetA, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		Checksum:          "same-hash",
	})
	if err != nil {
		t.Fatalf("create asset a failed: %v", err)
	}
	assetB, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "b.mp4",
		StorageKey:        "assets/b.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		Checksum:          "same-hash",
	})
	if err != nil {
		t.Fatalf("create asset b failed: %v", err)
	}
	_, err = service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "c.mp4",
		StorageKey:        "assets/c.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		Checksum:          "other-hash",
	})
	if err != nil {
		t.Fatalf("create asset c failed: %v", err)
	}

	duplicates := service.FindDuplicateAssetsByChecksum("same-hash", assetB.ID)
	if len(duplicates) != 1 {
		t.Fatalf("expected one duplicate after excluding current asset, got %d", len(duplicates))
	}
	if duplicates[0].ID != assetA.ID {
		t.Fatalf("expected asset a duplicate, got %#v", duplicates[0])
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
