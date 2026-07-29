package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	maxWorkbenchScriptVariants      = 8
	maxWorkbenchSellingPointInputs  = 24
	maxWorkbenchCustomSellingPoints = 12
	maxWorkbenchSellingPointRunes   = 120
	maxScriptVisualEvidenceItems    = 48
	maxScriptVisualEvidenceRunes    = 140
)

var (
	ErrScriptGenerationInput = errors.New("invalid script generation input")
	ErrLLMNotConfigured      = errors.New("llm is not configured")
)

type WorkbenchScriptGenerationInput struct {
	ProductID             string   `json:"product_id"`
	SellingPointIDs       []string `json:"selling_point_ids"`
	CustomSellingPoints   []string `json:"custom_selling_points"`
	VariantCount          int      `json:"variant_count"`
	TargetDurationSeconds int      `json:"target_duration_seconds"`
}

type GeneratedScriptBeat struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	SellingPoint string `json:"selling_point"`
	VisualGoal   string `json:"visual_goal"`
	SourceType   string `json:"source_type"`
}

type GeneratedScriptVariant struct {
	ID                  string                `json:"id"`
	Order               int                   `json:"order"`
	Hook                string                `json:"hook"`
	ScriptText          string                `json:"script_text"`
	EstimatedDurationMs int                   `json:"estimated_duration_ms"`
	EditingIntent       string                `json:"editing_intent"`
	Beats               []GeneratedScriptBeat `json:"beats"`
	Status              string                `json:"status"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

type ScriptGenerationService struct {
	productAssetService  *ProductAssetService
	systemConfigService  *SystemConfigService
	modelProviderService *ModelProviderService
	fallbackConfig       config.Config
	generator            modelgateway.ScriptGenerator
}

func NewScriptGenerationService(
	productAssetService *ProductAssetService,
	systemConfigService *SystemConfigService,
	modelProviderService *ModelProviderService,
	fallbackConfig config.Config,
) *ScriptGenerationService {
	return &ScriptGenerationService{
		productAssetService:  productAssetService,
		systemConfigService:  systemConfigService,
		modelProviderService: modelProviderService,
		fallbackConfig:       fallbackConfig,
	}
}

func (s *ScriptGenerationService) WithGenerator(generator modelgateway.ScriptGenerator) *ScriptGenerationService {
	if generator != nil {
		s.generator = generator
	}
	return s
}

func (s *ScriptGenerationService) Generate(ctx context.Context, input WorkbenchScriptGenerationInput) ([]GeneratedScriptVariant, error) {
	if s == nil || s.productAssetService == nil {
		return nil, fmt.Errorf("script generation service is not configured")
	}
	input, err := normalizeWorkbenchScriptGenerationInput(input)
	if err != nil {
		return nil, err
	}
	product, err := s.productAssetService.GetProduct(input.ProductID)
	if err != nil {
		return nil, err
	}
	if product.Status == "archived" {
		return nil, ErrProductNotFound
	}
	sellingPoints, expectedSellingPointNames, err := s.resolveSellingPoints(input)
	if err != nil {
		return nil, err
	}
	_, maximumBeats := modelgateway.ScriptBeatCountRange(input.TargetDurationSeconds)
	if len(sellingPoints) > input.VariantCount*maximumBeats {
		return nil, fmt.Errorf("%w: selected selling points exceed the capacity of %d scripts at %d seconds", ErrScriptGenerationInput, input.VariantCount, input.TargetDurationSeconds)
	}

	generator := s.generator
	if generator == nil {
		if err := EnsureLegacyOpenAICompatibleProvider(ctx, s.systemConfigService, s.modelProviderService); err != nil {
			return nil, err
		}
		cfg := ResolveLLMScriptConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
		if strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
			return nil, ErrLLMNotConfigured
		}
		generator = modelgateway.NewScriptGenerator(cfg)
	}

	generationInput := modelgateway.ScriptGenerationInput{
		ProductName:             product.Name,
		ProductDescription:      product.Description,
		ProductCategory:         product.Category,
		SellingPoints:           sellingPoints,
		AvailableVisualEvidence: s.availableScriptVisualEvidence(product.ID),
		VariantCount:            input.VariantCount,
		TargetDurationSeconds:   input.TargetDurationSeconds,
	}
	result, err := generator.GenerateScripts(ctx, generationInput)
	if err != nil {
		return nil, err
	}
	if err := modelgateway.ValidateScriptGenerationResult(result, generationInput); err != nil {
		return nil, err
	}
	if err := validateGeneratedSellingPointCoverage(result, expectedSellingPointNames); err != nil {
		return nil, err
	}

	now := time.Now()
	variants := make([]GeneratedScriptVariant, 0, len(result.Variants))
	for index, variant := range result.Variants {
		beats := make([]GeneratedScriptBeat, 0, len(variant.Beats))
		for _, beat := range variant.Beats {
			beats = append(beats, GeneratedScriptBeat{
				ID:           uuid.NewString(),
				Label:        beat.Label,
				SellingPoint: beat.SellingPoint,
				VisualGoal:   beat.VisualGoal,
				SourceType:   beat.SourceType,
			})
		}
		variants = append(variants, GeneratedScriptVariant{
			ID:                  uuid.NewString(),
			Order:               index + 1,
			Hook:                variant.Hook,
			ScriptText:          variant.ScriptText,
			EstimatedDurationMs: modelgateway.EstimateScriptDurationMs(variant.ScriptText),
			EditingIntent:       variant.EditingIntent,
			Beats:               beats,
			Status:              "draft",
			UpdatedAt:           now,
		})
	}
	return variants, nil
}

func normalizeWorkbenchScriptGenerationInput(input WorkbenchScriptGenerationInput) (WorkbenchScriptGenerationInput, error) {
	input.ProductID = strings.TrimSpace(input.ProductID)
	if input.ProductID == "" {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: product_id is required", ErrScriptGenerationInput)
	}
	if input.VariantCount < 1 || input.VariantCount > maxWorkbenchScriptVariants {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: variant_count must be between 1 and %d", ErrScriptGenerationInput, maxWorkbenchScriptVariants)
	}
	targetDuration, ok := modelgateway.NormalizeScriptTargetDuration(input.TargetDurationSeconds)
	if !ok {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: target_duration_seconds must be one of 15, 20, 30, 45, or 60", ErrScriptGenerationInput)
	}
	input.TargetDurationSeconds = targetDuration
	input.SellingPointIDs = normalizeScriptGenerationIDs(input.SellingPointIDs)
	if len(input.SellingPointIDs) > maxWorkbenchSellingPointInputs {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: too many selling points", ErrScriptGenerationInput)
	}
	input.CustomSellingPoints = normalizeCustomSellingPoints(input.CustomSellingPoints)
	if len(input.CustomSellingPoints) > maxWorkbenchCustomSellingPoints {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: too many custom selling points", ErrScriptGenerationInput)
	}
	for _, point := range input.CustomSellingPoints {
		if len([]rune(point)) > maxWorkbenchSellingPointRunes {
			return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: custom selling point is too long", ErrScriptGenerationInput)
		}
	}
	if len(input.SellingPointIDs) == 0 && len(input.CustomSellingPoints) == 0 {
		return WorkbenchScriptGenerationInput{}, fmt.Errorf("%w: at least one selling point is required", ErrScriptGenerationInput)
	}
	return input, nil
}

func normalizeScriptGenerationIDs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeCustomSellingPoints(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *ScriptGenerationService) resolveSellingPoints(input WorkbenchScriptGenerationInput) ([]modelgateway.ScriptGenerationSellingPoint, []string, error) {
	available := s.productAssetService.ListSellingPoints(input.ProductID)
	byID := make(map[string]SellingPoint, len(available))
	for _, point := range available {
		if point.Status == "archived" {
			continue
		}
		byID[point.ID] = point
	}
	points := make([]modelgateway.ScriptGenerationSellingPoint, 0, len(input.SellingPointIDs)+len(input.CustomSellingPoints))
	expectedNames := make([]string, 0, cap(points))
	seenNames := map[string]struct{}{}
	for _, id := range input.SellingPointIDs {
		point, ok := byID[id]
		if !ok {
			return nil, nil, fmt.Errorf("%w: selected selling point is unavailable", ErrScriptGenerationInput)
		}
		appendSellingPoint(&points, &expectedNames, seenNames, point.Title, point.Description, false)
	}
	for _, point := range input.CustomSellingPoints {
		appendSellingPoint(&points, &expectedNames, seenNames, point, "", true)
	}
	if len(points) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one selling point is required", ErrScriptGenerationInput)
	}
	return points, expectedNames, nil
}

func appendSellingPoint(target *[]modelgateway.ScriptGenerationSellingPoint, expectedNames *[]string, seen map[string]struct{}, name string, description string, isCustom bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if _, ok := seen[name]; ok {
		return
	}
	seen[name] = struct{}{}
	*target = append(*target, modelgateway.ScriptGenerationSellingPoint{
		Name:        name,
		Description: strings.TrimSpace(description),
		IsCustom:    isCustom,
	})
	*expectedNames = append(*expectedNames, name)
}

func validateGeneratedSellingPointCoverage(result modelgateway.ScriptGenerationResult, expectedNames []string) error {
	covered := make(map[string]bool, len(expectedNames))
	for _, variant := range result.Variants {
		for _, beat := range variant.Beats {
			for _, expected := range expectedNames {
				if strings.Contains(beat.SellingPoint, expected) {
					covered[expected] = true
				}
			}
		}
	}
	for _, expected := range expectedNames {
		if !covered[expected] {
			return modelgateway.NewError(modelgateway.ErrorCodeInvalidResponse, fmt.Sprintf("llm output does not cover selling point %q", expected), false, nil)
		}
	}
	return nil
}

func (s *ScriptGenerationService) availableScriptVisualEvidence(productID string) []string {
	assets := s.productAssetService.ListAssets(AssetFilters{
		ProductID:        productID,
		SourceType:       modelgateway.TTSVisualSourceType,
		Status:           "ready",
		ExcludeDiscarded: true,
	})
	result := make([]string, 0, min(len(assets), maxScriptVisualEvidenceItems))
	seen := map[string]struct{}{}
	for _, asset := range assets {
		if asset.AnalysisStatus != "ready" || (asset.UsabilityStatus != "usable" && asset.UsabilityStatus != "needs_review") {
			continue
		}
		parts := make([]string, 0, 2)
		if value := strings.TrimSpace(asset.SceneDescription); value != "" {
			parts = append(parts, "画面："+value)
		}
		if value := strings.TrimSpace(asset.ActionDescription); value != "" {
			parts = append(parts, "动作："+value)
		}
		if len(parts) == 0 {
			continue
		}
		text := truncateScriptEvidence(strings.Join(parts, "；"), maxScriptVisualEvidenceRunes)
		key := strings.ToLower(strings.TrimSpace(text))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, text)
		if len(result) == maxScriptVisualEvidenceItems {
			break
		}
	}
	return result
}

func truncateScriptEvidence(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
