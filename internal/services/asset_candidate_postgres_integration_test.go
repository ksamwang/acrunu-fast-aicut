package services

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCandidateBatchUsageQuery(t *testing.T) {
	databaseURL := os.Getenv("AICUT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICUT_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	const missingID = "00000000-0000-0000-0000-000000000000"
	counts, err := (postgresCandidateSearchStore{pool: pool}).LoadBatchAssetUseCounts(context.Background(), missingID, missingID)
	if err != nil {
		t.Fatalf("load batch asset use counts: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("expected no usage for a missing batch, got %#v", counts)
	}

	var existingBatchID string
	err = pool.QueryRow(context.Background(), `
		SELECT selections.generation_batch_id::text
		FROM generation_asset_selections AS selections
		WHERE selections.state = 'committed'
		ORDER BY selections.created_at DESC
		LIMIT 1`).Scan(&existingBatchID)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Skip("test database has no committed asset selections")
	}
	if err != nil {
		t.Fatalf("load existing generation batch: %v", err)
	}
	counts, err = (postgresCandidateSearchStore{pool: pool}).LoadBatchAssetUseCounts(context.Background(), existingBatchID, missingID)
	if err != nil {
		t.Fatalf("load existing batch asset use counts: %v", err)
	}
	if len(counts) == 0 {
		t.Fatal("expected committed asset usage for the selected batch")
	}
}
