package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	defaultCandidatesPerNarrationSegment = 10
	maxCandidatesPerNarrationSegment     = 12
	assetReusePenalty                    = 0.12
)

var ErrCandidateSearchUnavailable = errors.New("candidate search is not configured")

type ShotRequirement struct {
	NarrationSegmentID string `json:"narration_segment_id"`
	StartMs            int    `json:"start_ms"`
	EndMs              int    `json:"end_ms"`
	NarrationText      string `json:"narration_text"`
	SellingPoint       string `json:"selling_point"`
	VisualGoal         string `json:"visual_goal"`
	SourceType         string `json:"source_type"`
}

type AssetCandidate struct {
	ID                      string  `json:"id"`
	AssetID                 string  `json:"asset_id"`
	SpeechSegmentID         string  `json:"speech_segment_id,omitempty"`
	ObjectType              string  `json:"object_type"`
	SourceType              string  `json:"source_type"`
	SourceInMs              int     `json:"source_in_ms"`
	SourceOutMs             int     `json:"source_out_ms"`
	AssetDurationMs         int     `json:"asset_duration_ms"`
	DefaultUseOriginalAudio bool    `json:"default_use_original_audio"`
	SemanticScore           float64 `json:"semantic_score"`
	DiversityScore          float64 `json:"diversity_score"`
}

type CandidateSet struct {
	Requirement ShotRequirement  `json:"requirement"`
	Candidates  []AssetCandidate `json:"candidates"`
}

type CandidateSearchInput struct {
	ProductID         string
	ProviderID        string
	Model             string
	Dimension         int
	QueryEmbedding    []float64
	SourceTypes       []string
	SellingPointIDs   []string
	MinimumDurationMs int
	Limit             int
}

type CandidateSearchStore interface {
	SearchCandidates(context.Context, CandidateSearchInput) ([]AssetCandidate, error)
}

type candidateSearchFunc func(context.Context, CandidateSearchInput) ([]AssetCandidate, error)

func (f candidateSearchFunc) SearchCandidates(ctx context.Context, input CandidateSearchInput) ([]AssetCandidate, error) {
	return f(ctx, input)
}

type AssetCandidateService struct {
	pool                 *pgxpool.Pool
	productAssetService  *ProductAssetService
	systemConfigService  *SystemConfigService
	modelProviderService *ModelProviderService
	fallbackConfig       config.Config
	store                CandidateSearchStore
	embedder             modelgateway.TextEmbedder
}

func NewAssetCandidateService(
	pool *pgxpool.Pool,
	productAssetService *ProductAssetService,
	systemConfigService *SystemConfigService,
	modelProviderService *ModelProviderService,
	fallbackConfig config.Config,
) *AssetCandidateService {
	service := &AssetCandidateService{
		pool:                 pool,
		productAssetService:  productAssetService,
		systemConfigService:  systemConfigService,
		modelProviderService: modelProviderService,
		fallbackConfig:       fallbackConfig,
	}
	if pool != nil {
		service.store = postgresCandidateSearchStore{pool: pool}
	}
	return service
}

func (s *AssetCandidateService) WithEmbedder(embedder modelgateway.TextEmbedder) *AssetCandidateService {
	if embedder != nil {
		s.embedder = embedder
	}
	return s
}

func (s *AssetCandidateService) WithStore(store CandidateSearchStore) *AssetCandidateService {
	if store != nil {
		s.store = store
	}
	return s
}

