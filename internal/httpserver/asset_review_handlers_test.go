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
		"scene_description": "manual description",
		"shot_size":         "close_up",
		"camera_movement":   "static",
		"subjects":          []string{"product"},
		"scene_tags":        []string{"indoor"},
		"quality_tags":      []string{"soft_focus"},
		"usability_status":  "needs_review",
		"reviewer_notes":    "adjust crop",
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
