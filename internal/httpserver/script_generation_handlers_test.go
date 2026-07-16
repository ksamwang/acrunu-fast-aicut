package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type scriptGenerationHandlerGenerator struct{}

func (scriptGenerationHandlerGenerator) GenerateScripts(_ context.Context, input modelgateway.ScriptGenerationInput) (modelgateway.ScriptGenerationResult, error) {
	point := input.SellingPoints[0].Name
	return modelgateway.ScriptGenerationResult{Variants: []modelgateway.ScriptGenerationVariant{{
		Hook:          "解决骑行小麻烦",
		ScriptText:    "骑行时，裤脚总会蹭到链条。轻轻一贴，固定更稳，出发更安心。",
		EditingIntent: "从骑行痛点切入，再展示固定动作和结果。",
		Beats: []modelgateway.ScriptGenerationBeat{
			{Label: "开头", SellingPoint: point, VisualGoal: "展示裤脚靠近链条。", SourceType: "mixed"},
			{Label: "展示", SellingPoint: point, VisualGoal: "展示贴合固定动作。", SourceType: "visual_only"},
			{Label: "收束", SellingPoint: point, VisualGoal: "展示骑行状态。", SourceType: "mixed"},
		},
	}}}, nil
}

func TestGenerateWorkbenchScriptsHandlerUsesConfiguredService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	products := services.NewProductAssetService()
	product := products.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	point, err := products.CreateSellingPoint(product.ID, services.CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	scripts := services.NewScriptGenerationService(products, services.NewSystemConfigService(), services.NewModelProviderService(), config.Config{}).
		WithGenerator(scriptGenerationHandlerGenerator{})
	server := New(Options{
		Config:                  config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService:     products,
		ScriptGenerationService: scripts,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/workbench/scripts/generate", bytes.NewBufferString(`{
		"product_id":"`+product.ID+`",
		"selling_point_ids":["`+point.ID+`"],
		"custom_selling_points":[],
		"variant_count":1
	}`))
	request.Header.Set("Authorization", voiceoverUserAuthHeader())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []services.GeneratedScriptVariant `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].ID == "" || response.Data[0].Status != "draft" || len(response.Data[0].Beats) != 3 {
		t.Fatalf("unexpected response %#v", response.Data)
	}
}

func TestGenerateWorkbenchScriptsHandlerReportsMissingLLMConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	products := services.NewProductAssetService()
	product := products.CreateProduct(services.CreateProductInput{Name: "束裤带"})
	point, err := products.CreateSellingPoint(product.ID, services.CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		ProductAssetService: products,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/workbench/scripts/generate", bytes.NewBufferString(`{
		"product_id":"`+product.ID+`",
		"selling_point_ids":["`+point.ID+`"],
		"variant_count":1
	}`))
	request.Header.Set("Authorization", voiceoverUserAuthHeader())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