func (s *AssetCandidateService) Retrieve(ctx context.Context, productID string, requirements []ShotRequirement, limit int) ([]CandidateSet, error) {
	if s == nil || s.productAssetService == nil {
		return nil, ErrCandidateSearchUnavailable
	}
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil, fmt.Errorf("product id is required")
	}
	if _, err := s.productAssetService.GetProduct(productID); err != nil {
		return nil, err
	}
	if len(requirements) == 0 {
		return nil, fmt.Errorf("shot requirements are required")
	}
	if limit <= 0 {
		limit = defaultCandidatesPerNarrationSegment
	}
	if limit > maxCandidatesPerNarrationSegment {
		limit = maxCandidatesPerNarrationSegment
	}
	if s.store == nil {
		return nil, ErrCandidateSearchUnavailable
	}

	embedder, providerID, model, dimension, err := s.resolveEmbedder(ctx)
	if err != nil {
		return nil, err
	}
	sellingPointIDs := s.resolveSellingPointIDs(productID)
	assetUseCounts := map[string]int{}
	sets := make([]CandidateSet, 0, len(requirements))
	for _, requirement := range requirements {
		if err := validateShotRequirement(requirement); err != nil {
			return nil, err
		}
		queryText := buildCandidateQueryText(requirement)
		embedding, err := embedder.EmbedText(ctx, modelgateway.EmbedTextInput{Text: queryText})
		if err != nil {
			return nil, err
		}
		if len(embedding.Embedding) == 0 {
			return nil, fmt.Errorf("candidate query embedding is empty")
		}
		queryDimension := dimension
		if queryDimension <= 0 {
			queryDimension = len(embedding.Embedding)
		}
		if len(embedding.Embedding) != queryDimension {
			return nil, fmt.Errorf("candidate query embedding dimension mismatch: expected %d, got %d", queryDimension, len(embedding.Embedding))
		}

		matchedSellingPointIDs := matchingSellingPointIDs(sellingPointIDs, requirement.SellingPoint)
		candidates, err := s.store.SearchCandidates(ctx, CandidateSearchInput{
			ProductID:         productID,
			ProviderID:        providerID,
			Model:             model,
			Dimension:         queryDimension,
			QueryEmbedding:    embedding.Embedding,
			SourceTypes:       requirementSourceTypes(requirement.SourceType),
			SellingPointIDs:   matchedSellingPointIDs,
			MinimumDurationMs: requirement.EndMs - requirement.StartMs,
			Limit:             limit,
		})
		if err != nil {
			return nil, err
		}
		candidates = rerankCandidatesForDiversity(candidates, assetUseCounts)
		if len(candidates) > limit {
			candidates = candidates[:limit]
		}
		if len(candidates) > 0 {
			assetUseCounts[candidates[0].AssetID]++
		}
		sets = append(sets, CandidateSet{Requirement: requirement, Candidates: candidates})
	}
	return sets, nil
}

func BuildShotRequirements(narrationSegments []NarrationSegment, beats []VoiceoverBeat) ([]ShotRequirement, error) {
	if len(narrationSegments) == 0 {
		return nil, fmt.Errorf("narration segments are required")
	}
	if err := validateNarrationTimeline(narrationSegments, 0); err != nil {
		return nil, err
	}
	if len(beats) == 0 {
		beats = []VoiceoverBeat{{
			Label:      "叙事画面",
			VisualGoal: "用与旁白语义匹配的产品画面支撑表达。",
			SourceType: "visual_only",
		}}
	}
	requirements := make([]ShotRequirement, 0, len(narrationSegments))
	for index, segment := range narrationSegments {
		if segment.EndMs <= segment.StartMs || strings.TrimSpace(segment.Text) == "" || strings.TrimSpace(segment.ID) == "" {
			return nil, fmt.Errorf("narration segment %d is invalid", index+1)
		}
		beatIndex := minInt(len(beats)-1, index*len(beats)/len(narrationSegments))
		beat := beats[beatIndex]
		sourceType := strings.TrimSpace(beat.SourceType)
		if sourceType != "visual_only" && sourceType != "talking_head" && sourceType != "mixed" {
			sourceType = "visual_only"
		}
		requirements = append(requirements, ShotRequirement{
			NarrationSegmentID: segment.ID,
			StartMs:            segment.StartMs,
			EndMs:              segment.EndMs,
			NarrationText:      strings.TrimSpace(segment.Text),
			SellingPoint:       strings.TrimSpace(beat.SellingPoint),
			VisualGoal:         strings.TrimSpace(beat.VisualGoal),
			SourceType:         sourceType,
		})
	}
	return requirements, nil
}

