package services

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	defaultCandidatesPerNarrationSegment = 10
	maxCandidatesPerNarrationSegment     = 40
	semanticCandidateScoreWindow         = 0.08
	semanticFallbackDiversityPenalty     = 1.0
	batchAssetReusePenalty               = 0.10
	recentAssetReusePenalty              = 0.015
	stableDiversityJitterAmplitude       = 0.008
	maxCandidateSemanticSummaryRunes     = 320
)

var ErrCandidateSearchUnavailable = errors.New("candidate search is not configured")

type ShotRequirement struct {
	VisualBeatID        string   `json:"visual_beat_id"`
	NarrationSegmentID  string   `json:"narration_segment_id"`
	NarrationSegmentIDs []string `json:"narration_segment_ids"`
	NarrativeBeatID     string   `json:"narrative_beat_id,omitempty"`
	StartMs             int      `json:"start_ms"`
	EndMs               int      `json:"end_ms"`
	DurationClass       string   `json:"duration_class"`
	NarrationText       string   `json:"narration_text"`
	Label               string   `json:"label"`
	SellingPoint        string   `json:"selling_point"`
	VisualGoal          string   `json:"visual_goal"`
	SourceType          string   `json:"source_type"`
}

type AssetCandidate struct {
	ID                      string  `json:"id"`
	AssetID                 string  `json:"asset_id"`
	ReuseKey                string  `json:"-"`
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
	SemanticQualified       bool    `json:"semantic_qualified"`
	BatchUseCount           int     `json:"batch_use_count"`
	RecentUseCount          int     `json:"recent_use_count"`
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

type CandidateDiversityContext struct {
	GenerationRunID   string
	GenerationBatchID string
}

type assetUsageSnapshot struct {
	batch  map[string]int
	recent map[string]int
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
	return s.RetrieveWithDiversity(ctx, productID, requirements, limit, CandidateDiversityContext{})
}

func (s *AssetCandidateService) RetrieveWithDiversity(
	ctx context.Context,
	productID string,
	requirements []ShotRequirement,
	limit int,
	diversity CandidateDiversityContext,
) ([]CandidateSet, error) {
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
	usage, err := s.loadAssetUsage(ctx, productID, diversity)
	if err != nil {
		return nil, err
	}
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
			MinimumDurationMs: minimumCandidateDuration(requirement.DurationClass),
			Limit:             limit,
		}
		candidates, err := s.store.SearchCandidates(ctx, searchInput)
		if err != nil {
			return nil, err
		}
		candidates = deduplicateCandidatesByReuseKey(candidates)
		candidates = markCandidateSemanticQualification(candidates)
		diversityKey := fmt.Sprintf("%d:%d:%s", requirement.StartMs, requirement.EndMs, strings.TrimSpace(requirement.VisualGoal))
		candidates = rerankCandidatesForDiversity(candidates, usage, diversity, diversityKey)
		if len(candidates) > limit {
			candidates = candidates[:limit]
		}
		sets = append(sets, CandidateSet{Requirement: requirement, Candidates: candidates})
	}
	return sets, nil
}

func minimumCandidateDuration(_ string) int {
	return modelgateway.MinimumEditPlanClipDurationMs
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
	timelineEndMs := visualBeats[len(visualBeats)-1].EndMs
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
			NarrativeBeatID:     strings.TrimSpace(beat.NarrativeBeatID),
			StartMs:             beat.StartMs,
			EndMs:               beat.EndMs,
			DurationClass:       normalizeVisualBeatDurationClass(beat.DurationClass),
			NarrationText:       strings.Join(narrationTexts, ""),
			Label:               strings.TrimSpace(beat.Label),
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
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" || segment.StartMs < previousEndMs || segment.EndMs <= segment.StartMs {
			return fmt.Errorf("narration timeline is invalid at segment %d", index+1)
		}
		if expectedDurationMs > 0 && segment.StartMs != previousEndMs {
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
	return prioritizedSemanticSummary(text, maxCandidateSemanticSummaryRunes)
}

