package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type AssetRecord struct {
	ID                 string          `json:"id"`
	ProductID          string          `json:"product_id"`
	AssetName          string          `json:"asset_name,omitempty"`
	StorageKey         string          `json:"storage_key"`
	FileName           string          `json:"file_name"`
	FileExt            string          `json:"file_ext,omitempty"`
	MimeType           string          `json:"mime_type,omitempty"`
	FileSize           int64           `json:"file_size"`
	Checksum           string          `json:"checksum,omitempty"`
	SourceType         string          `json:"source_type"`
	IngestionSource    string          `json:"ingestion_source"`
	DurationMs         int             `json:"duration_ms,omitempty"`
	Width              int             `json:"width,omitempty"`
	Height             int             `json:"height,omitempty"`
	FPS                float64         `json:"fps,omitempty"`
	Codec              string          `json:"codec,omitempty"`
	Status             string          `json:"status"`
	AnalysisStatus     string          `json:"analysis_status"`
	UsabilityStatus    string          `json:"usability_status"`
	ManualCleanStatus  string          `json:"manual_clean_status"`
	SourcePath         string          `json:"source_path,omitempty"`
	SourceOriginalName string          `json:"source_original_name,omitempty"`
	SourceInMs         int             `json:"source_in_ms,omitempty"`
	SourceOutMs        int             `json:"source_out_ms,omitempty"`
	HasAudio           bool            `json:"has_audio"`
	AudioCodec         string          `json:"audio_codec,omitempty"`
	BitrateKbps        int             `json:"bitrate_kbps,omitempty"`
	SceneDescription   string          `json:"scene_description,omitempty"`
	ShotSize           string          `json:"shot_size,omitempty"`
	CameraMovement     string          `json:"camera_movement,omitempty"`
	Subjects           json.RawMessage `json:"subjects,omitempty"`
	SceneTags          json.RawMessage `json:"scene_tags,omitempty"`
	QualityTags        json.RawMessage `json:"quality_tags,omitempty"`
	ModelResult        json.RawMessage `json:"model_result,omitempty"`
	ReviewerNotes      string          `json:"reviewer_notes,omitempty"`
	AnalysisError      string          `json:"analysis_error,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	CreatedByUserID    string          `json:"created_by_user_id,omitempty"`
	UpdatedByUserID    string          `json:"updated_by_user_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	AnalyzedAt         *time.Time      `json:"analyzed_at,omitempty"`
	ArchivedAt         *time.Time      `json:"archived_at,omitempty"`
}

