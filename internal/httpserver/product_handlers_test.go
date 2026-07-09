package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestHandleGetProductStatsAndSellingPointAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	sellingPoint, err := productAssetService.CreateSellingPoint(product.ID, services.CreateSellingPointInput{
		Title:    "Auto Wake",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create selling point failed: %v", err)
	}

	_, err = productAssetService.CreateAsset(services.CreateAssetInput{
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

	_, err = productAssetService.CreateAsset(services.CreateAssetInput{
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

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	token := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	statsReq := httptest.NewRequest(http.MethodGet, "/api/products/"+product.ID+"/stats", nil)
	statsReq.Header.Set("Authorization", token)
	statsRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(statsRecorder, statsReq)

	if statsRecorder.Code != http.StatusOK {
		t.Fatalf("expected stats status 200, got %d, body=%s", statsRecorder.Code, statsRecorder.Body.String())
	}

	var statsResp struct {
		Data services.ProductAssetStats `json:"data"`
	}
	if err := json.Unmarshal(statsRecorder.Body.Bytes(), &statsResp); err != nil {
		t.Fatalf("unmarshal stats response failed: %v", err)
	}
	if statsResp.Data.AssetCount != 2 {
		t.Fatalf("expected asset count 2, got %d", statsResp.Data.AssetCount)
	}
	if statsResp.Data.UsableAssetCount != 1 {
		t.Fatalf("expected usable asset count 1, got %d", statsResp.Data.UsableAssetCount)
	}
	if statsResp.Data.PendingAnalysisCount != 1 {
		t.Fatalf("expected pending analysis count 1, got %d", statsResp.Data.PendingAnalysisCount)
	}

	assetsReq := httptest.NewRequest(http.MethodGet, "/api/selling-points/"+sellingPoint.ID+"/assets", nil)
	assetsReq.Header.Set("Authorization", token)
	assetsRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(assetsRecorder, assetsReq)

	if assetsRecorder.Code != http.StatusOK {
		t.Fatalf("expected selling point assets status 200, got %d, body=%s", assetsRecorder.Code, assetsRecorder.Body.String())
	}

	var assetsResp struct {
		Data []services.Asset `json:"data"`
	}
	if err := json.Unmarshal(assetsRecorder.Body.Bytes(), &assetsResp); err != nil {
		t.Fatalf("unmarshal assets response failed: %v", err)
	}
	if len(assetsResp.Data) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assetsResp.Data))
	}
}