func validateNarrationTimeline(segments []NarrationSegment, expectedDurationMs int) error {
	if len(segments) == 0 {
		return fmt.Errorf("narration segments are required")
	}
	previousEndMs := 0
	for index, segment := range segments {
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" || segment.StartMs != previousEndMs || segment.EndMs <= segment.StartMs {
			return fmt.Errorf("narration timeline is not continuous at segment %d", index+1)
		}
		previousEndMs = segment.EndMs
	}
	if expectedDurationMs > 0 && previousEndMs != expectedDurationMs {
		return fmt.Errorf("narration timeline does not cover the voiceover duration")
	}
	return nil
}

func (s *AssetCandidateService) resolveEmbedder(ctx context.Context) (modelgateway.TextEmbedder, string, string, int, error) {
	if s.embedder != nil {
		return s.embedder, "test-provider", "test-model", 0, nil
	}
	if s.systemConfigService == nil {
		return nil, "", "", 0, fmt.Errorf("system config service is required")
	}
	cfg := ResolveEmbeddingConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, "", "", 0, fmt.Errorf("embedding model is required")
	}
	providerID, err := s.resolveProviderID()
	if err != nil {
		return nil, "", "", 0, err
	}
	return modelgateway.NewTextEmbedder(cfg), providerID, cfg.Model, cfg.Dimensions, nil
}

func (s *AssetCandidateService) resolveProviderID() (string, error) {
	config, err := s.systemConfigService.Get("embedding.provider_id")
	if err != nil {
		return "", fmt.Errorf("embedding.provider_id is required")
	}
	providerID := strings.TrimSpace(configStringValue(config.Value))
	if providerID == "" {
		return "", fmt.Errorf("embedding.provider_id is required")
	}
	return providerID, nil
}

func (s *AssetCandidateService) resolveSellingPointIDs(productID string) map[string][]string {
	result := map[string][]string{}
	for _, point := range s.productAssetService.ListSellingPoints(productID) {
		if point.Status == "archived" {
			continue
		}
		title := strings.TrimSpace(point.Title)
		if title == "" {
			continue
		}
		result[title] = append(result[title], point.ID)
	}
	return result
}

func matchingSellingPointIDs(byTitle map[string][]string, sellingPoint string) []string {
	sellingPoint = strings.TrimSpace(sellingPoint)
	if sellingPoint == "" {
		return nil
	}
	matched := []string{}
	seen := map[string]struct{}{}
	for title, ids := range byTitle {
		if sellingPoint != title && !strings.Contains(sellingPoint, title) {
			continue
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			matched = append(matched, id)
		}
	}
	return matched
}

func buildCandidateQueryText(requirement ShotRequirement) string {
	parts := []string{"旁白：" + requirement.NarrationText}
	if requirement.SellingPoint != "" {
		parts = append(parts, "卖点："+requirement.SellingPoint)
	}
	if requirement.VisualGoal != "" {
		parts = append(parts, "画面目标："+requirement.VisualGoal)
	}
	return strings.Join(parts, "；")
}

func validateShotRequirement(requirement ShotRequirement) error {
	if strings.TrimSpace(requirement.NarrationSegmentID) == "" {
		return fmt.Errorf("narration segment id is required")
	}
	if requirement.StartMs < 0 || requirement.EndMs <= requirement.StartMs {
		return fmt.Errorf("narration segment range is invalid")
	}
	if strings.TrimSpace(requirement.NarrationText) == "" {
		return fmt.Errorf("narration text is required")
	}
	if requirement.SourceType != "visual_only" && requirement.SourceType != "talking_head" && requirement.SourceType != "mixed" {
		return fmt.Errorf("shot requirement source type is invalid")
	}
	return nil
}

func requirementSourceTypes(sourceType string) []string {
	if sourceType == "mixed" {
		return []string{"visual_only", "talking_head"}
	}
	return []string{sourceType}
}

