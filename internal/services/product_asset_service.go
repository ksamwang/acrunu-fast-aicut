package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrSellingPointNotFound = errors.New("selling point not found")
	ErrAssetNotFound        = errors.New("asset not found")
)

type Product struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Category    string         `json:"category,omitempty"`
	Status      string         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type SellingPoint struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Priority    int       `json:"priority"`
	Status      string    `json:"status"`
	AssetCount  int       `json:"asset_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Asset struct {
	ID                 string         `json:"id"`
	ProductID          string         `json:"product_id"`
	AssetName          string         `json:"asset_name,omitempty"`
	StorageKey         string         `json:"storage_key"`
	FileName           string         `json:"file_name"`
	FileExt            string         `json:"file_ext,omitempty"`
	MimeType           string         `json:"mime_type,omitempty"`
	FileSize           int64          `json:"file_size"`
	Checksum           string         `json:"checksum,omitempty"`
	SourceType         string         `json:"source_type"`
	IngestionSource    string         `json:"ingestion_source,omitempty"`
	DurationMs         int            `json:"duration_ms,omitempty"`
	Width              int            `json:"width,omitempty"`
	Height             int            `json:"height,omitempty"`
	FPS                float64        `json:"fps,omitempty"`
	Codec              string         `json:"codec,omitempty"`
	Status             string         `json:"status"`
	AnalysisStatus     string         `json:"analysis_status,omitempty"`
	UsabilityStatus    string         `json:"usability_status,omitempty"`
	ManualCleanStatus  string         `json:"manual_clean_status"`
	SourcePath         string         `json:"source_path,omitempty"`
	SourceOriginalName string         `json:"source_original_name,omitempty"`
	SourceInMs         int            `json:"source_in_ms,omitempty"`
	SourceOutMs        int            `json:"source_out_ms,omitempty"`
	HasAudio           bool           `json:"has_audio"`
	AudioCodec         string         `json:"audio_codec,omitempty"`
	BitrateKbps        int            `json:"bitrate_kbps,omitempty"`
	LikelyHasSpeech    bool           `json:"likely_has_speech"`
	SceneDescription   string         `json:"scene_description,omitempty"`
	ShotSize           string         `json:"shot_size,omitempty"`
	CameraMovement     string         `json:"camera_movement,omitempty"`
	Subjects           []string       `json:"subjects,omitempty"`
	SceneTags          []string       `json:"scene_tags,omitempty"`
	QualityTags        []string       `json:"quality_tags,omitempty"`
	ModelLabels        map[string]any `json:"model_labels,omitempty"`
	ModelResult        map[string]any `json:"model_result,omitempty"`
	ReviewOverrides    map[string]any `json:"review_overrides,omitempty"`
	ReviewerNotes      string         `json:"reviewer_notes,omitempty"`
	AnalysisError      string         `json:"analysis_error,omitempty"`
	CreatedByUserID    string         `json:"created_by_user_id"`
	UpdatedByUserID    string         `json:"updated_by_user_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	AnalyzedAt         *time.Time     `json:"analyzed_at,omitempty"`
	ArchivedAt         *time.Time     `json:"archived_at,omitempty"`
}

type AssetSellingPointsUpdate struct {
	SellingPointIDs []string
	UpdatedByUserID string
}

type CreateProductInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type UpdateProductInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type CreateSellingPointInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type UpdateSellingPointInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type ProductAssetService struct {
	mu                 sync.RWMutex
	products           map[string]Product
	sellingPoints      map[string]SellingPoint
	assets             map[string]Asset
	speechSegments     map[string][]repository.SpeechSegmentRecord
	assetSellingPoints map[string][]string
	queries            *db.Queries
	assetRepo          *repository.AssetRepository
}

func NewProductAssetService() *ProductAssetService {
	return &ProductAssetService{
		products:           map[string]Product{},
		sellingPoints:      map[string]SellingPoint{},
		assets:             map[string]Asset{},
		speechSegments:     map[string][]repository.SpeechSegmentRecord{},
		assetSellingPoints: map[string][]string{},
	}
}

func NewProductAssetServiceWithQueries(queries *db.Queries) *ProductAssetService {
	service := NewProductAssetService()
	service.queries = queries
	service.assetRepo = repository.NewAssetRepository(queries)
	return service
}

func (s *ProductAssetService) Queries() *db.Queries {
	return s.queries
}

type CreateAssetInput struct {
	ProductID          string
	AssetName          string
	StorageKey         string
	FileName           string
	FileExt            string
	MimeType           string
	FileSize           int64
	Checksum           string
	SourceType         string
	IngestionSource    string
	DurationMs         int
	Width              int
	Height             int
	FPS                float64
	Codec              string
	Status             string
	AnalysisStatus     string
	UsabilityStatus    string
	ManualCleanStatus  string
	SourcePath         string
	SourceOriginalName string
	SourceInMs         int
	SourceOutMs        int
	HasAudio           bool
	AudioCodec         string
	BitrateKbps        int
	LikelyHasSpeech    bool
	SceneDescription   string
	ShotSize           string
	CameraMovement     string
	Subjects           []string
	SceneTags          []string
	QualityTags        []string
	ModelLabels        map[string]any
	ModelResult        map[string]any
	ReviewerNotes      string
	AnalysisError      string
	SellingPointIDs    []string
	CreatedByUserID    string
}

type CreateSpeechSegmentInput struct {
	AssetID         string
	StartMs         int
	EndMs           int
	Transcript      string
	Confidence      *float64
	Source          string
	Status          string
	CreatedByUserID string
}

type AssetFilters struct {
	ProductID        string
	SourceType       string
	Status           string
	AnalysisStatus   string
	UsabilityStatus  string
	ShotSize         string
	SellingPointID   string
	Tag              string
	Keyword          string
	MinDurationMs    *int
	MaxDurationMs    *int
	HasAudio         *bool
	LikelyHasSpeech  *bool
	ExcludeDiscarded bool
	SortBy           string
}

type AssetFrameSnapshot struct {
	ID          string    `json:"id"`
	AssetID     string    `json:"asset_id"`
	FrameIndex  int       `json:"frame_index"`
	TimestampMs int       `json:"timestamp_ms"`
	StorageKey  string    `json:"storage_key"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type SpeechSegment struct {
	ID              string    `json:"id"`
	AssetID         string    `json:"asset_id"`
	StartMs         int       `json:"start_ms"`
	EndMs           int       `json:"end_ms"`
	Transcript      string    `json:"transcript"`
	Confidence      float64   `json:"confidence,omitempty"`
	Source          string    `json:"source"`
	Status          string    `json:"status"`
	CreatedByUserID string    `json:"created_by_user_id,omitempty"`
	UpdatedByUserID string    `json:"updated_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ProductAssetStats struct {
	ProductID            string `json:"product_id"`
	AssetCount           int    `json:"asset_count"`
	UsableAssetCount     int    `json:"usable_asset_count"`
	PendingAnalysisCount int    `json:"pending_analysis_count"`
}

