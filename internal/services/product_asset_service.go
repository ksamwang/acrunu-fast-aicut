package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrSellingPointNotFound = errors.New("selling point not found")
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
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Asset struct {
	ID                string    `json:"id"`
	ProductID         string    `json:"product_id"`
	AssetName         string    `json:"asset_name,omitempty"`
	StorageKey        string    `json:"storage_key"`
	FileName          string    `json:"file_name"`
	FileExt           string    `json:"file_ext,omitempty"`
	MimeType          string    `json:"mime_type,omitempty"`
	FileSize          int64     `json:"file_size"`
	Checksum          string    `json:"checksum,omitempty"`
	SourceType        string    `json:"source_type"`
	IngestionSource   string    `json:"ingestion_source,omitempty"`
	DurationMs        int       `json:"duration_ms,omitempty"`
	Width             int       `json:"width,omitempty"`
	Height            int       `json:"height,omitempty"`
	FPS               float64   `json:"fps,omitempty"`
	Codec             string    `json:"codec,omitempty"`
	Status            string    `json:"status"`
	AnalysisStatus    string    `json:"analysis_status,omitempty"`
	UsabilityStatus   string    `json:"usability_status,omitempty"`
	ManualCleanStatus string    `json:"manual_clean_status"`
	SourcePath        string    `json:"source_path,omitempty"`
	SourceOriginalName string   `json:"source_original_name,omitempty"`
	SourceInMs        int       `json:"source_in_ms,omitempty"`
	SourceOutMs       int       `json:"source_out_ms,omitempty"`
	HasAudio          bool      `json:"has_audio"`
	AudioCodec        string    `json:"audio_codec,omitempty"`
	BitrateKbps       int       `json:"bitrate_kbps,omitempty"`
	SceneDescription  string    `json:"scene_description,omitempty"`
	ShotSize          string    `json:"shot_size,omitempty"`
	CameraMovement    string    `json:"camera_movement,omitempty"`
	Subjects          []string  `json:"subjects,omitempty"`
	SceneTags         []string  `json:"scene_tags,omitempty"`
	QualityTags       []string  `json:"quality_tags,omitempty"`
	ReviewerNotes     string    `json:"reviewer_notes,omitempty"`
	AnalysisError     string    `json:"analysis_error,omitempty"`
	CreatedByUserID   string    `json:"created_by_user_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	AnalyzedAt        *time.Time `json:"analyzed_at,omitempty"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
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
	mu            sync.RWMutex
	products      map[string]Product
	sellingPoints map[string]SellingPoint
	assets        map[string]Asset
	queries       *db.Queries
	assetRepo     *repository.AssetRepository
}

func NewProductAssetService() *ProductAssetService {
	return &ProductAssetService{
		products:      map[string]Product{},
		sellingPoints: map[string]SellingPoint{},
		assets:        map[string]Asset{},
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
	ProductID         string
	AssetName         string
	StorageKey        string
	FileName          string
	FileExt           string
	MimeType          string
	FileSize          int64
	Checksum          string
	SourceType        string
	IngestionSource   string
	DurationMs        int
	Width             int
	Height            int
	FPS               float64
	Codec             string
	Status            string
	AnalysisStatus    string
	UsabilityStatus   string
	ManualCleanStatus string
	SourcePath        string
	SourceOriginalName string
	SourceInMs        int
	SourceOutMs       int
	HasAudio          bool
	AudioCodec        string
	BitrateKbps       int
	SceneDescription  string
	ShotSize          string
	CameraMovement    string
	Subjects          []string
	SceneTags         []string
	QualityTags       []string
	ReviewerNotes     string
	AnalysisError     string
	SellingPointIDs   []string
	CreatedByUserID   string
}

type AssetFilters struct {
	ProductID       string
	SourceType      string
	Status          string
	AnalysisStatus  string
	UsabilityStatus string
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
		ID:                uuid.NewString(),
		ProductID:         input.ProductID,
		AssetName:         assetName,
		StorageKey:        input.StorageKey,
		FileName:          input.FileName,
		FileExt:           input.FileExt,
		MimeType:          input.MimeType,
		FileSize:          input.FileSize,
		Checksum:          input.Checksum,
		SourceType:        input.SourceType,
		IngestionSource:   ingestionSource,
		DurationMs:        input.DurationMs,
		Width:             input.Width,
		Height:            input.Height,
		FPS:               input.FPS,
		Codec:             input.Codec,
		Status:            status,
		AnalysisStatus:    analysisStatus,
		UsabilityStatus:   usabilityStatus,
		ManualCleanStatus: manualCleanStatus,
		SourcePath:        input.SourcePath,
		SourceOriginalName: input.SourceOriginalName,
		SourceInMs:        input.SourceInMs,
		SourceOutMs:       input.SourceOutMs,
		HasAudio:          input.HasAudio,
		AudioCodec:        input.AudioCodec,
		BitrateKbps:       input.BitrateKbps,
		SceneDescription:  input.SceneDescription,
		ShotSize:          input.ShotSize,
		CameraMovement:    input.CameraMovement,
		Subjects:          append([]string(nil), input.Subjects...),
		SceneTags:         append([]string(nil), input.SceneTags...),
		QualityTags:       append([]string(nil), input.QualityTags...),
		ReviewerNotes:     input.ReviewerNotes,
		AnalysisError:     input.AnalysisError,
		CreatedByUserID:   input.CreatedByUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	s.assets[asset.ID] = asset
	return asset, nil
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
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].CreatedAt.After(assets[j].CreatedAt)
	})
	return assets
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
	return items
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
	return items
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
		SceneDescription:   assetTextParam(input.SceneDescription),
		ShotSize:           assetTextParam(input.ShotSize),
		CameraMovement:     assetTextParam(input.CameraMovement),
		Subjects:           subjectsJSON,
		SceneTags:          sceneTagsJSON,
		QualityTags:        qualityTagsJSON,
		ModelResult:        []byte(`{}`),
		ReviewerNotes:      assetTextParam(input.ReviewerNotes),
		AnalysisError:      assetTextParam(input.AnalysisError),
		AnalyzedAt:         pgtype.Timestamptz{},
		Metadata:           []byte(`{}`),
		CreatedByUserID:    assetNullableUUIDParam(input.CreatedByUserID),
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
	})
	if err != nil {
		return nil
	}
	items := make([]Asset, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetFromDBRecord(row))
	}
	return items
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

