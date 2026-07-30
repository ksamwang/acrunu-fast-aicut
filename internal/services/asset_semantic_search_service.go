package services

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	defaultSemanticAssetSearchLimit = 20
	maxSemanticAssetSearchLimit     = 100
	indexedEmbeddingModel           = "text-embedding-v4"
	indexedEmbeddingDimension       = 1024
)

type AssetSemanticSearchInput struct {
	Query   string
	Filters AssetFilters
	Limit   int
	Offset  int
}

type AssetSemanticSearchResult struct {
	Query string  `json:"query"`
	Items []Asset `json:"items"`
	Total int     `json:"total"`
}

type semanticAssetSearchStoreInput struct {
	ProviderID     string
	Model          string
	Dimension      int
	QueryEmbedding []float64
	Filters        AssetFilters
	Limit          int
	Offset         int
}

type semanticAssetSearchHit struct {
	AssetID       string
	SemanticScore float64
}

type preparedAssetSemanticSearch struct {
	ProviderID     string
	Model          string
	Dimension      int
	QueryEmbedding []float64
}

func (s *AssetEmbeddingService) SearchAssets(ctx context.Context, input AssetSemanticSearchInput) (AssetSemanticSearchResult, error) {
	if s == nil {
		return AssetSemanticSearchResult{}, fmt.Errorf("asset semantic search is not configured")
	}
	if s.productAssetService == nil {
		return AssetSemanticSearchResult{}, fmt.Errorf("product asset service is nil")
	}
	if input.Limit <= 0 {
		input.Limit = defaultSemanticAssetSearchLimit
	}
	if input.Limit > maxSemanticAssetSearchLimit {
		input.Limit = maxSemanticAssetSearchLimit
	}
	if input.Offset < 0 {
		input.Offset = 0
	}
	prepared, err := s.prepareAssetSemanticSearch(ctx, input.Query)
	if err != nil {
		return AssetSemanticSearchResult{}, err
	}
	input.Query = strings.TrimSpace(input.Query)

	hits, total, err := s.searchAssetEmbeddingObjects(ctx, semanticAssetSearchStoreInput{
		ProviderID:     prepared.ProviderID,
		Model:          prepared.Model,
		Dimension:      prepared.Dimension,
		QueryEmbedding: prepared.QueryEmbedding,
		Filters:        input.Filters,
		Limit:          input.Limit,
		Offset:         input.Offset,
	})
	if err != nil {
		return AssetSemanticSearchResult{}, err
	}
	items := make([]Asset, 0, len(hits))
	for _, hit := range hits {
		asset, ok := s.productAssetService.GetAsset(hit.AssetID)
		if !ok {
			continue
		}
		score := hit.SemanticScore
		asset.SemanticScore = &score
		items = append(items, asset)
	}
	return AssetSemanticSearchResult{Query: input.Query, Items: items, Total: total}, nil
}

func (s *AssetEmbeddingService) SearchAssetIDs(ctx context.Context, input AssetSemanticSearchInput) ([]string, error) {
	prepared, err := s.prepareAssetSemanticSearch(ctx, input.Query)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0)
	seen := map[string]struct{}{}
	for offset := 0; ; {
		hits, total, searchErr := s.searchAssetEmbeddingObjects(ctx, semanticAssetSearchStoreInput{
			ProviderID:     prepared.ProviderID,
			Model:          prepared.Model,
			Dimension:      prepared.Dimension,
			QueryEmbedding: prepared.QueryEmbedding,
			Filters:        input.Filters,
			Limit:          maxSemanticAssetSearchLimit,
			Offset:         offset,
		})
		if searchErr != nil {
			return nil, searchErr
		}
		for _, hit := range hits {
			if _, exists := seen[hit.AssetID]; exists {
				continue
			}
			seen[hit.AssetID] = struct{}{}
			ids = append(ids, hit.AssetID)
		}
		offset += len(hits)
		if len(hits) == 0 || offset >= total {
			break
		}
	}
	return ids, nil
}

