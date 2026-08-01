package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

type AssetEmbeddingObject struct {
	ID           string         `json:"id"`
	AssetID      string         `json:"asset_id"`
	ObjectType   string         `json:"object_type"`
	ObjectID     string         `json:"object_id"`
	Text         string         `json:"text"`
	TextHash     string         `json:"text_hash"`
	ProviderID   string         `json:"provider_id"`
	Model        string         `json:"model"`
	Dimension    int            `json:"dimension"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Status       string         `json:"status"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type AssetEmbeddingRunResult struct {
	AssetID    string                 `json:"asset_id"`
	ProviderID string                 `json:"provider_id"`
	Model      string                 `json:"model"`
	Dimension  int                    `json:"dimension"`
	Objects    []AssetEmbeddingObject `json:"objects"`
}

type AssetEmbeddingService struct {
	pool                 *pgxpool.Pool
	productAssetService  *ProductAssetService
	systemConfigService  *SystemConfigService
	modelProviderService *ModelProviderService
	fallbackConfig       config.Config
	semanticProjectionMu sync.Mutex
}

func NewAssetEmbeddingService(pool *pgxpool.Pool, productAssetService *ProductAssetService, systemConfigService *SystemConfigService, modelProviderService *ModelProviderService, fallbackConfig config.Config) *AssetEmbeddingService {
	return &AssetEmbeddingService{
		pool:                 pool,
		productAssetService:  productAssetService,
		systemConfigService:  systemConfigService,
		modelProviderService: modelProviderService,
		fallbackConfig:       fallbackConfig,
	}
}

func (s *AssetEmbeddingService) VectorizeAsset(ctx context.Context, assetID string) (AssetEmbeddingRunResult, error) {
	if s == nil || s.pool == nil {
		return AssetEmbeddingRunResult{}, fmt.Errorf("asset embedding store is not configured")
	}
	if s.productAssetService == nil {
		return AssetEmbeddingRunResult{}, fmt.Errorf("product asset service is nil")
	}

	preview, err := s.productAssetService.BuildAssetSemanticPreview(assetID)
	if err != nil {
		return AssetEmbeddingRunResult{}, err
	}
	cfg := ResolveEmbeddingConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	if strings.TrimSpace(cfg.Model) == "" {
		return AssetEmbeddingRunResult{}, fmt.Errorf("embedding model is required")
	}
	providerID, err := s.resolveEmbeddingProviderID()
	if err != nil {
		return AssetEmbeddingRunResult{}, err
	}

	embedder := modelgateway.NewTextEmbedder(cfg)
	objects := make([]AssetEmbeddingObject, 0, len(preview.EmbeddingTargets))
	for _, target := range preview.EmbeddingTargets {
		result, err := embedder.EmbedText(ctx, modelgateway.EmbedTextInput{Text: target.Text})
		if err != nil {
			return AssetEmbeddingRunResult{}, err
		}
		if len(result.Embedding) == 0 {
			return AssetEmbeddingRunResult{}, fmt.Errorf("embedding result is empty")
		}
		dimension := cfg.Dimensions
		if dimension <= 0 {
			dimension = len(result.Embedding)
		}
		if len(result.Embedding) != dimension {
			return AssetEmbeddingRunResult{}, fmt.Errorf("embedding dimension mismatch: expected %d, got %d", dimension, len(result.Embedding))
		}

		stored, err := s.upsertObject(ctx, target, providerID, cfg.Model, dimension, result.Embedding)
		if err != nil {
			return AssetEmbeddingRunResult{}, err
		}
		objects = append(objects, stored)
	}

	return AssetEmbeddingRunResult{
		AssetID:    preview.AssetID,
		ProviderID: providerID,
		Model:      cfg.Model,
		Dimension:  cfg.Dimensions,
		Objects:    objects,
	}, nil
}

func (s *AssetEmbeddingService) ListAssetEmbeddingObjects(ctx context.Context, assetID string) ([]AssetEmbeddingObject, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("asset embedding store is not configured")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, asset_id::text, object_type, object_id::text, text, text_hash,
		       provider_id::text, model, dimension, metadata, status, error_message, created_at, updated_at
		FROM asset_embedding_objects
		WHERE asset_id = $1
		ORDER BY object_type ASC, created_at ASC`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AssetEmbeddingObject{}
	for rows.Next() {
		item, err := scanAssetEmbeddingObject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AssetEmbeddingService) resolveEmbeddingProviderID() (string, error) {
	if s.systemConfigService == nil {
		return "", fmt.Errorf("system config service is nil")
	}
	config, err := s.systemConfigService.Get("embedding.provider_id")
	if err != nil {
		return "", fmt.Errorf("embedding.provider_id is required")
	}
	providerID := configStringValue(config.Value)
	if strings.TrimSpace(providerID) == "" {
		return "", fmt.Errorf("embedding.provider_id is required")
	}
	return providerID, nil
}

func (s *AssetEmbeddingService) upsertObject(ctx context.Context, target EmbeddingTarget, providerID string, model string, dimension int, embedding []float64) (AssetEmbeddingObject, error) {
	metadata, err := json.Marshal(target.Metadata)
	if err != nil {
		return AssetEmbeddingObject{}, err
	}
	vectorLiteral := vectorString(embedding)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO asset_embedding_objects (
		    asset_id, object_type, object_id, text, text_hash,
		    provider_id, model, dimension, embedding, metadata, status, error_message
		) VALUES (
		    $1, $2, $3, $4, $5,
		    $6, $7, $8, $9::vector, $10, 'ready', ''
		)
		ON CONFLICT (asset_id, object_type, object_id, provider_id, model, dimension) DO UPDATE SET
		    text = EXCLUDED.text,
		    text_hash = EXCLUDED.text_hash,
		    embedding = EXCLUDED.embedding,
		    metadata = EXCLUDED.metadata,
		    status = 'ready',
		    error_message = '',
		    updated_at = now()
		RETURNING id::text, asset_id::text, object_type, object_id::text, text, text_hash,
		          provider_id::text, model, dimension, metadata, status, error_message, created_at, updated_at`,
		target.AssetID,
		target.ObjectType,
		target.ObjectID,
		target.Text,
		textHash(target.Text),
		providerID,
		model,
		dimension,
		vectorLiteral,
		metadata,
	)
	return scanAssetEmbeddingObject(row)
}

type assetEmbeddingRowScanner interface {
	Scan(dest ...any) error
}

func scanAssetEmbeddingObject(row assetEmbeddingRowScanner) (AssetEmbeddingObject, error) {
	var item AssetEmbeddingObject
	var metadata []byte
	if err := row.Scan(
		&item.ID,
		&item.AssetID,
		&item.ObjectType,
		&item.ObjectID,
		&item.Text,
		&item.TextHash,
		&item.ProviderID,
		&item.Model,
		&item.Dimension,
		&metadata,
		&item.Status,
		&item.ErrorMessage,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return AssetEmbeddingObject{}, err
	}
	item.Metadata = map[string]any{}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &item.Metadata)
	}
	return item, nil
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func vectorString(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(value, 'f', -1, 64))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

var _ assetEmbeddingRowScanner = pgx.Row(nil)