type AssetAnalysisUpdate struct {
	AnalysisStatus   string
	UsabilityStatus  string
	SceneDescription string
	ShotSize         string
	CameraMovement   string
	Subjects         []string
	SceneTags        []string
	QualityTags      []string
	ModelLabels      map[string]any
	ModelResult      map[string]any
	AnalysisError    string
	AnalyzedAt       time.Time
	UpdatedByUserID  string
}

type AssetReviewUpdate struct {
	SceneDescription string
	ShotSize         string
	CameraMovement   string
	Subjects         []string
	SceneTags        []string
	QualityTags      []string
	UsabilityStatus  string
	ReviewerNotes    string
	UpdatedByUserID  string
}

type AssetArchiveUpdate struct {
	UpdatedByUserID string
}

func (s *ProductAssetService) CreateAsset(input CreateAssetInput) (Asset, error) {
	if s.queries != nil {
		return s.createAssetInPostgres(input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.products[input.ProductID]; !ok {
		return Asset{}, ErrProductNotFound
	}

	now := time.Now()
	status := input.Status
	if status == "" {
		status = "uploaded"
	}
	manualCleanStatus := input.ManualCleanStatus
	if manualCleanStatus == "" {
		manualCleanStatus = "cleaned"
	}
	assetName := input.AssetName
	if assetName == "" {
		assetName = input.FileName
	}
	ingestionSource := input.IngestionSource
	if ingestionSource == "" {
		ingestionSource = "local-agent"
	}
	analysisStatus := input.AnalysisStatus
	if analysisStatus == "" {
		analysisStatus = "pending_analysis"
	}
	usabilityStatus := input.UsabilityStatus
	if usabilityStatus == "" {
		usabilityStatus = "usable"
	}

	asset := Asset{
		ID:                 uuid.NewString(),
		ProductID:          input.ProductID,
		AssetName:          assetName,
		StorageKey:         input.StorageKey,
		FileName:           input.FileName,
		FileExt:            input.FileExt,
		MimeType:           input.MimeType,
		FileSize:           input.FileSize,
		Checksum:           input.Checksum,
		SourceType:         input.SourceType,
		IngestionSource:    ingestionSource,
		DurationMs:         input.DurationMs,
		Width:              input.Width,
		Height:             input.Height,
		FPS:                input.FPS,
		Codec:              input.Codec,
		Status:             status,
		AnalysisStatus:     analysisStatus,
		UsabilityStatus:    usabilityStatus,
		ManualCleanStatus:  manualCleanStatus,
		SourcePath:         input.SourcePath,
		SourceOriginalName: input.SourceOriginalName,
		SourceInMs:         input.SourceInMs,
		SourceOutMs:        input.SourceOutMs,
		HasAudio:           input.HasAudio,
		AudioCodec:         input.AudioCodec,
		BitrateKbps:        input.BitrateKbps,
		LikelyHasSpeech:    input.LikelyHasSpeech,
		SceneDescription:   input.SceneDescription,
		ShotSize:           input.ShotSize,
		CameraMovement:     input.CameraMovement,
		Subjects:           append([]string(nil), input.Subjects...),
		SceneTags:          append([]string(nil), input.SceneTags...),
		QualityTags:        append([]string(nil), input.QualityTags...),
		ModelLabels:        cloneObjectMap(input.ModelLabels),
		ModelResult:        cloneObjectMap(input.ModelResult),
		ReviewOverrides:    map[string]any{},
		ReviewerNotes:      input.ReviewerNotes,
		AnalysisError:      input.AnalysisError,
		CreatedByUserID:    input.CreatedByUserID,
		UpdatedByUserID:    input.CreatedByUserID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	s.assets[asset.ID] = asset
	if len(input.SellingPointIDs) > 0 {
		s.assetSellingPoints[asset.ID] = append([]string(nil), input.SellingPointIDs...)
	}
	return asset, nil
}

func (s *ProductAssetService) CreateSpeechSegment(input CreateSpeechSegmentInput) (repository.SpeechSegmentRecord, error) {
	if s.assetRepo == nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		record := repository.SpeechSegmentRecord{
			ID:              uuid.NewString(),
			AssetID:         input.AssetID,
			StartMs:         input.StartMs,
			EndMs:           input.EndMs,
			Transcript:      input.Transcript,
			Source:          firstNonEmpty(input.Source, "local-agent"),
			Status:          firstNonEmpty(input.Status, "ready"),
			CreatedByUserID: input.CreatedByUserID,
			UpdatedByUserID: input.CreatedByUserID,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if input.Confidence != nil {
			record.Confidence = *input.Confidence
		}
		s.speechSegments[input.AssetID] = append(s.speechSegments[input.AssetID], record)
		return record, nil
	}
	return s.assetRepo.CreateSpeechSegment(context.Background(), repository.CreateSpeechSegmentInput{
		AssetID:         input.AssetID,
		StartMs:         input.StartMs,
		EndMs:           input.EndMs,
		Transcript:      input.Transcript,
		Confidence:      input.Confidence,
		Source:          firstNonEmpty(input.Source, "local-agent"),
		Status:          firstNonEmpty(input.Status, "ready"),
		CreatedByUserID: input.CreatedByUserID,
	})
}

func (s *ProductAssetService) ListAssets(filters AssetFilters) []Asset {
	if s.queries != nil {
		return s.listAssetsFromPostgres(filters)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	assets := make([]Asset, 0, len(s.assets))
	for _, asset := range s.assets {
		if filters.ProductID != "" && asset.ProductID != filters.ProductID {
			continue
		}
		if filters.SourceType != "" && asset.SourceType != filters.SourceType {
			continue
		}
		if filters.Status != "" && asset.Status != filters.Status {
			continue
		}
		if filters.AnalysisStatus != "" && asset.AnalysisStatus != filters.AnalysisStatus {
			continue
		}
		if filters.UsabilityStatus != "" && asset.UsabilityStatus != filters.UsabilityStatus {
			continue
		}
		if filters.ShotSize != "" && asset.ShotSize != filters.ShotSize {
			continue
		}
		if filters.SellingPointID != "" && !containsSliceValue(s.assetSellingPoints[asset.ID], filters.SellingPointID) {
			continue
		}
		if filters.Tag != "" &&
			asset.SceneDescription != filters.Tag &&
			asset.ShotSize != filters.Tag &&
			asset.CameraMovement != filters.Tag &&
			!containsIgnoreCase(asset.SceneDescription, filters.Tag) &&
			!containsSliceValue(asset.Subjects, filters.Tag) &&
			!containsSliceValue(asset.SceneTags, filters.Tag) &&
			!containsSliceValue(asset.QualityTags, filters.Tag) {
			continue
		}
		if filters.MinDurationMs != nil && asset.DurationMs < *filters.MinDurationMs {
			continue
		}
		if filters.MaxDurationMs != nil && asset.DurationMs > *filters.MaxDurationMs {
			continue
		}
		if filters.HasAudio != nil && asset.HasAudio != *filters.HasAudio {
			continue
		}
		if filters.LikelyHasSpeech != nil && asset.LikelyHasSpeech != *filters.LikelyHasSpeech {
			continue
		}
		assets = append(assets, asset)
	}
	return postProcessAssets(assets, filters)
}

func (s *ProductAssetService) GetAsset(id string) (Asset, bool) {
	if s.queries != nil {
		return s.getAssetFromPostgres(id)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	asset, ok := s.assets[id]
	return asset, ok
}

func (s *ProductAssetService) ListAssetFrameSnapshots(assetID string) []AssetFrameSnapshot {
	if s.queries == nil || s.assetRepo == nil {
		return nil
	}
	rows, err := s.assetRepo.ListFrameSnapshotsByAsset(context.Background(), assetID)
	if err != nil {
		return nil
	}
	items := make([]AssetFrameSnapshot, 0, len(rows))
	for _, row := range rows {
		items = append(items, AssetFrameSnapshot{
			ID:          row.ID,
			AssetID:     row.AssetID,
			FrameIndex:  row.FrameIndex,
			TimestampMs: row.TimestampMs,
			StorageKey:  row.StorageKey,
			Width:       row.Width,
			Height:      row.Height,
			CreatedAt:   row.CreatedAt,
		})
	}
	return items
}

func (s *ProductAssetService) ListSpeechSegmentsByAsset(assetID string) ([]SpeechSegment, error) {
	if _, ok := s.GetAsset(assetID); !ok {
		return nil, ErrAssetNotFound
	}

	if s.assetRepo == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()

		rows := append([]repository.SpeechSegmentRecord(nil), s.speechSegments[assetID]...)
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].StartMs == rows[j].StartMs {
				if rows[i].EndMs == rows[j].EndMs {
					return rows[i].CreatedAt.Before(rows[j].CreatedAt)
				}
				return rows[i].EndMs < rows[j].EndMs
			}
			return rows[i].StartMs < rows[j].StartMs
		})

		items := make([]SpeechSegment, 0, len(rows))
		for _, row := range rows {
			items = append(items, speechSegmentFromRecord(row))
		}
		return items, nil
	}

	rows, err := s.assetRepo.ListSpeechSegmentsByAsset(context.Background(), assetID, repository.SpeechSegmentFilters{})
	if err != nil {
		return nil, err
	}
	items := make([]SpeechSegment, 0, len(rows))
	for _, row := range rows {
		items = append(items, speechSegmentFromRecord(row))
	}
	return items, nil
}

func (s *ProductAssetService) FindDuplicateAssetsByChecksum(checksum string, excludeAssetID string) []Asset {
	if checksum == "" {
		return nil
	}

	assets := s.ListAssets(AssetFilters{})
	duplicates := make([]Asset, 0)
	for _, asset := range assets {
		if asset.ID == excludeAssetID {
			continue
		}
		if !strings.EqualFold(asset.Checksum, checksum) {
			continue
		}
		duplicates = append(duplicates, asset)
	}
	return duplicates
}

func (s *ProductAssetService) ListAssetSellingPoints(assetID string) ([]SellingPoint, error) {
	asset, ok := s.GetAsset(assetID)
	if !ok {
		return nil, ErrAssetNotFound
	}

	if s.queries != nil {
		return s.listAssetSellingPointsFromPostgres(assetID, asset.ProductID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := append([]string(nil), s.assetSellingPoints[assetID]...)
	items := make([]SellingPoint, 0, len(ids))
	for _, sellingPointID := range ids {
		sellingPoint, ok := s.sellingPoints[sellingPointID]
		if !ok || sellingPoint.ProductID != asset.ProductID {
			continue
		}
		items = append(items, sellingPoint)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].Priority < items[j].Priority
	})
	return items, nil
}

func (s *ProductAssetService) UpdateAssetSellingPoints(assetID string, update AssetSellingPointsUpdate) ([]SellingPoint, error) {
	asset, ok := s.GetAsset(assetID)
	if !ok {
		return nil, ErrAssetNotFound
	}

	if s.queries != nil {
		return s.updateAssetSellingPointsInPostgres(assetID, asset.ProductID, update)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.assets[assetID]; !ok {
		return nil, ErrAssetNotFound
	}

	normalized := make([]string, 0, len(update.SellingPointIDs))
	seen := map[string]struct{}{}
	for _, sellingPointID := range update.SellingPointIDs {
		if sellingPointID == "" {
			continue
		}
		if _, exists := seen[sellingPointID]; exists {
			continue
		}
		sellingPoint, ok := s.sellingPoints[sellingPointID]
		if !ok || sellingPoint.ProductID != asset.ProductID {
			return nil, ErrSellingPointNotFound
		}
		seen[sellingPointID] = struct{}{}
		normalized = append(normalized, sellingPointID)
	}

	s.assetSellingPoints[assetID] = normalized
	assetRecord := s.assets[assetID]
	assetRecord.UpdatedByUserID = update.UpdatedByUserID
	assetRecord.UpdatedAt = time.Now()
	s.assets[assetID] = assetRecord

	items := make([]SellingPoint, 0, len(normalized))
	for _, sellingPointID := range normalized {
		items = append(items, s.sellingPoints[sellingPointID])
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].Priority < items[j].Priority
	})
	return items, nil
}

