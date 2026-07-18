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
	maxCandidateSemanticSummaryRunes     = 320
)

var ErrCandidateSearchUnavailable = errors.New("candidate search is not configured")

type ShotRequirement struct {
	VisualBeatID        string   `json:"visual_beat_id"`
	NarrationSegmentID  string   `json:"narration_segment_id"`
	NarrationSegmentIDs []string `json:"narration_segment_ids"`
	StartMs             int      `json:"start_ms"`
	EndMs               int      `json:"end_ms"`
	DurationClass       string   `json:"duration_class"`
	NarrationText       string   `json:"narration_text"`
	SellingPoint        string   `json:"selling_point"`
	VisualGoal          string   `json:"visual_goal"`
	SourceType          string   `json:"source_type"`
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
	SemanticSummary         string  `json:"semantic_summary"`
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
	assetUseCounts := map[string]int{}
	sets := make([]CandidateSet, 0, len(requirements))
	for _, requirement := range requirements {
		requirement.DurationClass = normalizeVisualBeatDurationClass(requirement.DurationClass)
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

		searchInput := CandidateSearchInput{
			ProductID:         productID,
			ProviderID:        providerID,
			Model:             model,
			Dimension:         queryDimension,
			QueryEmbedding:    embedding.Embedding,
			SourceTypes:       requirementSourceTypes(requirement.SourceType),
			MinimumDurationMs: requirement.EndMs - requirement.StartMs,
			Limit:             limit,
		}
		candidates, err := s.store.SearchCandidates(ctx, searchInput)
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

func BuildShotRequirements(visualBeats []VisualBeat, narrationSegments []NarrationSegment) ([]ShotRequirement, error) {
	if len(visualBeats) == 0 {
		return nil, fmt.Errorf("visual beats are required")
	}
	if err := validateNarrationTimeline(narrationSegments, 0); err != nil {
		return nil, err
	}
	requirements := make([]ShotRequirement, 0, len(visualBeats))
	expectedStartMs := narrationSegments[0].StartMs
	timelineEndMs := narrationSegments[len(narrationSegments)-1].EndMs
	for index, beat := range visualBeats {
		if beat.StartMs != expectedStartMs || beat.EndMs <= beat.StartMs || beat.EndMs > timelineEndMs {
			return nil, fmt.Errorf("visual beat %d timeline range is invalid", index+1)
		}
		anchor, overlaps := narrationSegmentsForVisualBeat(beat, narrationSegments)
		if anchor == nil || anchor.ID != beat.NarrationSegmentID {
			return nil, fmt.Errorf("visual beat %d narration anchor does not contain its start", index+1)
		}
		if len(overlaps) == 0 {
			return nil, fmt.Errorf("visual beat %d does not overlap narration", index+1)
		}
		sourceType := strings.TrimSpace(beat.SourceType)
		if sourceType != "visual_only" && sourceType != "talking_head" && sourceType != "mixed" {
			return nil, fmt.Errorf("visual beat %d source type is invalid", index+1)
		}
		narrationIDs := make([]string, 0, len(overlaps))
		narrationTexts := make([]string, 0, len(overlaps))
		for _, segment := range overlaps {
			narrationIDs = append(narrationIDs, segment.ID)
			narrationTexts = append(narrationTexts, strings.TrimSpace(segment.Text))
		}
		requirements = append(requirements, ShotRequirement{
			VisualBeatID:        strings.TrimSpace(beat.ID),
			NarrationSegmentID:  anchor.ID,
			NarrationSegmentIDs: narrationIDs,
			StartMs:             beat.StartMs,
			EndMs:               beat.EndMs,
			DurationClass:       normalizeVisualBeatDurationClass(beat.DurationClass),
			NarrationText:       strings.Join(narrationTexts, ""),
			SellingPoint:        strings.TrimSpace(beat.SellingPoint),
			VisualGoal:          strings.TrimSpace(beat.VisualGoal),
			SourceType:          sourceType,
		})
		expectedStartMs = beat.EndMs
	}
	if expectedStartMs != timelineEndMs {
		return nil, fmt.Errorf("visual beats do not cover the narration timeline")
	}
	return requirements, nil
}

func narrationSegmentsForVisualBeat(beat VisualBeat, segments []NarrationSegment) (*NarrationSegment, []NarrationSegment) {
	var anchor *NarrationSegment
	overlaps := make([]NarrationSegment, 0, 2)
	for index := range segments {
		segment := &segments[index]
		if beat.StartMs >= segment.StartMs && beat.StartMs < segment.EndMs {
			anchor = segment
		}
		if segment.StartMs < beat.EndMs && segment.EndMs > beat.StartMs {
			overlaps = append(overlaps, *segment)
		}
	}
	return anchor, overlaps
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

func buildCandidateQueryText(requirement ShotRequirement) string {
	return strings.TrimSpace(requirement.VisualGoal)
}

func candidateSemanticSummary(text string) string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxCandidateSemanticSummaryRunes {
		return text
	}
	return string(runes[:maxCandidateSemanticSummaryRunes]) + "..."
}

func validateShotRequirement(requirement ShotRequirement) error {
	if strings.TrimSpace(requirement.VisualBeatID) == "" {
		return fmt.Errorf("visual beat id is required")
	}
	if strings.TrimSpace(requirement.NarrationSegmentID) == "" {
		return fmt.Errorf("narration segment id is required")
	}
	if requirement.StartMs < 0 || requirement.EndMs <= requirement.StartMs {
		return fmt.Errorf("narration segment range is invalid")
	}
	if !isVisualBeatDurationValid(normalizeVisualBeatDurationClass(requirement.DurationClass), requirement.EndMs-requirement.StartMs) {
		return fmt.Errorf("visual beat duration does not match its duration class")
	}
	if strings.TrimSpace(requirement.NarrationText) == "" || strings.TrimSpace(requirement.VisualGoal) == "" {
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
	distanceExpression := vectorCosineDistanceSQL("e.embedding", "$1", input.Model, input.Dimension)
	query := fmt.Sprintf(`
		SELECT
			e.id::text,
			a.id::text,
			'',
			e.object_type,
			e.text,
			a.source_type,
			0 AS source_in_ms,
			COALESCE(a.duration_ms, 0) AS source_out_ms,
			COALESCE(a.duration_ms, 0),
			a.default_use_original_audio,
			1 - (%s) AS semantic_score
		FROM asset_embedding_objects e
		JOIN assets a ON a.id = e.asset_id
		WHERE e.status = 'ready'
		  AND e.provider_id = $2::uuid
		  AND e.model = $3
		  AND e.dimension = $4
		  AND a.product_id = $5::uuid
		  AND a.status = 'ready'
		  AND a.usability_status IN ('usable', 'needs_review')
		  AND e.object_type = 'shot'
		  AND a.source_type = ANY($6::text[])
		  AND COALESCE(a.duration_ms, 0) >= $7
		ORDER BY %s ASC, e.id ASC
		LIMIT $8`, distanceExpression, distanceExpression)
	rows, err := s.pool.Query(ctx, query,
		vectorString(input.QueryEmbedding),
		input.ProviderID,
		input.Model,
		input.Dimension,
		input.ProductID,
		input.SourceTypes,
		input.MinimumDurationMs,
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
			&candidate.SemanticSummary,
			&candidate.SourceType,
			&candidate.SourceInMs,
			&candidate.SourceOutMs,
			&candidate.AssetDurationMs,
			&candidate.DefaultUseOriginalAudio,
			&candidate.SemanticScore,
		); err != nil {
			return nil, err
		}
		candidate.SemanticSummary = candidateSemanticSummary(candidate.SemanticSummary)
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
