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
		Hook:          "骑车时裤脚总往链条上蹭",
		ScriptText:    "骑车时裤脚总往链条上蹭，不仅容易沾上油污，还可能卷进齿盘。出门前把束裤带绕在脚踝上，调整到合适松紧，再把魔术贴压紧，裤脚马上被收住。弹力材质贴合腿部，蹬车时不会明显束缚；大面积魔术贴固定牢靠，骑行过程中不容易松开。晚上经过车灯照射时，反光条更加醒目，夜间骑行也更安心。骑完撕下来卷好放进口袋，一条小绑带，就把整洁、安全和收纳都照顾到了。",
		EditingIntent: "采用痛点解决角度，从骑行痛点切入，再展示固定动作和使用结果。",
		Beats: []modelgateway.ScriptGenerationBeat{
			{Label: "痛点", SellingPoint: point, VisualGoal: "骑行时裤脚靠近自行车链条", SourceType: "visual_only"},
			{Label: "固定", SellingPoint: point, VisualGoal: "双手将束裤带绕过脚踝并压紧魔术贴", SourceType: "visual_only"},
			{Label: "贴合", SellingPoint: point, VisualGoal: "束裤带贴合脚踝并固定收紧裤脚", SourceType: "visual_only"},
			{Label: "结果", SellingPoint: point, VisualGoal: "骑行过程中束裤带保持固定状态", SourceType: "visual_only"},
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
	if len(response.Data) != 1 || response.Data[0].ID == "" || response.Data[0].Status != "draft" || len(response.Data[0].Beats) != 4 {
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