func rerankCandidatesForDiversity(candidates []AssetCandidate, assetUseCounts map[string]int) []AssetCandidate {
	result := append([]AssetCandidate(nil), candidates...)
	for index := range result {
		penalty := assetReusePenalty * float64(assetUseCounts[result[index].AssetID])
		result[index].DiversityScore = result[index].SemanticScore - penalty
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].DiversityScore == result[j].DiversityScore {
			return result[i].ID < result[j].ID
		}
		return result[i].DiversityScore > result[j].DiversityScore
	})
	return result
}

type postgresCandidateSearchStore struct {
	pool *pgxpool.Pool
}

func (s postgresCandidateSearchStore) SearchCandidates(ctx context.Context, input CandidateSearchInput) ([]AssetCandidate, error) {
	if s.pool == nil {
		return nil, ErrCandidateSearchUnavailable
	}
	if input.Limit <= 0 || len(input.QueryEmbedding) == 0 || input.Dimension <= 0 {
		return nil, fmt.Errorf("candidate search input is invalid")
	}
	for _, value := range input.QueryEmbedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("candidate query embedding contains a non-finite value")
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			e.id::text,
			a.id::text,
			COALESCE(s.id::text, ''),
			e.object_type,
			a.source_type,
			CASE WHEN e.object_type = 'speech_segment' THEN s.start_ms ELSE 0 END AS source_in_ms,
			CASE WHEN e.object_type = 'speech_segment' THEN s.end_ms ELSE COALESCE(a.duration_ms, 0) END AS source_out_ms,
			COALESCE(a.duration_ms, 0),
			a.default_use_original_audio,
			1 - (e.embedding <=> $1::vector) AS semantic_score
		FROM asset_embedding_objects e
		JOIN assets a ON a.id = e.asset_id
		LEFT JOIN speech_segments s ON e.object_type = 'speech_segment' AND s.id = e.object_id
		WHERE e.status = 'ready'
		  AND e.provider_id = $2::uuid
		  AND e.model = $3
		  AND e.dimension = $4
		  AND a.product_id = $5::uuid
		  AND a.status = 'ready'
		  AND a.usability_status IN ('usable', 'needs_review')
		  AND (
				(a.source_type = 'visual_only' AND e.object_type = 'shot' AND 'visual_only' = ANY($6::text[]))
				OR
				(a.source_type = 'talking_head' AND e.object_type = 'speech_segment' AND 'talking_head' = ANY($6::text[]))
		  )
		  AND (
				CASE WHEN e.object_type = 'speech_segment'
					THEN COALESCE(s.end_ms - s.start_ms, 0)
					ELSE COALESCE(a.duration_ms, 0)
				END
			) >= $7
		  AND (
				cardinality($8::uuid[]) = 0
				OR EXISTS (
					SELECT 1
					FROM asset_selling_points asp
					WHERE asp.asset_id = a.id AND asp.selling_point_id = ANY($8::uuid[])
				)
		  )
		ORDER BY e.embedding <=> $1::vector ASC, e.id ASC
		LIMIT $9`,
		vectorString(input.QueryEmbedding),
		input.ProviderID,
		input.Model,
		input.Dimension,
		input.ProductID,
		input.SourceTypes,
		input.MinimumDurationMs,
		input.SellingPointIDs,
		input.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AssetCandidate{}
	for rows.Next() {
		var candidate AssetCandidate
		if err := rows.Scan(
			&candidate.ID,
			&candidate.AssetID,
			&candidate.SpeechSegmentID,
			&candidate.ObjectType,
			&candidate.SourceType,
			&candidate.SourceInMs,
			&candidate.SourceOutMs,
			&candidate.AssetDurationMs,
			&candidate.DefaultUseOriginalAudio,
			&candidate.SemanticScore,
		); err != nil {
			return nil, err
		}
		candidate.DiversityScore = candidate.SemanticScore
		items = append(items, candidate)
	}
	return items, rows.Err()
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