func (s *ProductAssetService) UpdateAssetAnalysis(assetID string, update AssetAnalysisUpdate) error {
	if s.queries != nil {
		return s.updateAssetAnalysisInPostgres(assetID, update)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[assetID]
	if !ok {
		return ErrAssetNotFound
	}

	asset.AnalysisStatus = update.AnalysisStatus
	asset.ModelLabels = cloneObjectMap(resolveModelLabels(update))
	asset.ModelResult = cloneObjectMap(update.ModelResult)
	asset.AnalysisError = update.AnalysisError
	asset.UpdatedByUserID = update.UpdatedByUserID
	if !update.AnalyzedAt.IsZero() {
		analyzedAt := update.AnalyzedAt
		asset.AnalyzedAt = &analyzedAt
	}
	applyAssetEffectiveLabels(&asset, asset.ModelLabels, asset.ReviewOverrides)
	asset.UpdatedAt = time.Now()
	s.assets[assetID] = asset
	return nil
}

func (s *ProductAssetService) UpdateAssetReview(assetID string, update AssetReviewUpdate) (Asset, error) {
	if s.queries != nil {
		return s.updateAssetReviewInPostgres(assetID, update)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[assetID]
	if !ok {
		return Asset{}, ErrAssetNotFound
	}

	asset.ReviewOverrides = cloneObjectMap(buildReviewOverrides(update))
	applyAssetEffectiveLabels(&asset, asset.ModelLabels, asset.ReviewOverrides)
	asset.ReviewerNotes = update.ReviewerNotes
	asset.UpdatedByUserID = update.UpdatedByUserID
	asset.UpdatedAt = time.Now()
	s.assets[assetID] = asset
	return asset, nil
}

func (s *ProductAssetService) ArchiveAsset(assetID string, update AssetArchiveUpdate) (Asset, error) {
	if s.queries != nil {
		return s.archiveAssetInPostgres(assetID, update)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[assetID]
	if !ok {
		return Asset{}, ErrAssetNotFound
	}

	now := time.Now()
	asset.Status = "archived"
	asset.UpdatedByUserID = update.UpdatedByUserID
	asset.UpdatedAt = now
	asset.ArchivedAt = &now
	s.assets[assetID] = asset
	return asset, nil
}

func (s *ProductAssetService) RestoreAsset(assetID string, update AssetArchiveUpdate) (Asset, error) {
	if s.queries != nil {
		return s.restoreAssetInPostgres(assetID, update)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[assetID]
	if !ok {
		return Asset{}, ErrAssetNotFound
	}

	asset.Status = "ready"
	asset.UpdatedByUserID = update.UpdatedByUserID
	asset.UpdatedAt = time.Now()
	asset.ArchivedAt = nil
	s.assets[assetID] = asset
	return asset, nil
}

func (s *ProductAssetService) CreateProduct(input CreateProductInput) Product {
	if s.queries != nil {
		return s.createProductInPostgres(input)
	}

	now := time.Now()
	product := Product{
		ID:          uuid.NewString(),
		Name:        input.Name,
		Description: input.Description,
		Category:    input.Category,
		Status:      "active",
		Metadata:    input.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[product.ID] = product
	return product
}

func (s *ProductAssetService) ListProducts() []Product {
	if s.queries != nil {
		return s.listProductsFromPostgres()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]Product, 0, len(s.products))
	for _, product := range s.products {
		products = append(products, product)
	}
	sort.Slice(products, func(i, j int) bool {
		return products[i].CreatedAt.After(products[j].CreatedAt)
	})
	return products
}

func (s *ProductAssetService) GetProductAssetStats(productID string) (ProductAssetStats, error) {
	if _, err := s.GetProduct(productID); err != nil {
		return ProductAssetStats{}, err
	}

	assets := s.ListAssets(AssetFilters{ProductID: productID})
	stats := ProductAssetStats{ProductID: productID}
	for _, asset := range assets {
		if asset.Status == "archived" {
			continue
		}
		stats.AssetCount++
		if asset.UsabilityStatus == "usable" {
			stats.UsableAssetCount++
		}
		if asset.AnalysisStatus == "pending_analysis" || asset.AnalysisStatus == "analyzing" {
			stats.PendingAnalysisCount++
		}
	}
	return stats, nil
}

func (s *ProductAssetService) GetProduct(id string) (Product, error) {
	if s.queries != nil {
		return s.getProductFromPostgres(id)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	product, ok := s.products[id]
	if !ok {
		return Product{}, ErrProductNotFound
	}
	return product, nil
}

func (s *ProductAssetService) UpdateProduct(id string, input UpdateProductInput) (Product, error) {
	if s.queries != nil {
		return s.updateProductInPostgres(id, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	product, ok := s.products[id]
	if !ok {
		return Product{}, ErrProductNotFound
	}

	product.Name = input.Name
	product.Description = input.Description
	product.Category = input.Category
	product.Metadata = input.Metadata
	product.UpdatedAt = time.Now()
	s.products[id] = product
	return product, nil
}

func (s *ProductAssetService) ArchiveProduct(id string) error {
	if s.queries != nil {
		return s.archiveProductInPostgres(id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	product, ok := s.products[id]
	if !ok {
		return ErrProductNotFound
	}

	product.Status = "archived"
	product.UpdatedAt = time.Now()
	s.products[id] = product
	return nil
}

func (s *ProductAssetService) CreateSellingPoint(productID string, input CreateSellingPointInput) (SellingPoint, error) {
	if s.queries != nil {
		return s.createSellingPointInPostgres(productID, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.products[productID]; !ok {
		return SellingPoint{}, ErrProductNotFound
	}

	now := time.Now()
	sellingPoint := SellingPoint{
		ID:          uuid.NewString(),
		ProductID:   productID,
		Title:       input.Title,
		Description: input.Description,
		Priority:    input.Priority,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.sellingPoints[sellingPoint.ID] = sellingPoint
	return sellingPoint, nil
}

func (s *ProductAssetService) ListSellingPoints(productID string) []SellingPoint {
	if s.queries != nil {
		return s.listSellingPointsFromPostgres(productID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []SellingPoint{}
	for _, sellingPoint := range s.sellingPoints {
		if sellingPoint.ProductID == productID {
			items = append(items, sellingPoint)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Priority > items[j].Priority
	})
	return s.decorateSellingPointsWithAssetCounts(items)
}

func (s *ProductAssetService) ListAssetsBySellingPoint(sellingPointID string) ([]Asset, error) {
	if sellingPointID == "" {
		return nil, ErrSellingPointNotFound
	}

	if s.queries == nil {
		s.mu.RLock()
		_, ok := s.sellingPoints[sellingPointID]
		s.mu.RUnlock()
		if !ok {
			return nil, ErrSellingPointNotFound
		}
	} else {
		if _, err := s.getSellingPointByIDFromPostgres(sellingPointID); err != nil {
			return nil, err
		}
	}

	assets := s.ListAssets(AssetFilters{SellingPointID: sellingPointID})
	filtered := make([]Asset, 0, len(assets))
	for _, asset := range assets {
		if asset.Status == "archived" {
			continue
		}
		filtered = append(filtered, asset)
	}
	return filtered, nil
}

func (s *ProductAssetService) UpdateSellingPoint(id string, input UpdateSellingPointInput) (SellingPoint, error) {
	if s.queries != nil {
		return s.updateSellingPointInPostgres(id, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sellingPoint, ok := s.sellingPoints[id]
	if !ok {
		return SellingPoint{}, ErrSellingPointNotFound
	}

	sellingPoint.Title = input.Title
	sellingPoint.Description = input.Description
	sellingPoint.Priority = input.Priority
	sellingPoint.UpdatedAt = time.Now()
	s.sellingPoints[id] = sellingPoint
	return sellingPoint, nil
}

func (s *ProductAssetService) ArchiveSellingPoint(id string) error {
	if s.queries != nil {
		return s.archiveSellingPointInPostgres(id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sellingPoint, ok := s.sellingPoints[id]
	if !ok {
		return ErrSellingPointNotFound
	}

	sellingPoint.Status = "archived"
	sellingPoint.UpdatedAt = time.Now()
	s.sellingPoints[id] = sellingPoint
	return nil
}

func (s *ProductAssetService) createProductInPostgres(input CreateProductInput) Product {
	row, err := s.queries.CreateProduct(context.Background(), db.CreateProductParams{
		Name:            input.Name,
		Description:     assetTextParam(input.Description),
		Category:        assetTextParam(input.Category),
		Status:          "active",
		Metadata:        mustJSON(input.Metadata, map[string]any{}),
		CreatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		return Product{}
	}
	return productFromDB(row)
}

func (s *ProductAssetService) listProductsFromPostgres() []Product {
	rows, err := s.queries.ListProducts(context.Background(), pgtype.Text{})
	if err != nil {
		return nil
	}
	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		items = append(items, productFromDB(row))
	}
	return items
}

func (s *ProductAssetService) getProductFromPostgres(id string) (Product, error) {
	row, err := s.queries.GetProductByID(context.Background(), assetNullableUUIDParam(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, err
	}
	return productFromDB(row), nil
}

func (s *ProductAssetService) updateProductInPostgres(id string, input UpdateProductInput) (Product, error) {
	row, err := s.queries.UpdateProduct(context.Background(), db.UpdateProductParams{
		ID:              assetNullableUUIDParam(id),
		Name:            input.Name,
		Description:     assetTextParam(input.Description),
		Category:        assetTextParam(input.Category),
		Metadata:        mustJSON(input.Metadata, map[string]any{}),
		UpdatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, err
	}
	return productFromDB(row), nil
}

func (s *ProductAssetService) archiveProductInPostgres(id string) error {
	err := s.queries.ArchiveProduct(context.Background(), db.ArchiveProductParams{
		ID:              assetNullableUUIDParam(id),
		UpdatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProductNotFound
		}
	}
	return err
}

func (s *ProductAssetService) createSellingPointInPostgres(productID string, input CreateSellingPointInput) (SellingPoint, error) {
	row, err := s.queries.CreateSellingPoint(context.Background(), db.CreateSellingPointParams{
		ProductID:       assetNullableUUIDParam(productID),
		Title:           input.Title,
		Description:     assetTextParam(input.Description),
		Priority:        int32(input.Priority),
		Status:          "active",
		CreatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		return SellingPoint{}, err
	}
	return sellingPointFromDB(row), nil
}

func (s *ProductAssetService) listSellingPointsFromPostgres(productID string) []SellingPoint {
	rows, err := s.queries.ListSellingPointsByProduct(context.Background(), db.ListSellingPointsByProductParams{
		ProductID: assetNullableUUIDParam(productID),
		Status:    pgtype.Text{},
	})
	if err != nil {
		return nil
	}
	items := make([]SellingPoint, 0, len(rows))
	for _, row := range rows {
		items = append(items, sellingPointFromDB(row))
	}
	return s.decorateSellingPointsWithAssetCounts(items)
}

func (s *ProductAssetService) getSellingPointByIDFromPostgres(id string) (SellingPoint, error) {
	row, err := s.queries.GetSellingPointByID(context.Background(), assetNullableUUIDParam(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellingPoint{}, ErrSellingPointNotFound
		}
		return SellingPoint{}, err
	}
	return sellingPointFromDB(row), nil
}

func (s *ProductAssetService) updateSellingPointInPostgres(id string, input UpdateSellingPointInput) (SellingPoint, error) {
	row, err := s.queries.UpdateSellingPoint(context.Background(), db.UpdateSellingPointParams{
		ID:              assetNullableUUIDParam(id),
		Title:           input.Title,
		Description:     assetTextParam(input.Description),
		Priority:        int32(input.Priority),
		UpdatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellingPoint{}, ErrSellingPointNotFound
		}
		return SellingPoint{}, err
	}
	return sellingPointFromDB(row), nil
}

func (s *ProductAssetService) archiveSellingPointInPostgres(id string) error {
	err := s.queries.ArchiveSellingPoint(context.Background(), db.ArchiveSellingPointParams{
		ID:              assetNullableUUIDParam(id),
		UpdatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSellingPointNotFound
		}
	}
	return err
}

func (s *ProductAssetService) createAssetInPostgres(input CreateAssetInput) (Asset, error) {
	subjectsJSON := mustJSON(input.Subjects, []string{})
	sceneTagsJSON := mustJSON(input.SceneTags, []string{})
	qualityTagsJSON := mustJSON(input.QualityTags, []string{})
	modelLabelsJSON := mustJSON(input.ModelLabels, map[string]any{})
	modelResultJSON := mustJSON(input.ModelResult, map[string]any{})

	row, err := s.queries.CreateAsset(context.Background(), db.CreateAssetParams{
		ProductID:          assetNullableUUIDParam(input.ProductID),
		AssetName:          assetTextParam(firstNonEmpty(input.AssetName, input.FileName)),
		StorageKey:         input.StorageKey,
		FileName:           input.FileName,
		FileExt:            assetTextParam(input.FileExt),
		MimeType:           assetTextParam(input.MimeType),
		FileSize:           input.FileSize,
		Checksum:           assetTextParam(input.Checksum),
		SourceType:         input.SourceType,
		IngestionSource:    firstNonEmpty(input.IngestionSource, "local-agent"),
		DurationMs:         int4Param(input.DurationMs),
		Width:              int4Param(input.Width),
		Height:             int4Param(input.Height),
		Fps:                numericParam(input.FPS),
		Codec:              assetTextParam(input.Codec),
		Status:             firstNonEmpty(input.Status, "ready"),
		AnalysisStatus:     firstNonEmpty(input.AnalysisStatus, "pending_analysis"),
		UsabilityStatus:    firstNonEmpty(input.UsabilityStatus, "usable"),
		ManualCleanStatus:  firstNonEmpty(input.ManualCleanStatus, "cleaned"),
		SourcePath:         assetTextParam(input.SourcePath),
		SourceOriginalName: assetTextParam(firstNonEmpty(input.SourceOriginalName, input.FileName)),
		SourceInMs:         int4Param(input.SourceInMs),
		SourceOutMs:        int4Param(input.SourceOutMs),
		HasAudio:           input.HasAudio,
		AudioCodec:         assetTextParam(input.AudioCodec),
		BitrateKbps:        int4Param(input.BitrateKbps),
		LikelyHasSpeech:    input.LikelyHasSpeech,
		SceneDescription:   assetTextParam(input.SceneDescription),
		ShotSize:           assetTextParam(input.ShotSize),
		CameraMovement:     assetTextParam(input.CameraMovement),
		Subjects:           subjectsJSON,
		SceneTags:          sceneTagsJSON,
		QualityTags:        qualityTagsJSON,
		ModelLabels:        modelLabelsJSON,
		ModelResult:        modelResultJSON,
		ReviewOverrides:    []byte(`{}`),
		ReviewerNotes:      assetTextParam(input.ReviewerNotes),
		AnalysisError:      assetTextParam(input.AnalysisError),
		AnalyzedAt:         pgtype.Timestamptz{},
		Metadata:           []byte(`{}`),
		CreatedByUserID:    assetNullableUUIDParam(input.CreatedByUserID),
		UpdatedByUserID:    assetNullableUUIDParam(input.CreatedByUserID),
	})
	if err != nil {
		if isForeignKeyProductError(err) {
			return Asset{}, ErrProductNotFound
		}
		return Asset{}, err
	}
	for _, sellingPointID := range input.SellingPointIDs {
		if sellingPointID == "" {
			continue
		}
		if addErr := s.assetRepo.AddSellingPoint(context.Background(), assetUUIDString(row.ID), sellingPointID); addErr != nil {
			return Asset{}, addErr
		}
	}
	return assetFromDBRow(row), nil
}

func (s *ProductAssetService) listAssetsFromPostgres(filters AssetFilters) []Asset {
	if s.assetRepo == nil {
		return nil
	}
	rows, err := s.assetRepo.List(context.Background(), repository.AssetFilters{
		ProductID:       filters.ProductID,
		SourceType:      filters.SourceType,
		Status:          filters.Status,
		AnalysisStatus:  filters.AnalysisStatus,
		UsabilityStatus: filters.UsabilityStatus,
		ShotSize:        filters.ShotSize,
		SellingPointID:  filters.SellingPointID,
		Tag:             filters.Tag,
		MinDurationMs:   filters.MinDurationMs,
		MaxDurationMs:   filters.MaxDurationMs,
		HasAudio:        filters.HasAudio,
		LikelyHasSpeech: filters.LikelyHasSpeech,
	})
	if err != nil {
		return nil
	}
	items := make([]Asset, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetFromDBRecord(row))
	}
	return postProcessAssets(items, filters)
}

func postProcessAssets(items []Asset, filters AssetFilters) []Asset {
	filtered := make([]Asset, 0, len(items))
	for _, asset := range items {
		if filters.ExcludeDiscarded && strings.EqualFold(asset.UsabilityStatus, "discarded") {
			continue
		}
		if filters.Keyword != "" && !containsIgnoreCase(asset.SceneDescription, filters.Keyword) {
			continue
		}
		filtered = append(filtered, asset)
	}

	sort.Slice(filtered, func(i, j int) bool {
		switch filters.SortBy {
		case "updated_at_desc":
			if filtered[i].UpdatedAt.Equal(filtered[j].UpdatedAt) {
				return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
			}
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		case "analyzed_at_desc":
			left := derefTime(filtered[i].AnalyzedAt)
			right := derefTime(filtered[j].AnalyzedAt)
			if left.Equal(right) {
				return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
			}
			return left.After(right)
		default:
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		}
	})

	return filtered
}

func (s *ProductAssetService) getAssetFromPostgres(id string) (Asset, bool) {
	if s.assetRepo == nil {
		return Asset{}, false
	}
	row, err := s.assetRepo.GetByID(context.Background(), id)
	if err != nil {
		return Asset{}, false
	}
	return assetFromDBRecord(row), true
}

func (s *ProductAssetService) updateAssetAnalysisInPostgres(assetID string, update AssetAnalysisUpdate) error {
	current, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return ErrAssetNotFound
	}
	modelLabels := resolveModelLabels(update)
	effective := mergeLabelMaps(modelLabels, current.ReviewOverrides)

	return s.queries.UpdateAssetAnalysis(context.Background(), db.UpdateAssetAnalysisParams{
		ID:               assetNullableUUIDParam(assetID),
		AnalysisStatus:   firstNonEmpty(update.AnalysisStatus, "ready"),
		UsabilityStatus:  firstNonEmpty(stringValueFromMap(effective, "usability_status"), "usable"),
		SceneDescription: assetTextParam(stringValueFromMap(effective, "scene_description")),
		ShotSize:         assetTextParam(stringValueFromMap(effective, "shot_size")),
		CameraMovement:   assetTextParam(stringValueFromMap(effective, "camera_movement")),
		Subjects:         mustJSON(stringSliceValueFromMap(effective, "subjects"), []string{}),
		SceneTags:        mustJSON(stringSliceValueFromMap(effective, "scene_tags"), []string{}),
		QualityTags:      mustJSON(stringSliceValueFromMap(effective, "quality_tags"), []string{}),
		ModelLabels:      mustJSON(modelLabels, map[string]any{}),
		ModelResult:      mustJSON(update.ModelResult, map[string]any{}),
		AnalysisError:    assetTextParam(update.AnalysisError),
		AnalyzedAt:       pgtype.Timestamptz{Time: update.AnalyzedAt, Valid: !update.AnalyzedAt.IsZero()},
		UpdatedByUserID:  assetNullableUUIDParam(update.UpdatedByUserID),
	})
}

func (s *ProductAssetService) updateAssetReviewInPostgres(assetID string, update AssetReviewUpdate) (Asset, error) {
	current, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return Asset{}, ErrAssetNotFound
	}

	reviewOverrides := buildReviewOverrides(update)
	effective := mergeLabelMaps(current.ModelLabels, reviewOverrides)

	err := s.queries.UpdateAssetAnalysis(context.Background(), db.UpdateAssetAnalysisParams{
		ID:               assetNullableUUIDParam(assetID),
		AnalysisStatus:   firstNonEmpty(current.AnalysisStatus, "ready"),
		UsabilityStatus:  firstNonEmpty(stringValueFromMap(effective, "usability_status"), current.UsabilityStatus),
		SceneDescription: assetTextParam(stringValueFromMap(effective, "scene_description")),
		ShotSize:         assetTextParam(stringValueFromMap(effective, "shot_size")),
		CameraMovement:   assetTextParam(stringValueFromMap(effective, "camera_movement")),
		Subjects:         mustJSON(stringSliceValueFromMap(effective, "subjects"), []string{}),
		SceneTags:        mustJSON(stringSliceValueFromMap(effective, "scene_tags"), []string{}),
		QualityTags:      mustJSON(stringSliceValueFromMap(effective, "quality_tags"), []string{}),
		ModelLabels:      mustJSON(current.ModelLabels, map[string]any{}),
		ModelResult:      mustJSON(current.ModelResult, map[string]any{}),
		AnalysisError:    assetTextParam(current.AnalysisError),
		AnalyzedAt:       pgtype.Timestamptz{Time: derefTime(current.AnalyzedAt), Valid: current.AnalyzedAt != nil},
		UpdatedByUserID:  assetNullableUUIDParam(update.UpdatedByUserID),
	})
	if err != nil {
		return Asset{}, err
	}

	if err := s.queries.UpdateAssetReview(context.Background(), db.UpdateAssetReviewParams{
		ID:              assetNullableUUIDParam(assetID),
		ReviewerNotes:   assetTextParam(update.ReviewerNotes),
		ReviewOverrides: mustJSON(reviewOverrides, map[string]any{}),
		UsabilityStatus: firstNonEmpty(stringValueFromMap(effective, "usability_status"), current.UsabilityStatus),
		UpdatedByUserID: assetNullableUUIDParam(update.UpdatedByUserID),
	}); err != nil {
		return Asset{}, err
	}

	updated, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return Asset{}, ErrAssetNotFound
	}
	return updated, nil
}

func (s *ProductAssetService) archiveAssetInPostgres(assetID string, update AssetArchiveUpdate) (Asset, error) {
	if err := s.queries.ArchiveAsset(context.Background(), db.ArchiveAssetParams{
		ID:              assetNullableUUIDParam(assetID),
		UpdatedByUserID: assetNullableUUIDParam(update.UpdatedByUserID),
	}); err != nil {
		return Asset{}, err
	}

	updated, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return Asset{}, ErrAssetNotFound
	}
	return updated, nil
}

func (s *ProductAssetService) restoreAssetInPostgres(assetID string, update AssetArchiveUpdate) (Asset, error) {
	if err := s.queries.RestoreAsset(context.Background(), db.RestoreAssetParams{
		ID:              assetNullableUUIDParam(assetID),
		UpdatedByUserID: assetNullableUUIDParam(update.UpdatedByUserID),
	}); err != nil {
		return Asset{}, err
	}

	updated, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return Asset{}, ErrAssetNotFound
	}
	return updated, nil
}

func (s *ProductAssetService) listAssetSellingPointsFromPostgres(assetID string, productID string) ([]SellingPoint, error) {
	if s.assetRepo == nil {
		return nil, nil
	}

	ids, err := s.assetRepo.ListSellingPointIDs(context.Background(), assetID)
	if err != nil {
		return nil, err
	}

	all := s.listSellingPointsFromPostgres(productID)
	index := make(map[string]SellingPoint, len(all))
	for _, item := range all {
		index[item.ID] = item
	}

	items := make([]SellingPoint, 0, len(ids))
	for _, id := range ids {
		if item, ok := index[id]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *ProductAssetService) updateAssetSellingPointsInPostgres(assetID string, productID string, update AssetSellingPointsUpdate) ([]SellingPoint, error) {
	if s.assetRepo == nil {
		return nil, nil
	}

	asset, ok := s.getAssetFromPostgres(assetID)
	if !ok {
		return nil, ErrAssetNotFound
	}

	currentIDs, err := s.assetRepo.ListSellingPointIDs(context.Background(), assetID)
	if err != nil {
		return nil, err
	}

	available := s.listSellingPointsFromPostgres(productID)
	availableMap := make(map[string]SellingPoint, len(available))
	for _, item := range available {
		availableMap[item.ID] = item
	}

	desiredIDs := make([]string, 0, len(update.SellingPointIDs))
	desiredSet := map[string]struct{}{}
	for _, sellingPointID := range update.SellingPointIDs {
		if sellingPointID == "" {
			continue
		}
		if _, exists := desiredSet[sellingPointID]; exists {
			continue
		}
		if _, ok := availableMap[sellingPointID]; !ok {
			return nil, ErrSellingPointNotFound
		}
		desiredSet[sellingPointID] = struct{}{}
		desiredIDs = append(desiredIDs, sellingPointID)
	}

	currentSet := map[string]struct{}{}
	for _, id := range currentIDs {
		currentSet[id] = struct{}{}
	}

	for _, id := range currentIDs {
		if _, keep := desiredSet[id]; !keep {
			if err := s.assetRepo.RemoveSellingPoint(context.Background(), assetID, id); err != nil {
				return nil, err
			}
		}
	}

	for _, id := range desiredIDs {
		if _, exists := currentSet[id]; !exists {
			if err := s.assetRepo.AddSellingPoint(context.Background(), assetID, id); err != nil {
				return nil, err
			}
		}
	}

	if err := s.queries.UpdateAssetStatus(context.Background(), db.UpdateAssetStatusParams{
		ID:              assetNullableUUIDParam(assetID),
		Status:          asset.Status,
		UpdatedByUserID: assetNullableUUIDParam(update.UpdatedByUserID),
	}); err != nil {
		return nil, err
	}

	items := make([]SellingPoint, 0, len(desiredIDs))
	for _, id := range desiredIDs {
		items = append(items, availableMap[id])
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].Priority < items[j].Priority
	})
	return items, nil
}

func AssetAnalysisUpdateFromResult(result modelgateway.AnalyzeAssetResult, analysisStatus string, analyzedAt time.Time) AssetAnalysisUpdate {
	return AssetAnalysisUpdate{
		AnalysisStatus:   analysisStatus,
		UsabilityStatus:  result.UsabilityStatus,
		SceneDescription: result.SceneDescription,
		ShotSize:         result.ShotSize,
		CameraMovement:   result.CameraMovement,
		Subjects:         append([]string(nil), result.Subjects...),
		SceneTags:        append([]string(nil), result.SceneTags...),
		QualityTags:      append([]string(nil), result.QualityTags...),
		ModelLabels:      modelLabelsFromResult(result),
		ModelResult:      result.ModelResult,
		AnalyzedAt:       analyzedAt,
	}
}

func productFromDB(row db.Product) Product {
	return Product{
		ID:          assetUUIDString(row.ID),
		Name:        row.Name,
		Description: assetTextString(row.Description),
		Category:    assetTextString(row.Category),
		Status:      row.Status,
		Metadata:    jsonObject(row.Metadata),
		CreatedAt:   timeValue(row.CreatedAt),
		UpdatedAt:   timeValue(row.UpdatedAt),
	}
}

func sellingPointFromDB(row db.ProductSellingPoint) SellingPoint {
	return SellingPoint{
		ID:          assetUUIDString(row.ID),
		ProductID:   assetUUIDString(row.ProductID),
		Title:       row.Title,
		Description: assetTextString(row.Description),
		Priority:    int(row.Priority),
		Status:      row.Status,
		CreatedAt:   timeValue(row.CreatedAt),
		UpdatedAt:   timeValue(row.UpdatedAt),
	}
}

func (s *ProductAssetService) decorateSellingPointsWithAssetCounts(items []SellingPoint) []SellingPoint {
	if len(items) == 0 {
		return items
	}

	for i := range items {
		assets := s.ListAssets(AssetFilters{SellingPointID: items[i].ID})
		count := 0
		for _, asset := range assets {
			if asset.Status == "archived" {
				continue
			}
			count++
		}
		items[i].AssetCount = count
	}

	return items
}

func assetFromDBRecord(row repository.AssetRecord) Asset {
	return Asset{
		ID:                 row.ID,
		ProductID:          row.ProductID,
		AssetName:          row.AssetName,
		StorageKey:         row.StorageKey,
		FileName:           row.FileName,
		FileExt:            row.FileExt,
		MimeType:           row.MimeType,
		FileSize:           row.FileSize,
		Checksum:           row.Checksum,
		SourceType:         row.SourceType,
		IngestionSource:    row.IngestionSource,
		DurationMs:         row.DurationMs,
		Width:              row.Width,
		Height:             row.Height,
		FPS:                row.FPS,
		Codec:              row.Codec,
		Status:             row.Status,
		AnalysisStatus:     row.AnalysisStatus,
		UsabilityStatus:    row.UsabilityStatus,
		ManualCleanStatus:  row.ManualCleanStatus,
		SourcePath:         row.SourcePath,
		SourceOriginalName: row.SourceOriginalName,
		SourceInMs:         row.SourceInMs,
		SourceOutMs:        row.SourceOutMs,
		HasAudio:           row.HasAudio,
		AudioCodec:         row.AudioCodec,
		BitrateKbps:        row.BitrateKbps,
		LikelyHasSpeech:    row.LikelyHasSpeech,
		SceneDescription:   row.SceneDescription,
		ShotSize:           row.ShotSize,
		CameraMovement:     row.CameraMovement,
		Subjects:           decodeStringList(row.Subjects),
		SceneTags:          decodeStringList(row.SceneTags),
		QualityTags:        decodeStringList(row.QualityTags),
		ModelLabels:        jsonObject(row.ModelLabels),
		ModelResult:        jsonObject(row.ModelResult),
		ReviewOverrides:    jsonObject(row.ReviewOverrides),
		ReviewerNotes:      row.ReviewerNotes,
		AnalysisError:      row.AnalysisError,
		CreatedByUserID:    row.CreatedByUserID,
		UpdatedByUserID:    row.UpdatedByUserID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		AnalyzedAt:         row.AnalyzedAt,
		ArchivedAt:         row.ArchivedAt,
	}
}

func speechSegmentFromRecord(row repository.SpeechSegmentRecord) SpeechSegment {
	return SpeechSegment{
		ID:              row.ID,
		AssetID:         row.AssetID,
		StartMs:         row.StartMs,
		EndMs:           row.EndMs,
		Transcript:      row.Transcript,
		Confidence:      row.Confidence,
		Source:          row.Source,
		Status:          row.Status,
		CreatedByUserID: row.CreatedByUserID,
		UpdatedByUserID: row.UpdatedByUserID,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func assetFromDBRow(row db.Asset) Asset {
	return Asset{
		ID:                 assetUUIDString(row.ID),
		ProductID:          assetUUIDString(row.ProductID),
		AssetName:          assetTextString(row.AssetName),
		StorageKey:         row.StorageKey,
		FileName:           row.FileName,
		FileExt:            assetTextString(row.FileExt),
		MimeType:           assetTextString(row.MimeType),
		FileSize:           row.FileSize,
		Checksum:           assetTextString(row.Checksum),
		SourceType:         row.SourceType,
		IngestionSource:    row.IngestionSource,
		DurationMs:         int4Value(row.DurationMs),
		Width:              int4Value(row.Width),
		Height:             int4Value(row.Height),
		FPS:                numericValue(row.Fps),
		Codec:              assetTextString(row.Codec),
		Status:             row.Status,
		AnalysisStatus:     row.AnalysisStatus,
		UsabilityStatus:    row.UsabilityStatus,
		ManualCleanStatus:  row.ManualCleanStatus,
		SourcePath:         assetTextString(row.SourcePath),
		SourceOriginalName: assetTextString(row.SourceOriginalName),
		SourceInMs:         int4Value(row.SourceInMs),
		SourceOutMs:        int4Value(row.SourceOutMs),
		HasAudio:           row.HasAudio,
		AudioCodec:         assetTextString(row.AudioCodec),
		BitrateKbps:        int4Value(row.BitrateKbps),
		LikelyHasSpeech:    row.LikelyHasSpeech,
		SceneDescription:   assetTextString(row.SceneDescription),
		ShotSize:           assetTextString(row.ShotSize),
		CameraMovement:     assetTextString(row.CameraMovement),
		Subjects:           decodeStringList(row.Subjects),
		SceneTags:          decodeStringList(row.SceneTags),
		QualityTags:        decodeStringList(row.QualityTags),
		ModelLabels:        jsonObject(row.ModelLabels),
		ModelResult:        jsonObject(row.ModelResult),
		ReviewOverrides:    jsonObject(row.ReviewOverrides),
		ReviewerNotes:      assetTextString(row.ReviewerNotes),
		AnalysisError:      assetTextString(row.AnalysisError),
		CreatedByUserID:    assetUUIDString(row.CreatedByUserID),
		UpdatedByUserID:    assetUUIDString(row.UpdatedByUserID),
		CreatedAt:          timeValue(row.CreatedAt),
		UpdatedAt:          timeValue(row.UpdatedAt),
		AnalyzedAt:         optionalTime(row.AnalyzedAt),
		ArchivedAt:         optionalTime(row.ArchivedAt),
	}
}

func assetUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func assetNullableUUIDParam(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}
	}
	return id
}

func assetTextParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func assetTextString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func int4Param(value int) pgtype.Int4 {
	if value == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(value), Valid: true}
}

func numericParam(value float64) pgtype.Numeric {
	if value == 0 {
		return pgtype.Numeric{}
	}
	var numeric pgtype.Numeric
	_ = numeric.Scan(value)
	return numeric
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func int4Value(value pgtype.Int4) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int32)
}

func numericValue(value pgtype.Numeric) float64 {
	if !value.Valid {
		return 0
	}
	number, err := value.Float64Value()
	if err != nil || !number.Valid {
		return 0
	}
	return number.Float64
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func jsonObject(value []byte) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(value, &decoded); err != nil || decoded == nil {
		return map[string]any{}
	}
	return decoded
}

func cloneObjectMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		switch typed := item.(type) {
		case []string:
			cloned[key] = append([]string(nil), typed...)
		case []any:
			copied := make([]any, len(typed))
			copy(copied, typed)
			cloned[key] = copied
		default:
			cloned[key] = typed
		}
	}
	return cloned
}

func modelLabelsFromResult(result modelgateway.AnalyzeAssetResult) map[string]any {
	return map[string]any{
		"scene_description": result.SceneDescription,
		"shot_size":         result.ShotSize,
		"camera_movement":   result.CameraMovement,
		"subjects":          append([]string(nil), result.Subjects...),
		"scene_tags":        append([]string(nil), result.SceneTags...),
		"quality_tags":      append([]string(nil), result.QualityTags...),
		"usability_status":  result.UsabilityStatus,
	}
}

func resolveModelLabels(update AssetAnalysisUpdate) map[string]any {
	if len(update.ModelLabels) > 0 {
		return cloneObjectMap(update.ModelLabels)
	}
	return map[string]any{
		"scene_description": update.SceneDescription,
		"shot_size":         update.ShotSize,
		"camera_movement":   update.CameraMovement,
		"subjects":          append([]string(nil), update.Subjects...),
		"scene_tags":        append([]string(nil), update.SceneTags...),
		"quality_tags":      append([]string(nil), update.QualityTags...),
		"usability_status":  update.UsabilityStatus,
	}
}

func buildReviewOverrides(update AssetReviewUpdate) map[string]any {
	return map[string]any{
		"scene_description": update.SceneDescription,
		"shot_size":         update.ShotSize,
		"camera_movement":   update.CameraMovement,
		"subjects":          append([]string(nil), update.Subjects...),
		"scene_tags":        append([]string(nil), update.SceneTags...),
		"quality_tags":      append([]string(nil), update.QualityTags...),
		"usability_status":  update.UsabilityStatus,
	}
}

func mergeLabelMaps(base map[string]any, override map[string]any) map[string]any {
	merged := cloneObjectMap(base)
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func applyAssetEffectiveLabels(asset *Asset, modelLabels map[string]any, reviewOverrides map[string]any) {
	effective := mergeLabelMaps(modelLabels, reviewOverrides)
	asset.SceneDescription = stringValueFromMap(effective, "scene_description")
	asset.ShotSize = stringValueFromMap(effective, "shot_size")
	asset.CameraMovement = stringValueFromMap(effective, "camera_movement")
	asset.Subjects = stringSliceValueFromMap(effective, "subjects")
	asset.SceneTags = stringSliceValueFromMap(effective, "scene_tags")
	asset.QualityTags = stringSliceValueFromMap(effective, "quality_tags")
	if value := stringValueFromMap(effective, "usability_status"); value != "" {
		asset.UsabilityStatus = value
	}
}

func stringValueFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return value
	default:
		return ""
	}
}

func stringSliceValueFromMap(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	switch value := values[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			items = append(items, text)
		}
		return items
	default:
		return nil
	}
}

func decodeStringList(value []byte) []string {
	if len(value) == 0 {
		return nil
	}
	var decoded []string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil
	}
	return decoded
}

func mustJSON(value any, fallback any) []byte {
	if value == nil {
		value = fallback
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded, _ = json.Marshal(fallback)
		return encoded
	}
	return encoded
}

func firstNonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func isForeignKeyProductError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func containsIgnoreCase(value string, keyword string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(keyword))
}

func containsSliceValue(values []string, keyword string) bool {
	for _, value := range values {
		if strings.EqualFold(value, keyword) {
			return true
		}
	}
	return false
}
