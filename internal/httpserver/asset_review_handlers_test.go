package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestHandleUpdateAssetReview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
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
	if err := productAssetService.UpdateAssetAnalysis(asset.ID, services.AssetAnalysisUpdate{
		AnalysisStatus:   "ready",
		UsabilityStatus:  "usable",
		SceneDescription: "model description",
		ShotSize:         "medium_shot",
		CameraMovement:   "static",
		Subjects:         []string{"product"},
		SceneTags:        []string{"demo"},
		ModelLabels: map[string]any{
			"scene_description":  "model description",
			"action_description": "展示键盘图案设计细节",
			"shot_size":          "medium_shot",
			"camera_movement":    "static",
			"subjects":           []string{"product"},
			"scene_tags":         []string{"demo"},
			"quality_tags":       []string{},
			"usability_status":   "usable",
		},
		ModelResult: map[string]any{
			"provider": "mock",
			"score":    0.92,
		},
		UpdatedByUserID: "analyzer-1",
	}); err != nil {
		t.Fatalf("seed asset analysis failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	body, err := json.Marshal(map[string]any{
		"scene_description":  "manual description",
		"action_description": "人物双手反复拉伸和放松束裤带，展示弹性",
		"shot_size":          "close_up",
		"camera_movement":    "static",
		"subjects":           []string{"product"},
		"scene_tags":         []string{"indoor"},
		"quality_tags":       []string{"soft_focus"},
		"usability_status":   "needs_review",
		"reviewer_notes":     "adjust crop",
	})
	if err != nil {
		t.Fatalf("marshal body failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/assets/"+asset.ID+"/review", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	}))
	recorder := httptest.NewRecorder()

	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	updated, ok := productAssetService.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if updated.SceneDescription != "manual description" {
		t.Fatalf("expected scene description updated, got %s", updated.SceneDescription)
	}
	if updated.ModelLabels["scene_description"] != "model description" {
		t.Fatalf("expected model labels kept separately, got %#v", updated.ModelLabels)
	}
	if updated.ReviewOverrides["scene_description"] != "manual description" {
		t.Fatalf("expected review overrides stored, got %#v", updated.ReviewOverrides)
	}
	if updated.ActionDescription != "人物双手反复拉伸和放松束裤带，展示弹性" {
		t.Fatalf("expected action description updated, got %s", updated.ActionDescription)
	}
	if updated.ModelLabels["action_description"] != "展示键盘图案设计细节" {
		t.Fatalf("expected original model action preserved, got %#v", updated.ModelLabels)
	}
	if updated.ReviewOverrides["action_description"] != "人物双手反复拉伸和放松束裤带，展示弹性" {
		t.Fatalf("expected action override stored, got %#v", updated.ReviewOverrides)
	}
	if updated.ReviewerNotes != "adjust crop" {
		t.Fatalf("expected reviewer notes updated, got %s", updated.ReviewerNotes)
	}
	if updated.UpdatedByUserID != "editor-1" {
		t.Fatalf("expected updated user id editor-1, got %s", updated.UpdatedByUserID)
	}
	if provider, ok := updated.ModelResult["provider"].(string); !ok || provider != "mock" {
		t.Fatalf("expected model result preserved, got %#v", updated.ModelResult)
	}

	var resp struct {
		Data services.Asset `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.Data.ModelLabels["scene_description"] != "model description" {
		t.Fatalf("expected response model labels, got %#v", resp.Data.ModelLabels)
	}
	if resp.Data.ReviewOverrides["scene_description"] != "manual description" {
		t.Fatalf("expected response review overrides, got %#v", resp.Data.ReviewOverrides)
	}
	if resp.Data.ActionDescription != "人物双手反复拉伸和放松束裤带，展示弹性" {
		t.Fatalf("expected response action description, got %s", resp.Data.ActionDescription)
	}
}

func TestHandleArchiveAndRestoreAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
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

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	archiveReq := httptest.NewRequest(http.MethodPost, "/api/assets/"+asset.ID+"/archive", nil)
	archiveReq.Header.Set("Authorization", userToken)
	archiveRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(archiveRecorder, archiveReq)

	if archiveRecorder.Code != http.StatusOK {
		t.Fatalf("expected archive status 200, got %d, body=%s", archiveRecorder.Code, archiveRecorder.Body.String())
	}

	archived, ok := productAssetService.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist after archive")
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %s", archived.Status)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("expected archived_at to be set")
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/assets/"+asset.ID+"/restore", nil)
	restoreReq.Header.Set("Authorization", userToken)
	restoreRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(restoreRecorder, restoreReq)

	if restoreRecorder.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d, body=%s", restoreRecorder.Code, restoreRecorder.Body.String())
	}

	restored, ok := productAssetService.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist after restore")
	}
	if restored.Status != "ready" {
		t.Fatalf("expected ready status after restore, got %s", restored.Status)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("expected archived_at cleared after restore")
	}
}

func TestHandleReanalyzeAssetQueuesNineFramesAndPreservesReview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storageRoot := t.TempDir()
	productAssetService := services.NewProductAssetService()
	taskService := services.NewTaskService(storageRoot)
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		DurationMs:        4920,
		CreatedByUserID:   "editor-1",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}
	if err := productAssetService.UpdateAssetAnalysis(asset.ID, services.AssetAnalysisUpdate{
		AnalysisStatus: "ready",
		ModelLabels: map[string]any{
			"scene_description":  "模型描述",
			"action_description": "模型动作",
			"usability_status":   "usable",
		},
		ModelResult: map[string]any{"prompt_version": "phase2-v6"},
	}); err != nil {
		t.Fatalf("seed analysis failed: %v", err)
	}
	if _, err := productAssetService.UpdateAssetReview(asset.ID, services.AssetReviewUpdate{
		SceneDescription:  "人工描述",
		ActionDescription: "人工动作",
		UsabilityStatus:   "usable",
		UpdatedByUserID:   "editor-1",
	}); err != nil {
		t.Fatalf("seed review failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: storageRoot, QueueBackend: "file"},
		ProductAssetService: productAssetService,
		TaskService:         taskService,
	})
	defer server.queueClient.Close()
	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/assets/"+asset.ID+"/reanalyze", nil)
	req.Header.Set("Authorization", userToken)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			Asset       services.Asset `json:"asset"`
			FrameTaskID string         `json:"frame_task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.Data.FrameTaskID == "" || resp.Data.Asset.AnalysisStatus != "pending_analysis" {
		t.Fatalf("expected queued frame task and pending asset, got %#v", resp.Data)
	}
	if resp.Data.Asset.SceneDescription != "人工描述" || resp.Data.Asset.ModelLabels["scene_description"] != "模型描述" {
		t.Fatalf("expected review and prior model labels to be preserved, got %#v", resp.Data.Asset)
	}

	tasks, err := taskService.ListTasks(t.Context(), services.TaskFilters{TaskType: "asset_extract_frames"})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("expected one frame extraction task, got %#v err=%v", tasks, err)
	}
	frameCountIsNine := false
	switch frameCount := tasks[0].PayloadSummary["frame_count"].(type) {
	case int:
		frameCountIsNine = frameCount == 9
	case float64:
		frameCountIsNine = frameCount == 9
	}
	if !frameCountIsNine {
		t.Fatalf("expected fixed nine-frame extraction, got %#v", tasks[0].PayloadSummary)
	}
	if skipAnalyze, _ := tasks[0].PayloadSummary["skip_analyze"].(bool); skipAnalyze {
		t.Fatalf("expected reanalysis extraction to continue to VLM, got %#v", tasks[0].PayloadSummary)
	}
}

func TestHandleListAssetSelectionAndBulkArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	createAsset := func(fileName string) services.Asset {
		asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
			ProductID:         product.ID,
			FileName:          fileName,
			StorageKey:        "assets/" + fileName,
			SourceType:        "visual_only",
			Status:            "ready",
			AnalysisStatus:    "ready",
			UsabilityStatus:   "usable",
			ManualCleanStatus: "cleaned",
		})
		if err != nil {
			t.Fatalf("create asset %s failed: %v", fileName, err)
		}
		return asset
	}
	assetA := createAsset("a.mp4")
	assetB := createAsset("b.mp4")
	assetC := createAsset("c.mp4")
	if _, err := productAssetService.ArchiveAsset(assetC.ID, services.AssetArchiveUpdate{UpdatedByUserID: "editor-1"}); err != nil {
		t.Fatalf("archive fixture asset: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})
	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	selectionReq := httptest.NewRequest(http.MethodGet, "/api/assets/selection?product_id="+product.ID, nil)
	selectionReq.Header.Set("Authorization", userToken)
	selectionRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(selectionRecorder, selectionReq)
	if selectionRecorder.Code != http.StatusOK {
		t.Fatalf("expected selection status 200, got %d, body=%s", selectionRecorder.Code, selectionRecorder.Body.String())
	}
	var selectionResp struct {
		Data struct {
			AssetIDs []string `json:"asset_ids"`
			Total    int      `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(selectionRecorder.Body.Bytes(), &selectionResp); err != nil {
		t.Fatalf("unmarshal selection response: %v", err)
	}
	if selectionResp.Data.Total != 2 || len(selectionResp.Data.AssetIDs) != 2 {
		t.Fatalf("expected two archivable assets, got %#v", selectionResp.Data)
	}

	body, err := json.Marshal(map[string]any{
		"asset_ids": []string{assetA.ID, assetB.ID, assetC.ID, "missing-asset", assetA.ID},
	})
	if err != nil {
		t.Fatalf("marshal bulk archive body: %v", err)
	}
	archiveReq := httptest.NewRequest(http.MethodPost, "/api/assets/bulk-archive", bytes.NewReader(body))
	archiveReq.Header.Set("Content-Type", "application/json")
	archiveReq.Header.Set("Authorization", userToken)
	archiveRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(archiveRecorder, archiveReq)
	if archiveRecorder.Code != http.StatusOK {
		t.Fatalf("expected bulk archive status 200, got %d, body=%s", archiveRecorder.Code, archiveRecorder.Body.String())
	}
	var archiveResp struct {
		Data services.AssetBulkArchiveResult `json:"data"`
	}
	if err := json.Unmarshal(archiveRecorder.Body.Bytes(), &archiveResp); err != nil {
		t.Fatalf("unmarshal bulk archive response: %v", err)
	}
	if len(archiveResp.Data.Archived) != 2 || len(archiveResp.Data.SkippedIDs) != 1 || len(archiveResp.Data.Failures) != 1 {
		t.Fatalf("unexpected bulk archive result %#v", archiveResp.Data)
	}
	for _, assetID := range []string{assetA.ID, assetB.ID, assetC.ID} {
		asset, ok := productAssetService.GetAsset(assetID)
		if !ok || asset.Status != "archived" {
			t.Fatalf("expected asset %s archived, got %#v", assetID, asset)
		}
	}
}

func TestHandleListAndUpdateAssetSellingPoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	sellingPoint1, err := productAssetService.CreateSellingPoint(product.ID, services.CreateSellingPointInput{
		Title:    "Auto Wake",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create selling point 1 failed: %v", err)
	}
	sellingPoint2, err := productAssetService.CreateSellingPoint(product.ID, services.CreateSellingPointInput{
		Title:    "Battery Saver",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create selling point 2 failed: %v", err)
	}
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "a.mp4",
		StorageKey:        "assets/a.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SellingPointIDs:   []string{sellingPoint1.ID},
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/assets/"+asset.ID+"/selling-points", nil)
	listReq.Header.Set("Authorization", userToken)
	listRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(listRecorder, listReq)

	if listRecorder.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d, body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	var listResp struct {
		Data []services.SellingPoint `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != sellingPoint1.ID {
		t.Fatalf("expected sellingPoint1 in list response, got %#v", listResp.Data)
	}

	body, err := json.Marshal(map[string]any{
		"selling_point_ids": []string{sellingPoint2.ID},
	})
	if err != nil {
		t.Fatalf("marshal body failed: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/assets/"+asset.ID+"/selling-points", bytes.NewReader(body))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", userToken)
	updateRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(updateRecorder, updateReq)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d, body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	updated, err := productAssetService.ListAssetSellingPoints(asset.ID)
	if err != nil {
		t.Fatalf("list asset selling points after update failed: %v", err)
	}
	if len(updated) != 1 || updated[0].ID != sellingPoint2.ID {
		t.Fatalf("expected sellingPoint2 after update, got %#v", updated)
	}
}

func TestHandleListAssetSpeechSegments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "talk.mp4",
		StorageKey:        "assets/talk.mp4",
		SourceType:        "talking_head",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}
	if _, err := productAssetService.CreateSpeechSegment(services.CreateSpeechSegmentInput{
		AssetID:         asset.ID,
		StartMs:         0,
		EndMs:           1200,
		Transcript:      "第一句",
		Source:          "local-agent",
		Status:          "ready",
		CreatedByUserID: "editor-1",
	}); err != nil {
		t.Fatalf("create speech segment failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/assets/"+asset.ID+"/speech-segments", nil)
	req.Header.Set("Authorization", userToken)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data []services.SpeechSegment `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Transcript != "第一句" {
		t.Fatalf("expected persisted speech segment, got %#v", resp.Data)
	}
}

func TestHandleGetAssetSemanticPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "talk.mp4",
		StorageKey:        "assets/talk.mp4",
		SourceType:        "talking_head",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SceneDescription:  "主持人介绍产品",
		ShotSize:          "medium_close_up",
		CameraMovement:    "static",
		LikelyHasSpeech:   true,
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}
	if _, err := productAssetService.CreateSpeechSegment(services.CreateSpeechSegmentInput{
		AssetID:         asset.ID,
		StartMs:         0,
		EndMs:           1500,
		Transcript:      "第一句口播",
		Source:          "local-agent",
		Status:          "ready",
		CreatedByUserID: "editor-1",
	}); err != nil {
		t.Fatalf("create speech segment failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/assets/"+asset.ID+"/semantic-preview", nil)
	req.Header.Set("Authorization", userToken)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data services.AssetSemanticPreview `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.Data.OpenSemanticDescription == "" {
		t.Fatalf("expected open semantic description")
	}
	if len(resp.Data.EmbeddingTargets) != 2 {
		t.Fatalf("expected shot + speech segment targets, got %#v", resp.Data.EmbeddingTargets)
	}
}

func TestHandleUpdateAssetBusinessTags(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	asset, err := productAssetService.CreateAsset(services.CreateAssetInput{
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

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	body, err := json.Marshal(map[string]any{
		"is_curated":      true,
		"business_tags":   []string{"首镜优先", "核心卖点"},
		"narrative_roles": []string{"开头钩子"},
		"usage_notes":     "优先用于点击率测试",
	})
	if err != nil {
		t.Fatalf("marshal body failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/assets/"+asset.ID+"/business-tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	}))
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	updated, ok := productAssetService.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if curated, ok := updated.Metadata["is_curated"].(bool); !ok || !curated {
		t.Fatalf("expected curated metadata, got %#v", updated.Metadata)
	}
	if note := updated.Metadata["usage_notes"]; note != "优先用于点击率测试" {
		t.Fatalf("expected usage note persisted, got %#v", updated.Metadata)
	}
}
