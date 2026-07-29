package services

import (
	"context"
	"errors"
	"strings"
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
	if _, err := products.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "fasten.mp4",
		StorageKey:        "assets/fasten.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "ready",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		SceneDescription:  "束裤带环绕脚踝并收紧裤脚",
		ModelLabels: map[string]any{
			"action_description": "双手压紧束裤带魔术贴完成固定",
		},
	}); err != nil {
		t.Fatalf("create visual evidence asset: %v", err)
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
	if generator.input.ProductName != product.Name || len(generator.input.SellingPoints) != 2 || generator.input.TargetDurationSeconds != 30 {
		t.Fatalf("unexpected generator input %#v", generator.input)
	}
	if len(generator.input.AvailableVisualEvidence) != 1 || !strings.Contains(generator.input.AvailableVisualEvidence[0], "压紧束裤带魔术贴") {
		t.Fatalf("expected real product footage evidence, got %#v", generator.input.AvailableVisualEvidence)
	}
	if len(variants) != 1 || variants[0].ID == "" || variants[0].Order != 1 || variants[0].Status != "draft" || len(variants[0].Beats) != 4 {
		t.Fatalf("unexpected generated variants %#v", variants)
	}
	for _, beat := range variants[0].Beats {
		if beat.SourceType != modelgateway.TTSVisualSourceType {
			t.Fatalf("expected visual-only TTS beat, got %#v", beat)
		}
	}
	if variants[0].EstimatedDurationMs < 8000 {
		t.Fatalf("expected estimated duration, got %d", variants[0].EstimatedDurationMs)
	}
}

func TestScriptGenerationServiceRejectsUnsupportedTargetDuration(t *testing.T) {
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带"})
	service := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(
		&recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "避免蹭链条")},
	)

	_, err := service.Generate(context.Background(), WorkbenchScriptGenerationInput{
		ProductID:             product.ID,
		CustomSellingPoints:   []string{"避免蹭链条"},
		VariantCount:          1,
		TargetDurationSeconds: 25,
	})
	if !errors.Is(err, ErrScriptGenerationInput) {
		t.Fatalf("expected invalid target duration error, got %v", err)
	}
}

func TestScriptGenerationServiceRejectsSellingPointsBeyondBeatCapacity(t *testing.T) {
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带"})
	service := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(
		&recordingScriptGenerator{result: validScriptGenerationResult("卖点一", "卖点二")},
	)

	_, err := service.Generate(context.Background(), WorkbenchScriptGenerationInput{
		ProductID: product.ID,
		CustomSellingPoints: []string{
			"卖点一", "卖点二", "卖点三", "卖点四", "卖点五", "卖点六",
		},
		VariantCount:          1,
		TargetDurationSeconds: 15,
	})
	if !errors.Is(err, ErrScriptGenerationInput) || !strings.Contains(err.Error(), "exceed the capacity") {
		t.Fatalf("expected selling point capacity error, got %v", err)
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
		Hook:          "骑车时裤脚总往链条上蹭",
		ScriptText:    "骑车时裤脚总往链条上蹭，不仅容易沾上油污，还可能卷进齿盘。出门前把束裤带绕在脚踝上，调整到合适松紧，再把魔术贴压紧，裤脚马上被收住。弹力材质贴合腿部，蹬车时不会明显束缚；大面积魔术贴固定牢靠，骑行过程中不容易松开。晚上经过车灯照射时，反光条更加醒目，夜间骑行也更安心。骑完撕下来卷好放进口袋，一条小绑带，就把整洁、安全和收纳都照顾到了。",
		EditingIntent: "采用痛点解决角度，从裤脚蹭链条切入，依次展示固定、贴合、反光和收纳结果。",
		Beats: []modelgateway.ScriptGenerationBeat{
			{Label: "痛点", SellingPoint: firstSellingPoint, VisualGoal: "骑行时裤脚靠近自行车链条", SourceType: "visual_only"},
			{Label: "固定", SellingPoint: firstSellingPoint, VisualGoal: "双手将束裤带绕过脚踝并压紧魔术贴", SourceType: "visual_only"},
			{Label: "贴合", SellingPoint: firstSellingPoint, VisualGoal: "束裤带贴合脚踝并固定收紧裤脚", SourceType: "visual_only"},
			{Label: "结果", SellingPoint: secondSellingPoint, VisualGoal: "车灯照射束裤带反光条并明显反光", SourceType: "visual_only"},
		},
	}}}
}