type SpeechSegmentRecord struct {
	ID              string     `json:"id"`
	AssetID         string     `json:"asset_id"`
	StartMs         int        `json:"start_ms"`
	EndMs           int        `json:"end_ms"`
	Transcript      string     `json:"transcript"`
	Confidence      float64    `json:"confidence,omitempty"`
	Source          string     `json:"source"`
	Status          string     `json:"status"`
	CreatedByUserID string     `json:"created_by_user_id,omitempty"`
	UpdatedByUserID string     `json:"updated_by_user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type AssetFilters struct {
	ProductID       string
	SourceType      string
	Status          string
	AnalysisStatus  string
	UsabilityStatus string
}

type SpeechSegmentFilters struct {
	Status string
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

type UpdateSpeechSegmentInput struct {
	ID              string
	StartMs         int
	EndMs           int
	Transcript      string
	Confidence      *float64
	Source          string
	Status          string
	UpdatedByUserID string
}

type AssetRepository struct {
	queries db.Querier
}

func NewAssetRepository(queries db.Querier) *AssetRepository {
	return &AssetRepository{queries: queries}
}

func (r *AssetRepository) GetByID(ctx context.Context, assetID string) (AssetRecord, error) {
	row, err := r.queries.GetAssetByID(ctx, uuidParam(assetID))
	if err != nil {
		return AssetRecord{}, err
	}
	return assetFromDB(row), nil
}

func (r *AssetRepository) List(ctx context.Context, filters AssetFilters) ([]AssetRecord, error) {
	rows, err := r.queries.ListAssets(ctx, db.ListAssetsParams{
		ProductID:       nullableUUIDParam(filters.ProductID),
		SourceType:      textParam(filters.SourceType),
		Status:          textParam(filters.Status),
		AnalysisStatus:  textParam(filters.AnalysisStatus),
		UsabilityStatus: textParam(filters.UsabilityStatus),
	})
	if err != nil {
		return nil, err
	}
	items := make([]AssetRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetFromDB(row))
	}
	return items, nil
}

func (r *AssetRepository) AddSellingPoint(ctx context.Context, assetID string, sellingPointID string) error {
	return r.queries.AddAssetSellingPoint(ctx, db.AddAssetSellingPointParams{
		AssetID:        uuidParam(assetID),
		SellingPointID: uuidParam(sellingPointID),
	})
}

func (r *AssetRepository) RemoveSellingPoint(ctx context.Context, assetID string, sellingPointID string) error {
	return r.queries.RemoveAssetSellingPoint(ctx, db.RemoveAssetSellingPointParams{
		AssetID:        uuidParam(assetID),
		SellingPointID: uuidParam(sellingPointID),
	})
}

func (r *AssetRepository) ListSellingPointIDs(ctx context.Context, assetID string) ([]string, error) {
	rows, err := r.queries.ListSellingPointIDsByAsset(ctx, uuidParam(assetID))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, uuidString(row))
	}
	return ids, nil
}

func (r *AssetRepository) CreateSpeechSegment(ctx context.Context, input CreateSpeechSegmentInput) (SpeechSegmentRecord, error) {
	row, err := r.queries.CreateSpeechSegment(ctx, db.CreateSpeechSegmentParams{
		AssetID:         uuidParam(input.AssetID),
		StartMs:         int32(input.StartMs),
		EndMs:           int32(input.EndMs),
		Transcript:      input.Transcript,
		Confidence:      nullableNumeric(input.Confidence),
		Source:          input.Source,
		Status:          input.Status,
		CreatedByUserID: nullableUUIDParam(input.CreatedByUserID),
	})
	if err != nil {
		return SpeechSegmentRecord{}, err
	}
	return speechSegmentFromDB(row), nil
}

func (r *AssetRepository) UpdateSpeechSegment(ctx context.Context, input UpdateSpeechSegmentInput) (SpeechSegmentRecord, error) {
	row, err := r.queries.UpdateSpeechSegment(ctx, db.UpdateSpeechSegmentParams{
		ID:              uuidParam(input.ID),
		StartMs:         int32(input.StartMs),
		EndMs:           int32(input.EndMs),
		Transcript:      input.Transcript,
		Confidence:      nullableNumeric(input.Confidence),
		Source:          input.Source,
		Status:          input.Status,
		UpdatedByUserID: nullableUUIDParam(input.UpdatedByUserID),
	})
	if err != nil {
		return SpeechSegmentRecord{}, err
	}
	return speechSegmentFromDB(row), nil
}

func (r *AssetRepository) ListSpeechSegmentsByAsset(ctx context.Context, assetID string, filters SpeechSegmentFilters) ([]SpeechSegmentRecord, error) {
	rows, err := r.queries.ListSpeechSegmentsByAsset(ctx, db.ListSpeechSegmentsByAssetParams{
		AssetID: uuidParam(assetID),
		Status:  textParam(filters.Status),
	})
	if err != nil {
		return nil, err
	}
	items := make([]SpeechSegmentRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, speechSegmentFromDB(row))
	}
	return items, nil
}

func assetFromDB(row db.Asset) AssetRecord {
	return AssetRecord{
		ID:                 uuidString(row.ID),
		ProductID:          uuidString(row.ProductID),
		AssetName:          textString(row.AssetName),
		StorageKey:         row.StorageKey,
		FileName:           row.FileName,
		FileExt:            textString(row.FileExt),
		MimeType:           textString(row.MimeType),
		FileSize:           row.FileSize,
		Checksum:           textString(row.Checksum),
		SourceType:         row.SourceType,
		IngestionSource:    row.IngestionSource,
		DurationMs:         int32Value(row.DurationMs),
		Width:              int32Value(row.Width),
		Height:             int32Value(row.Height),
		FPS:                numericValue(row.Fps),
		Codec:              textString(row.Codec),
		Status:             row.Status,
		AnalysisStatus:     row.AnalysisStatus,
		UsabilityStatus:    row.UsabilityStatus,
		ManualCleanStatus:  row.ManualCleanStatus,
		SourcePath:         textString(row.SourcePath),
		SourceOriginalName: textString(row.SourceOriginalName),
		SourceInMs:         int32Value(row.SourceInMs),
		SourceOutMs:        int32Value(row.SourceOutMs),
		HasAudio:           row.HasAudio,
		AudioCodec:         textString(row.AudioCodec),
		BitrateKbps:        int32Value(row.BitrateKbps),
		SceneDescription:   textString(row.SceneDescription),
		ShotSize:           textString(row.ShotSize),
		CameraMovement:     textString(row.CameraMovement),
		Subjects:           cloneJSON(row.Subjects),
		SceneTags:          cloneJSON(row.SceneTags),
		QualityTags:        cloneJSON(row.QualityTags),
		ModelResult:        cloneJSON(row.ModelResult),
		ReviewerNotes:      textString(row.ReviewerNotes),
		AnalysisError:      textString(row.AnalysisError),
		Metadata:           cloneJSON(row.Metadata),
		CreatedByUserID:    uuidString(row.CreatedByUserID),
		UpdatedByUserID:    uuidString(row.UpdatedByUserID),
		CreatedAt:          timeValue(row.CreatedAt),
		UpdatedAt:          timeValue(row.UpdatedAt),
		AnalyzedAt:         optionalTime(row.AnalyzedAt),
		ArchivedAt:         optionalTime(row.ArchivedAt),
	}
}

func speechSegmentFromDB(row db.SpeechSegment) SpeechSegmentRecord {
	return SpeechSegmentRecord{
		ID:              uuidString(row.ID),
		AssetID:         uuidString(row.AssetID),
		StartMs:         int(row.StartMs),
		EndMs:           int(row.EndMs),
		Transcript:      row.Transcript,
		Confidence:      numericValue(row.Confidence),
		Source:          row.Source,
		Status:          row.Status,
		CreatedByUserID: uuidString(row.CreatedByUserID),
		UpdatedByUserID: uuidString(row.UpdatedByUserID),
		CreatedAt:       timeValue(row.CreatedAt),
		UpdatedAt:       timeValue(row.UpdatedAt),
	}
}

func uuidParam(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUIDParam(value string) pgtype.UUID {
	return uuidParam(value)
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func textParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func int32Value(value pgtype.Int4) int {
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

func nullableNumeric(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	var numeric pgtype.Numeric
	_ = numeric.Scan(*value)
	return numeric
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timeValue := value.Time
	return &timeValue
}

func cloneJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
