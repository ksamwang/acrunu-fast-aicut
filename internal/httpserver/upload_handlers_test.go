package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestSplitCommaSeparated(t *testing.T) {
	got := splitCommaSeparated("sp-1, sp-2 ,,sp-3")
	want := []string{"sp-1", "sp-2", "sp-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestFirstNonEmptyForm(t *testing.T) {
	if got := firstNonEmptyForm("", "cleaned"); got != "cleaned" {
		t.Fatalf("expected fallback cleaned, got %s", got)
	}
	if got := firstNonEmptyForm("manual", "cleaned"); got != "manual" {
		t.Fatalf("expected manual, got %s", got)
	}
}

func TestParseOptionalInt(t *testing.T) {
	if got := parseOptionalInt(""); got != nil {
		t.Fatalf("expected nil for empty value, got %v", *got)
	}
	if got := parseOptionalInt("1200"); got == nil || *got != 1200 {
		t.Fatalf("expected 1200, got %#v", got)
	}
}

func TestParseOptionalBool(t *testing.T) {
	if got := parseOptionalBool(""); got != nil {
		t.Fatalf("expected nil for empty value, got %v", *got)
	}
	if got := parseOptionalBool("true"); got == nil || !*got {
		t.Fatalf("expected true, got %#v", got)
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := parsePositiveInt("", 20); got != 20 {
		t.Fatalf("expected fallback 20, got %d", got)
	}
	if got := parsePositiveInt("0", 20); got != 20 {
		t.Fatalf("expected fallback for zero, got %d", got)
	}
	if got := parsePositiveInt("3", 20); got != 3 {
		t.Fatalf("expected parsed value 3, got %d", got)
	}
}

func TestHandleListAssetsSupportsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	productAssetService := services.NewProductAssetService()
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	for _, fileName := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		_, err := productAssetService.CreateAsset(services.CreateAssetInput{
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
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: productAssetService,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/assets?page=2&page_size=1", nil)
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

	var response struct {
		Data assetListResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}

	if response.Data.Total != 3 {
		t.Fatalf("expected total 3, got %d", response.Data.Total)
	}
	if response.Data.Page != 2 || response.Data.PageSize != 1 {
		t.Fatalf("expected page=2 page_size=1, got %+v", response.Data)
	}
	if len(response.Data.Items) != 1 {
		t.Fatalf("expected 1 paged item, got %d", len(response.Data.Items))
	}
}