func (s *AssetEmbeddingService) prepareAssetSemanticSearch(ctx context.Context, query string) (preparedAssetSemanticSearch, error) {
	if s == nil || s.pool == nil {
		return preparedAssetSemanticSearch{}, fmt.Errorf("asset semantic search is not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return preparedAssetSemanticSearch{}, fmt.Errorf("semantic search query is required")
	}
	cfg := ResolveEmbeddingConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	if strings.TrimSpace(cfg.Model) == "" {
		return preparedAssetSemanticSearch{}, fmt.Errorf("embedding model is required")
	}
	providerID, err := s.resolveEmbeddingProviderID()
	if err != nil {
		return preparedAssetSemanticSearch{}, err
	}
	result, err := modelgateway.NewTextEmbedder(cfg).EmbedText(ctx, modelgateway.EmbedTextInput{Text: query})
	if err != nil {
		return preparedAssetSemanticSearch{}, err
	}
	if len(result.Embedding) == 0 {
		return preparedAssetSemanticSearch{}, fmt.Errorf("semantic search embedding is empty")
	}
	dimension := cfg.Dimensions
	if dimension <= 0 {
		dimension = len(result.Embedding)
	}
	if len(result.Embedding) != dimension {
		return preparedAssetSemanticSearch{}, fmt.Errorf("semantic search embedding dimension mismatch: expected %d, got %d", dimension, len(result.Embedding))
	}
	return preparedAssetSemanticSearch{
		ProviderID:     providerID,
		Model:          cfg.Model,
		Dimension:      dimension,
		QueryEmbedding: result.Embedding,
	}, nil
}

func (s *AssetEmbeddingService) searchAssetEmbeddingObjects(ctx context.Context, input semanticAssetSearchStoreInput) ([]semanticAssetSearchHit, int, error) {
	if input.Limit <= 0 || input.Dimension <= 0 || len(input.QueryEmbedding) == 0 {
		return nil, 0, fmt.Errorf("semantic asset search input is invalid")
	}
	for _, value := range input.QueryEmbedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, 0, fmt.Errorf("semantic search embedding contains a non-finite value")
		}
	}

	countArgs := []any{}
	conditions, countArgs := buildSemanticAssetSearchConditions(input, countArgs)
	whereClause := strings.Join(conditions, "\n\t\t  AND ")
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM asset_embedding_objects e
		JOIN assets a ON a.id = e.asset_id
		WHERE `+whereClause, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []semanticAssetSearchHit{}, 0, nil
	}

	searchArgs := []any{vectorString(input.QueryEmbedding)}
	conditions, searchArgs = buildSemanticAssetSearchConditions(input, searchArgs)
	whereClause = strings.Join(conditions, "\n\t\t  AND ")
	distanceExpression := vectorCosineDistanceSQL("e.embedding", "$1", input.Model, input.Dimension)
	limitPlaceholder := semanticSearchBind(&searchArgs, input.Limit)
	offsetPlaceholder := semanticSearchBind(&searchArgs, input.Offset)
	query := fmt.Sprintf(`
		SELECT
			a.id::text,
			1 - (%s) AS semantic_score
		FROM asset_embedding_objects e
		JOIN assets a ON a.id = e.asset_id
		WHERE %s
		ORDER BY %s ASC, e.id ASC
		LIMIT %s OFFSET %s`, distanceExpression, whereClause, distanceExpression, limitPlaceholder, offsetPlaceholder)
	rows, err := s.pool.Query(ctx, query, searchArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	hits := make([]semanticAssetSearchHit, 0, input.Limit)
	for rows.Next() {
		var hit semanticAssetSearchHit
		if err := rows.Scan(&hit.AssetID, &hit.SemanticScore); err != nil {
			return nil, 0, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

func buildSemanticAssetSearchConditions(input semanticAssetSearchStoreInput, args []any) ([]string, []any) {
	conditions := []string{
		"e.status = 'ready'",
		"e.object_type = 'shot'",
		fmt.Sprintf("e.provider_id = %s::uuid", semanticSearchBind(&args, input.ProviderID)),
		fmt.Sprintf("e.model = %s", semanticSearchBind(&args, input.Model)),
		fmt.Sprintf("e.dimension = %s", semanticSearchBind(&args, input.Dimension)),
	}
	filters := input.Filters
	if value := strings.TrimSpace(filters.ProductID); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.product_id = %s::uuid", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.SourceType); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.source_type = %s", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.Status); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.status = %s", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.AnalysisStatus); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.analysis_status = %s", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.UsabilityStatus); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.usability_status = %s", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.ShotSize); value != "" {
		conditions = append(conditions, fmt.Sprintf("a.shot_size = %s", semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.SellingPointID); value != "" {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM asset_selling_points asp
			WHERE asp.asset_id = a.id AND asp.selling_point_id = %s::uuid
		)`, semanticSearchBind(&args, value)))
	}
	if value := strings.TrimSpace(filters.Tag); value != "" {
		exactPlaceholder := semanticSearchBind(&args, value)
		containsPlaceholder := semanticSearchBind(&args, "%"+value+"%")
		conditions = append(conditions, fmt.Sprintf(`(
			COALESCE(a.scene_description, '') = %s
			OR COALESCE(a.shot_size, '') = %s
			OR COALESCE(a.camera_movement, '') = %s
			OR COALESCE(a.scene_description, '') ILIKE %s
			OR COALESCE(a.subjects::text, '') ILIKE %s
			OR COALESCE(a.scene_tags::text, '') ILIKE %s
			OR COALESCE(a.quality_tags::text, '') ILIKE %s
		)`, exactPlaceholder, exactPlaceholder, exactPlaceholder, containsPlaceholder, containsPlaceholder, containsPlaceholder, containsPlaceholder))
	}
	if filters.MinDurationMs != nil {
		conditions = append(conditions, fmt.Sprintf("COALESCE(a.duration_ms, 0) >= %s", semanticSearchBind(&args, *filters.MinDurationMs)))
	}
	if filters.MaxDurationMs != nil {
		conditions = append(conditions, fmt.Sprintf("COALESCE(a.duration_ms, 0) <= %s", semanticSearchBind(&args, *filters.MaxDurationMs)))
	}
	if filters.HasAudio != nil {
		conditions = append(conditions, fmt.Sprintf("a.has_audio = %s", semanticSearchBind(&args, *filters.HasAudio)))
	}
	if filters.LikelyHasSpeech != nil {
		conditions = append(conditions, fmt.Sprintf("a.likely_has_speech = %s", semanticSearchBind(&args, *filters.LikelyHasSpeech)))
	}
	if filters.ExcludeDiscarded {
		conditions = append(conditions, "a.usability_status <> 'discarded'")
	}
	return conditions, args
}

func semanticSearchBind(args *[]any, value any) string {
	*args = append(*args, value)
	return fmt.Sprintf("$%d", len(*args))
}

func vectorCosineDistanceSQL(vectorColumn string, queryParameter string, model string, dimension int) string {
	if model == indexedEmbeddingModel && dimension == indexedEmbeddingDimension {
		return fmt.Sprintf("%s::vector(%d) <=> %s::vector(%d)", vectorColumn, indexedEmbeddingDimension, queryParameter, indexedEmbeddingDimension)
	}
	return fmt.Sprintf("%s <=> %s::vector", vectorColumn, queryParameter)
}
