package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

func TestAssetFromDB(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	analyzedAt := now.Add(2 * time.Minute)
	archivedAt := now.Add(5 * time.Minute)
	assetID := mustUUID(t)
	productID := mustUUID(t)
	userID := mustUUID(t)

	record := assetFromDB(db.Asset{
		ID:                 pgUUID(assetID),
		ProductID:          pgUUID(productID),
		AssetName:          pgText("Demo Shot"),
		StorageKey:         "assets/demo.mp4",
		FileName:           "demo.mp4",
		FileExt:            pgText(".mp4"),
		MimeType:           pgText("video/mp4"),
		FileSize:           2048,
		Checksum:           pgText("abc123"),
		SourceType:         "talking_head",
		IngestionSource:    "manual-import",
		DurationMs:         pgInt4(1500),
		Width:              pgInt4(1080),
		Height:             pgInt4(1920),
		Fps:                pgNumeric(29.97),
		Codec:              pgText("h264"),
		Status:             "ready",
		AnalysisStatus:     "analyzing",
		UsabilityStatus:    "needs_review",
		ManualCleanStatus:  "cleaned",
		SourcePath:         pgText("D:/shots/demo.mp4"),
		SourceOriginalName: pgText("camera_take.mp4"),
		SourceInMs:         pgInt4(100),
		SourceOutMs:        pgInt4(1600),
		HasAudio:           true,
		AudioCodec:         pgText("aac"),
		BitrateKbps:        pgInt4(3200),
		SceneDescription:   pgText("driver installs product"),
		ShotSize:           pgText("medium_close_up"),
		CameraMovement:     pgText("push_in"),
		Subjects:           []byte(`["driver","product"]`),
		SceneTags:          []byte(`["car","interior"]`),
		QualityTags:        []byte(`["slight_shake"]`),
		ModelResult:        []byte(`{"score":0.92}`),
		ReviewerNotes:      pgText("usable after crop"),
		AnalysisError:      pgText(""),
		Metadata:           []byte(`{"source":"demo"}`),
		CreatedByUserID:    pgUUID(userID),
		UpdatedByUserID:    pgUUID(userID),
		CreatedAt:          pgTime(now),
		UpdatedAt:          pgTime(now),
		AnalyzedAt:         pgTime(analyzedAt),
		ArchivedAt:         pgTime(archivedAt),
	})

	if record.ID != assetID.String() {
		t.Fatalf("expected asset id %s, got %s", assetID, record.ID)
	}
	if record.ProductID != productID.String() {
		t.Fatalf("expected product id %s, got %s", productID, record.ProductID)
	}
	if record.AssetName != "Demo Shot" {
		t.Fatalf("expected asset name to map, got %q", record.AssetName)
	}
	if record.FPS != 29.97 {
		t.Fatalf("expected fps to map, got %v", record.FPS)
	}
	if string(record.Subjects) != `["driver","product"]` {
		t.Fatalf("expected subjects json to map, got %s", string(record.Subjects))
	}
	if record.AnalyzedAt == nil || !record.AnalyzedAt.Equal(analyzedAt) {
		t.Fatalf("expected analyzed_at to map, got %+v", record.AnalyzedAt)
	}
	if record.ArchivedAt == nil || !record.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("expected archived_at to map, got %+v", record.ArchivedAt)
	}
}

func TestSpeechSegmentFromDB(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	segmentID := mustUUID(t)
	assetID := mustUUID(t)
	userID := mustUUID(t)

	record := speechSegmentFromDB(db.SpeechSegment{
		ID:              pgUUID(segmentID),
		AssetID:         pgUUID(assetID),
		StartMs:         320,
		EndMs:           1240,
		Transcript:      "大家好",
		Confidence:      pgNumeric(0.9876),
		Source:          "manual",
		Status:          "active",
		CreatedByUserID: pgUUID(userID),
		UpdatedByUserID: pgUUID(userID),
		CreatedAt:       pgTime(now),
		UpdatedAt:       pgTime(now),
	})

	if record.ID != segmentID.String() {
		t.Fatalf("expected speech segment id %s, got %s", segmentID, record.ID)
	}
	if record.AssetID != assetID.String() {
		t.Fatalf("expected asset id %s, got %s", assetID, record.AssetID)
	}
	if record.Confidence != 0.9876 {
		t.Fatalf("expected confidence to map, got %v", record.Confidence)
	}
	if record.Transcript != "大家好" {
		t.Fatalf("expected transcript to map, got %q", record.Transcript)
	}
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("failed to generate uuid: %v", err)
	}
	return id
}

func pgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}

func pgText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func pgInt4(value int32) pgtype.Int4 {
	return pgtype.Int4{Int32: value, Valid: true}
}

func pgTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func pgNumeric(value float64) pgtype.Numeric {
	var numeric pgtype.Numeric
	_ = numeric.Scan(fmt.Sprintf("%.4f", value))
	return numeric
}