func assetFromDBRecord(row repository.AssetRecord) Asset {
	return Asset{
		ID:                row.ID,
		ProductID:         row.ProductID,
		AssetName:         row.AssetName,
		StorageKey:        row.StorageKey,
		FileName:          row.FileName,
		FileExt:           row.FileExt,
		MimeType:          row.MimeType,
		FileSize:          row.FileSize,
		Checksum:          row.Checksum,
		SourceType:        row.SourceType,
		IngestionSource:   row.IngestionSource,
		DurationMs:        row.DurationMs,
		Width:             row.Width,
		Height:            row.Height,
		FPS:               row.FPS,
		Codec:             row.Codec,
		Status:            row.Status,
		AnalysisStatus:    row.AnalysisStatus,
		UsabilityStatus:   row.UsabilityStatus,
		ManualCleanStatus: row.ManualCleanStatus,
		SourcePath:        row.SourcePath,
		SourceOriginalName: row.SourceOriginalName,
		SourceInMs:        row.SourceInMs,
		SourceOutMs:       row.SourceOutMs,
		HasAudio:          row.HasAudio,
		AudioCodec:        row.AudioCodec,
		BitrateKbps:       row.BitrateKbps,
		SceneDescription:  row.SceneDescription,
		ShotSize:          row.ShotSize,
		CameraMovement:    row.CameraMovement,
		Subjects:          decodeStringList(row.Subjects),
		SceneTags:         decodeStringList(row.SceneTags),
		QualityTags:       decodeStringList(row.QualityTags),
		ReviewerNotes:     row.ReviewerNotes,
		AnalysisError:     row.AnalysisError,
		CreatedByUserID:   row.CreatedByUserID,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		AnalyzedAt:        row.AnalyzedAt,
		ArchivedAt:        row.ArchivedAt,
	}
}

func assetFromDBRow(row db.Asset) Asset {
	return Asset{
		ID:                assetUUIDString(row.ID),
		ProductID:         assetUUIDString(row.ProductID),
		AssetName:         assetTextString(row.AssetName),
		StorageKey:        row.StorageKey,
		FileName:          row.FileName,
		FileExt:           assetTextString(row.FileExt),
		MimeType:          assetTextString(row.MimeType),
		FileSize:          row.FileSize,
		Checksum:          assetTextString(row.Checksum),
		SourceType:        row.SourceType,
		IngestionSource:   row.IngestionSource,
		DurationMs:        int4Value(row.DurationMs),
		Width:             int4Value(row.Width),
		Height:            int4Value(row.Height),
		FPS:               numericValue(row.Fps),
		Codec:             assetTextString(row.Codec),
		Status:            row.Status,
		AnalysisStatus:    row.AnalysisStatus,
		UsabilityStatus:   row.UsabilityStatus,
		ManualCleanStatus: row.ManualCleanStatus,
		SourcePath:        assetTextString(row.SourcePath),
		SourceOriginalName: assetTextString(row.SourceOriginalName),
		SourceInMs:        int4Value(row.SourceInMs),
		SourceOutMs:       int4Value(row.SourceOutMs),
		HasAudio:          row.HasAudio,
		AudioCodec:        assetTextString(row.AudioCodec),
		BitrateKbps:       int4Value(row.BitrateKbps),
		SceneDescription:  assetTextString(row.SceneDescription),
		ShotSize:          assetTextString(row.ShotSize),
		CameraMovement:    assetTextString(row.CameraMovement),
		Subjects:          decodeStringList(row.Subjects),
		SceneTags:         decodeStringList(row.SceneTags),
		QualityTags:       decodeStringList(row.QualityTags),
		ReviewerNotes:     assetTextString(row.ReviewerNotes),
		AnalysisError:     assetTextString(row.AnalysisError),
		CreatedByUserID:   assetUUIDString(row.CreatedByUserID),
		CreatedAt:         timeValue(row.CreatedAt),
		UpdatedAt:         timeValue(row.UpdatedAt),
		AnalyzedAt:        optionalTime(row.AnalyzedAt),
		ArchivedAt:        optionalTime(row.ArchivedAt),
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
