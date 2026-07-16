package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type recordingScriptGenerator struct {
	input  modelgateway.ScriptGenerationInput
	result modelgateway.ScriptGenerationResult
	err    error
}

func (g *recordingScriptGenerator) GenerateScripts(_ context.Context, input modelgateway.ScriptGenerationInput) (modelgateway.ScriptGenerationResult, error) {
	g.input = input
	return g.result, g.err
}

func TestScriptGenerationServiceUsesStoredProductAndSellingPoints(t *testing.T) {
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带", Description: "骑行裤脚固定用品", Category: "骑行"})
	point, err := products.CreateSellingPoint(product.ID, CreateSellingPointInput{Title: "避免蹭链条", Description: "固定裤脚，避免蹭到链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	generator := &recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "夜骑更安心")}
	service := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(generator)

	variants, err := service.Generate(context.Background(), WorkbenchScriptGenerationInput{
		ProductID:           product.ID,
		SellingPointIDs:     []string{point.ID},
		CustomSellingPoints: []string{"夜骑更安心"},
		VariantCount:        1,
	})
	if err != nil {
		t.Fatalf("generate scripts: %v", err)
	}
	if generator.input.ProductName != product.Name || len(generator.input.SellingPoints) != 2 {
		t.Fatalf("unexpected generator input %#v", generator.input)
	}
	if len(variants) != 1 || variants[0].ID == "" || variants[0].Order != 1 || variants[0].Status != "draft" || len(variants[0].Beats) != 3 {
		t.Fatalf("unexpected generated variants %#v", variants)
	}
	if variants[0].EstimatedDurationMs < 8000 {
		t.Fatalf("expected estimated duration, got %d", variants[0].EstimatedDurationMs)
	}
}

func TestScriptGenerationServiceRejectsUnavailableSellingPoint(t *testing.T) {
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带"})
	service := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(
		&recordingScriptGenerator{result: validScriptGenerationResult("不存在", "不存在")},
	)

	_, err := service.Generate(context.Background(), WorkbenchScriptGenerationInput{
		ProductID:       product.ID,
		SellingPointIDs: []string{"missing"},
		VariantCount:    1,
	})
	if !errors.Is(err, ErrScriptGenerationInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestScriptGenerationServiceRejectsMissingSellingPointCoverage(t *testing.T) {
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带"})
	point, err := products.CreateSellingPoint(product.ID, CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	service := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(
		&recordingScriptGenerator{result: validScriptGenerationResult("错误卖点", "错误卖点")},
	)

	_, err = service.Generate(context.Background(), WorkbenchScriptGenerationInput{
		ProductID:       product.ID,
		SellingPointIDs: []string{point.ID},
		VariantCount:    1,
	})
	var gatewayErr *modelgateway.Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != modelgateway.ErrorCodeInvalidResponse {
		t.Fatalf("expected invalid model output error, got %v", err)
	}
}

func validScriptGenerationResult(firstSellingPoint string, secondSellingPoint string) modelgateway.ScriptGenerationResult {
	return modelgateway.ScriptGenerationResult{Variants: []modelgateway.ScriptGenerationVariant{{
		Hook:          "裤脚不再蹭链条",
		ScriptText:    "骑车时，裤脚总会蹭到链条。轻轻一贴，固定更稳，骑行更利落。",
		EditingIntent: "从骑行痛点切入，再展示固定动作和结果。",
		Beats: []modelgateway.ScriptGenerationBeat{
			{Label: "开头", SellingPoint: firstSellingPoint, VisualGoal: "展示裤脚接近链条。", SourceType: "mixed"},
			{Label: "展示", SellingPoint: firstSellingPoint, VisualGoal: "展示贴合固定动作。", SourceType: "visual_only"},
			{Label: "收束", SellingPoint: secondSellingPoint, VisualGoal: "展示骑行结果。", SourceType: "mixed"},
		},
	}}}
}