func prioritizedSemanticSummary(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return ""
	}
	parts := strings.Split(text, "；")
	ordered := make([]string, 0, len(parts))
	used := make([]bool, len(parts))
	for _, prefix := range []string{"产品：", "画面描述：", "动作：", "景别：", "运镜："} {
		for index, part := range parts {
			part = strings.TrimSpace(part)
			if !used[index] && strings.HasPrefix(part, prefix) {
				ordered = append(ordered, part)
				used[index] = true
			}
		}
	}
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if !used[index] && part != "" {
			ordered = append(ordered, part)
		}
	}
	result := strings.Join(ordered, "；")
	runes := []rune(result)
	if len(runes) <= limit {
		return result
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
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

func deduplicateCandidatesByReuseKey(candidates []AssetCandidate) []AssetCandidate {
	result := make([]AssetCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate.ReuseKey = candidateReuseKey(candidate)
		if _, exists := seen[candidate.ReuseKey]; exists {
			continue
		}
		seen[candidate.ReuseKey] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func markCandidateSemanticQualification(candidates []AssetCandidate) []AssetCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	bestScore := candidates[0].SemanticScore
	for _, candidate := range candidates[1:] {
		if candidate.SemanticScore > bestScore {
			bestScore = candidate.SemanticScore
		}
	}
	minimumScore := bestScore - semanticCandidateScoreWindow
	result := append([]AssetCandidate(nil), candidates...)
	for index := range result {
		result[index].SemanticQualified = result[index].SemanticScore >= minimumScore
	}
	return result
}

func rerankCandidatesForDiversity(candidates []AssetCandidate, usage assetUsageSnapshot, diversity CandidateDiversityContext, visualKey string) []AssetCandidate {
	result := append([]AssetCandidate(nil), candidates...)
	for index := range result {
		result[index].ReuseKey = candidateReuseKey(result[index])
		result[index].BatchUseCount = usage.batch[result[index].ReuseKey]
		result[index].RecentUseCount = usage.recent[result[index].ReuseKey]
		recentPenalty := recentAssetReusePenalty * math.Log1p(float64(result[index].RecentUseCount))
		jitter := stableDiversityJitter(diversity.GenerationRunID, visualKey, result[index].ReuseKey)
		result[index].DiversityScore = result[index].SemanticScore -
			batchAssetReusePenalty*float64(result[index].BatchUseCount) - recentPenalty + jitter
		if !result[index].SemanticQualified {
			result[index].DiversityScore -= semanticFallbackDiversityPenalty
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].DiversityScore == result[j].DiversityScore {
			return result[i].ID < result[j].ID
		}
		return result[i].DiversityScore > result[j].DiversityScore
	})
	return result
}

func candidateReuseKey(candidate AssetCandidate) string {
	reuseKey := strings.TrimSpace(candidate.ReuseKey)
	if reuseKey != "" {
		return strings.ToLower(reuseKey)
	}
	return strings.ToLower(strings.TrimSpace(candidate.AssetID))
}

func stableDiversityJitter(parts ...string) float64 {
	hasher := fnv.New64a()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(strings.TrimSpace(part)))
		_, _ = hasher.Write([]byte{0})
	}
	unit := float64(hasher.Sum64()) / float64(^uint64(0))
	return (unit*2 - 1) * stableDiversityJitterAmplitude
}

func (s *AssetCandidateService) loadAssetUsage(ctx context.Context, productID string, diversity CandidateDiversityContext) (assetUsageSnapshot, error) {
	usage := assetUsageSnapshot{batch: map[string]int{}, recent: map[string]int{}}
	if s == nil || s.pool == nil {
		return usage, nil
	}
	batchID := strings.TrimSpace(diversity.GenerationBatchID)
	runID := strings.TrimSpace(diversity.GenerationRunID)
	if batchID != "" {
		rows, err := s.pool.Query(ctx, `
			SELECT selections.reuse_key, COUNT(*)::int
			FROM generation_asset_selections selections
			WHERE selections.generation_batch_id = $1::uuid
			  AND (
				NULLIF($2, '')::uuid IS NULL
				OR selections.generation_run_id <> NULLIF($2, '')::uuid
			  )
			  AND (
				selections.state = 'committed'
				OR (selections.state = 'reserved' AND selections.expires_at > now())
			  )
			GROUP BY selections.reuse_key`, batchID, runID)
		if err != nil {
			return usage, err
		}
		for rows.Next() {
			var reuseKey string
			var count int
			if err := rows.Scan(&reuseKey, &count); err != nil {
				rows.Close()
				return usage, err
			}
			usage.batch[reuseKey] = count
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return usage, err
		}
		rows.Close()
	}

	rows, err := s.pool.Query(ctx, `
		SELECT selections.reuse_key, COUNT(*)::int
		FROM generation_asset_selections selections
		JOIN generation_runs runs ON runs.id = selections.generation_run_id
		WHERE runs.product_id = $1::uuid
		  AND selections.state = 'committed'
		  AND (
			NULLIF($2, '')::uuid IS NULL
			OR selections.generation_run_id <> NULLIF($2, '')::uuid
		  )
		  AND (
			NULLIF($3, '')::uuid IS NULL
			OR selections.generation_batch_id <> NULLIF($3, '')::uuid
		  )
		  AND selections.created_at >= now() - INTERVAL '30 days'
		GROUP BY selections.reuse_key`, productID, runID, batchID)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var reuseKey string
		var count int
		if err := rows.Scan(&reuseKey, &count); err != nil {
			return usage, err
		}
		usage.recent[reuseKey] = count
	}
	return usage, rows.Err()
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
			COALESCE(NULLIF(LOWER(BTRIM(a.checksum)), ''), a.id::text),
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
		  AND a.analysis_status = 'ready'
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
			&candidate.ReuseKey,
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
