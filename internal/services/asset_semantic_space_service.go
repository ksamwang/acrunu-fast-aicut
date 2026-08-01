package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"gonum.org/v1/gonum/mat"
)

const (
	semanticProjectionAlgorithm = "jl32-pca3-v1"
	semanticProjectionSourceCap = 5000
	semanticSpacePointLimit     = 5000
)

type AssetSemanticSpacePoint struct {
	AssetID           string  `json:"asset_id"`
	X2                float64 `json:"x2"`
	Y2                float64 `json:"y2"`
	X3                float64 `json:"x3"`
	Y3                float64 `json:"y3"`
	Z3                float64 `json:"z3"`
	ProductID         string  `json:"product_id"`
	AssetName         string  `json:"asset_name,omitempty"`
	StorageKey        string  `json:"storage_key"`
	FileName          string  `json:"file_name"`
	SourceType        string  `json:"source_type"`
	Status            string  `json:"status"`
	UsabilityStatus   string  `json:"usability_status,omitempty"`
	DurationMs        int     `json:"duration_ms,omitempty"`
	ShotSize          string  `json:"shot_size,omitempty"`
	SceneDescription  string  `json:"scene_description,omitempty"`
	ActionDescription string  `json:"action_description,omitempty"`
	ThumbnailURL      string  `json:"thumbnail_url,omitempty"`
}

type AssetSemanticSpaceResult struct {
	ProjectionID          string                    `json:"projection_id,omitempty"`
	ProviderID            string                    `json:"provider_id,omitempty"`
	Model                 string                    `json:"model,omitempty"`
	Dimension             int                       `json:"dimension,omitempty"`
	Algorithm             string                    `json:"algorithm,omitempty"`
	Total                 int                       `json:"total"`
	Returned              int                       `json:"returned"`
	Sampled               bool                      `json:"sampled"`
	MissingEmbeddingCount int                       `json:"missing_embedding_count"`
	UpdatedAt             time.Time                 `json:"updated_at,omitempty"`
	Points                []AssetSemanticSpacePoint `json:"points"`
}

type AssetSemanticNeighbor struct {
	AssetID string  `json:"asset_id"`
	Score   float64 `json:"score"`
}

type AssetSemanticNeighborResult struct {
	AssetID string                  `json:"asset_id,omitempty"`
	Query   string                  `json:"query,omitempty"`
	Items   []AssetSemanticNeighbor `json:"items"`
}

type semanticProjectionRun struct {
	ID              string
	ProviderID      string
	Model           string
	Dimension       int
	Algorithm       string
	SourceCount     int
	ProjectedCount  int
	SourceUpdatedAt *time.Time
	UpdatedAt       time.Time
}

type semanticProjectionSource struct {
	AssetID   string
	Embedding []float32
}

type semanticProjectionCoordinate struct {
	AssetID string
	X2      float64
	Y2      float64
	X3      float64
	Y3      float64
	Z3      float64
}

func (s *AssetEmbeddingService) GetAssetSemanticSpace(ctx context.Context, filters AssetFilters) (AssetSemanticSpaceResult, error) {
	if s == nil || s.pool == nil {
		return AssetSemanticSpaceResult{}, fmt.Errorf("asset semantic space is not configured")
	}
	providerID, model, dimension, err := s.resolveSemanticEmbeddingIdentity(ctx)
	if err != nil {
		return AssetSemanticSpaceResult{}, err
	}
	run, err := s.ensureSemanticProjection(ctx, providerID, model, dimension)
	if err != nil {
		return AssetSemanticSpaceResult{}, err
	}
	if run.ID == "" {
		return AssetSemanticSpaceResult{ProviderID: providerID, Model: model, Dimension: dimension, Algorithm: semanticProjectionAlgorithm, Points: []AssetSemanticSpacePoint{}}, nil
	}

	points, total, missing, err := s.listSemanticProjectionPoints(ctx, run, filters)
	if err != nil {
		return AssetSemanticSpaceResult{}, err
	}
	return AssetSemanticSpaceResult{
		ProjectionID:          run.ID,
		ProviderID:            run.ProviderID,
		Model:                 run.Model,
		Dimension:             run.Dimension,
		Algorithm:             run.Algorithm,
		Total:                 total,
		Returned:              len(points),
		Sampled:               total > len(points) || run.SourceCount > run.ProjectedCount,
		MissingEmbeddingCount: missing,
		UpdatedAt:             run.UpdatedAt,
		Points:                points,
	}, nil
}

func (s *AssetEmbeddingService) QueryAssetSemanticSpace(ctx context.Context, query string, filters AssetFilters, limit int) (AssetSemanticNeighborResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	result, err := s.SearchAssets(ctx, AssetSemanticSearchInput{Query: query, Filters: filters, Limit: limit})
	if err != nil {
		return AssetSemanticNeighborResult{}, err
	}
	items := make([]AssetSemanticNeighbor, 0, len(result.Items))
	for _, asset := range result.Items {
		if asset.SemanticScore == nil {
			continue
		}
		items = append(items, AssetSemanticNeighbor{AssetID: asset.ID, Score: *asset.SemanticScore})
	}
	return AssetSemanticNeighborResult{Query: strings.TrimSpace(query), Items: items}, nil
}

func (s *AssetEmbeddingService) FindAssetSemanticNeighbors(ctx context.Context, assetID string, filters AssetFilters, limit int) (AssetSemanticNeighborResult, error) {
	if s == nil || s.pool == nil {
		return AssetSemanticNeighborResult{}, fmt.Errorf("asset semantic search is not configured")
	}
	if _, err := uuid.Parse(assetID); err != nil {
		return AssetSemanticNeighborResult{}, fmt.Errorf("asset id is invalid")
	}
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	providerID, model, dimension, err := s.resolveSemanticEmbeddingIdentity(ctx)
	if err != nil {
		return AssetSemanticNeighborResult{}, err
	}

	args := []any{assetID, providerID, model, dimension}
	assetConditions, args := buildSemanticAssetFilterConditions("a", filters, args)
	conditions := append([]string{
		"e.status = 'ready'",
		"e.object_type = 'shot'",
		"e.provider_id = $2::uuid",
		"e.model = $3",
		"e.dimension = $4",
		"e.asset_id <> $1::uuid",
	}, assetConditions...)
	limitPlaceholder := semanticSearchBind(&args, limit)
	distanceExpression := vectorCosineDistanceSQL("e.embedding", "target.embedding", model, dimension)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		WITH target AS (
			SELECT embedding
			FROM asset_embedding_objects
			WHERE asset_id = $1::uuid
			  AND object_type = 'shot'
			  AND status = 'ready'
			  AND provider_id = $2::uuid
			  AND model = $3
			  AND dimension = $4
			LIMIT 1
		)
		SELECT e.asset_id::text, 1 - (%s) AS semantic_score
		FROM asset_embedding_objects e
		JOIN assets a ON a.id = e.asset_id
		CROSS JOIN target
		WHERE %s
		ORDER BY %s ASC, e.id ASC
		LIMIT %s`, distanceExpression, strings.Join(conditions, "\n AND "), distanceExpression, limitPlaceholder), args...)
	if err != nil {
		return AssetSemanticNeighborResult{}, err
	}
	defer rows.Close()
	items := make([]AssetSemanticNeighbor, 0, limit)
	for rows.Next() {
		var item AssetSemanticNeighbor
		if err := rows.Scan(&item.AssetID, &item.Score); err != nil {
			return AssetSemanticNeighborResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AssetSemanticNeighborResult{}, err
	}
	return AssetSemanticNeighborResult{AssetID: assetID, Items: items}, nil
}

func (s *AssetEmbeddingService) resolveSemanticEmbeddingIdentity(ctx context.Context) (string, string, int, error) {
	cfg := ResolveEmbeddingConfigWithProviders(ctx, s.systemConfigService, s.modelProviderService, s.fallbackConfig)
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return "", "", 0, fmt.Errorf("embedding model is required")
	}
	providerID, err := s.resolveEmbeddingProviderID()
	if err != nil {
		return "", "", 0, err
	}
	dimension := cfg.Dimensions
	if dimension <= 0 {
		err = s.pool.QueryRow(ctx, `
			SELECT dimension
			FROM asset_embedding_objects
			WHERE provider_id = $1::uuid AND model = $2 AND object_type = 'shot' AND status = 'ready'
			GROUP BY dimension
			ORDER BY COUNT(*) DESC
			LIMIT 1`, providerID, model).Scan(&dimension)
		if err != nil {
			if err == pgx.ErrNoRows {
				return providerID, model, 0, nil
			}
			return "", "", 0, err
		}
	}
	return providerID, model, dimension, nil
}

func (s *AssetEmbeddingService) ensureSemanticProjection(ctx context.Context, providerID string, model string, dimension int) (semanticProjectionRun, error) {
	if dimension <= 0 {
		return semanticProjectionRun{}, nil
	}
	s.semanticProjectionMu.Lock()
	defer s.semanticProjectionMu.Unlock()

	var sourceCount int
	var sourceUpdatedAt *time.Time
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(updated_at)
		FROM asset_embedding_objects
		WHERE provider_id = $1::uuid AND model = $2 AND dimension = $3
		  AND object_type = 'shot' AND status = 'ready'`, providerID, model, dimension).Scan(&sourceCount, &sourceUpdatedAt); err != nil {
		return semanticProjectionRun{}, err
	}
	if sourceCount == 0 {
		return semanticProjectionRun{}, nil
	}

	current, found, err := s.latestSemanticProjectionRun(ctx, providerID, model, dimension)
	if err != nil {
		return semanticProjectionRun{}, err
	}
	if found && current.Algorithm == semanticProjectionAlgorithm && current.SourceCount == sourceCount && sameOptionalTime(current.SourceUpdatedAt, sourceUpdatedAt) {
		return current, nil
	}
	run, buildErr := s.rebuildSemanticProjection(ctx, providerID, model, dimension, sourceCount, sourceUpdatedAt)
	if buildErr != nil && found {
		return current, nil
	}
	return run, buildErr
}

func (s *AssetEmbeddingService) latestSemanticProjectionRun(ctx context.Context, providerID string, model string, dimension int) (semanticProjectionRun, bool, error) {
	var run semanticProjectionRun
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, provider_id::text, model, dimension, algorithm,
		       source_count, projected_count, source_updated_at, updated_at
		FROM asset_semantic_projection_runs
		WHERE provider_id = $1::uuid AND model = $2 AND dimension = $3 AND status = 'ready'
		ORDER BY created_at DESC
		LIMIT 1`, providerID, model, dimension).Scan(
		&run.ID, &run.ProviderID, &run.Model, &run.Dimension, &run.Algorithm,
		&run.SourceCount, &run.ProjectedCount, &run.SourceUpdatedAt, &run.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return semanticProjectionRun{}, false, nil
	}
	if err != nil {
		return semanticProjectionRun{}, false, err
	}
	return run, true, nil
}

func (s *AssetEmbeddingService) rebuildSemanticProjection(ctx context.Context, providerID string, model string, dimension int, sourceCount int, sourceUpdatedAt *time.Time) (semanticProjectionRun, error) {
	runID := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO asset_semantic_projection_runs (
			id, provider_id, model, dimension, algorithm, source_count, source_updated_at, status
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'building')`,
		runID, providerID, model, dimension, semanticProjectionAlgorithm, sourceCount, sourceUpdatedAt)
	if err != nil {
		return semanticProjectionRun{}, err
	}

	sources, err := s.loadSemanticProjectionSources(ctx, providerID, model, dimension)
	if err == nil {
		var coordinates []semanticProjectionCoordinate
		coordinates, err = projectSemanticEmbeddings(sources)
		if err == nil {
			err = s.storeSemanticProjection(ctx, runID, coordinates)
		}
	}
	if err != nil {
		_, _ = s.pool.Exec(context.Background(), `
			UPDATE asset_semantic_projection_runs
			SET status = 'failed', error_message = $2, updated_at = now()
			WHERE id = $1::uuid`, runID, err.Error())
		return semanticProjectionRun{}, err
	}

	var run semanticProjectionRun
	err = s.pool.QueryRow(ctx, `
		UPDATE asset_semantic_projection_runs
		SET status = 'ready', projected_count = $2, error_message = '', updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, provider_id::text, model, dimension, algorithm,
		          source_count, projected_count, source_updated_at, updated_at`, runID, len(sources)).Scan(
		&run.ID, &run.ProviderID, &run.Model, &run.Dimension, &run.Algorithm,
		&run.SourceCount, &run.ProjectedCount, &run.SourceUpdatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return semanticProjectionRun{}, err
	}
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM asset_semantic_projection_runs
		WHERE provider_id = $1::uuid AND model = $2 AND dimension = $3
		  AND id <> $4::uuid AND status IN ('ready', 'failed')`, providerID, model, dimension, runID)
	return run, nil
}

func (s *AssetEmbeddingService) loadSemanticProjectionSources(ctx context.Context, providerID string, model string, dimension int) ([]semanticProjectionSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT asset_id::text, embedding
		FROM asset_embedding_objects
		WHERE provider_id = $1::uuid AND model = $2 AND dimension = $3
		  AND object_type = 'shot' AND status = 'ready'
		ORDER BY md5(asset_id::text)
		LIMIT $4`, providerID, model, dimension, semanticProjectionSourceCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := make([]semanticProjectionSource, 0)
	for rows.Next() {
		var source semanticProjectionSource
		var vector pgvector.Vector
		if err := rows.Scan(&source.AssetID, &vector); err != nil {
			return nil, err
		}
		source.Embedding = append([]float32(nil), vector.Slice()...)
		if len(source.Embedding) != dimension {
			return nil, fmt.Errorf("asset %s embedding dimension mismatch: expected %d, got %d", source.AssetID, dimension, len(source.Embedding))
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *AssetEmbeddingService) storeSemanticProjection(ctx context.Context, runID string, coordinates []semanticProjectionCoordinate) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	batch := &pgx.Batch{}
	for _, point := range coordinates {
		batch.Queue(`
			INSERT INTO asset_semantic_projection_points (
				projection_id, asset_id, x2, y2, x3, y3, z3
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)`,
			runID, point.AssetID, point.X2, point.Y2, point.X3, point.Y3, point.Z3)
	}
	results := tx.SendBatch(ctx, batch)
	for range coordinates {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AssetEmbeddingService) listSemanticProjectionPoints(ctx context.Context, run semanticProjectionRun, filters AssetFilters) ([]AssetSemanticSpacePoint, int, int, error) {
	args := []any{run.ID}
	assetConditions, args := buildSemanticAssetFilterConditions("a", filters, args)
	conditions := append([]string{"p.projection_id = $1::uuid"}, assetConditions...)
	whereClause := strings.Join(conditions, "\n AND ")
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM asset_semantic_projection_points p
		JOIN assets a ON a.id = p.asset_id
		WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, 0, err
	}

	limitPlaceholder := semanticSearchBind(&args, semanticSpacePointLimit)
	rows, err := s.pool.Query(ctx, `
		SELECT p.asset_id::text, p.x2, p.y2, p.x3, p.y3, p.z3,
		       a.product_id::text, COALESCE(a.asset_name, ''), a.storage_key, a.file_name,
		       a.source_type, a.status, COALESCE(a.usability_status, ''), COALESCE(a.duration_ms, 0),
		       COALESCE(a.shot_size, ''), COALESCE(a.scene_description, ''), COALESCE(a.action_description, ''),
		       COALESCE(frame.storage_key, '')
		FROM asset_semantic_projection_points p
		JOIN assets a ON a.id = p.asset_id
		LEFT JOIN LATERAL (
			SELECT storage_key
			FROM asset_frame_snapshots
			WHERE asset_id = a.id
			ORDER BY frame_index ASC
			LIMIT 1
		) frame ON true
		WHERE `+whereClause+`
		ORDER BY md5(p.asset_id::text)
		LIMIT `+limitPlaceholder, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	points := make([]AssetSemanticSpacePoint, 0, min(total, semanticSpacePointLimit))
	for rows.Next() {
		var point AssetSemanticSpacePoint
		var thumbnailStorageKey string
		if err := rows.Scan(
			&point.AssetID, &point.X2, &point.Y2, &point.X3, &point.Y3, &point.Z3,
			&point.ProductID, &point.AssetName, &point.StorageKey, &point.FileName,
			&point.SourceType, &point.Status, &point.UsabilityStatus, &point.DurationMs,
			&point.ShotSize, &point.SceneDescription, &point.ActionDescription, &thumbnailStorageKey,
		); err != nil {
			return nil, 0, 0, err
		}
		if thumbnailStorageKey != "" {
			point.ThumbnailURL = "/storage/" + thumbnailStorageKey
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	allAssetArgs := []any{}
	allAssetConditions, allAssetArgs := buildSemanticAssetFilterConditions("a", filters, allAssetArgs)
	allAssetWhere := "TRUE"
	if len(allAssetConditions) > 0 {
		allAssetWhere = strings.Join(allAssetConditions, "\n AND ")
	}
	var matchingAssets int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM assets a WHERE "+allAssetWhere, allAssetArgs...).Scan(&matchingAssets); err != nil {
		return nil, 0, 0, err
	}
	missing := matchingAssets - total
	if missing < 0 {
		missing = 0
	}
	return points, total, missing, nil
}

func projectSemanticEmbeddings(sources []semanticProjectionSource) ([]semanticProjectionCoordinate, error) {
	if len(sources) == 0 {
		return []semanticProjectionCoordinate{}, nil
	}
	dimension := len(sources[0].Embedding)
	if dimension == 0 {
		return nil, fmt.Errorf("semantic projection source is empty")
	}
	reducedDimension := min(32, dimension)
	type sparseProjectionWeight struct {
		component int
		weight    float64
	}
	sparseWeights := make([][]sparseProjectionWeight, dimension)
	for featureIndex := 0; featureIndex < dimension; featureIndex++ {
		for component := 0; component < reducedDimension; component++ {
			weight := semanticProjectionWeight(featureIndex, component)
			if weight != 0 {
				sparseWeights[featureIndex] = append(sparseWeights[featureIndex], sparseProjectionWeight{component: component, weight: weight})
			}
		}
	}
	projected := make([][]float64, len(sources))
	means := make([]float64, reducedDimension)
	for rowIndex, source := range sources {
		if len(source.Embedding) != dimension {
			return nil, fmt.Errorf("semantic projection embeddings have inconsistent dimensions")
		}
		projected[rowIndex] = make([]float64, reducedDimension)
		var normSquared float64
		for _, value := range source.Embedding {
			normSquared += float64(value) * float64(value)
		}
		norm := math.Sqrt(normSquared)
		if norm == 0 {
			norm = 1
		}
		for featureIndex, value := range source.Embedding {
			normalized := float64(value) / norm
			for _, projectionWeight := range sparseWeights[featureIndex] {
				projected[rowIndex][projectionWeight.component] += normalized * projectionWeight.weight
			}
		}
		for component := range projected[rowIndex] {
			projected[rowIndex][component] /= math.Sqrt(float64(reducedDimension))
			means[component] += projected[rowIndex][component]
		}
	}
	for component := range means {
		means[component] /= float64(len(projected))
	}
	for rowIndex := range projected {
		for component := range projected[rowIndex] {
			projected[rowIndex][component] -= means[component]
		}
	}

	coordinates := make([][3]float64, len(sources))
	if len(sources) > 1 {
		covariance := mat.NewSymDense(reducedDimension, nil)
		denominator := float64(len(sources) - 1)
		for left := 0; left < reducedDimension; left++ {
			for right := left; right < reducedDimension; right++ {
				var sum float64
				for rowIndex := range projected {
					sum += projected[rowIndex][left] * projected[rowIndex][right]
				}
				covariance.SetSym(left, right, sum/denominator)
			}
		}
		var eigen mat.EigenSym
		if ok := eigen.Factorize(covariance, true); !ok {
			return nil, fmt.Errorf("semantic projection PCA failed")
		}
		values := eigen.Values(nil)
		var vectors mat.Dense
		eigen.VectorsTo(&vectors)
		componentCount := min(3, reducedDimension)
		for outputComponent := 0; outputComponent < componentCount; outputComponent++ {
			eigenColumn := len(values) - 1 - outputComponent
			sign := stableEigenvectorSign(&vectors, eigenColumn)
			for rowIndex := range projected {
				var value float64
				for component := 0; component < reducedDimension; component++ {
					value += projected[rowIndex][component] * vectors.At(component, eigenColumn) * sign
				}
				coordinates[rowIndex][outputComponent] = value
			}
		}
	}
	normalizeSemanticCoordinates(coordinates)
	result := make([]semanticProjectionCoordinate, len(sources))
	for index, source := range sources {
		result[index] = semanticProjectionCoordinate{
			AssetID: source.AssetID,
			X2:      coordinates[index][0],
			Y2:      coordinates[index][1],
			X3:      coordinates[index][0],
			Y3:      coordinates[index][1],
			Z3:      coordinates[index][2],
		}
	}
	return result, nil
}

func semanticProjectionWeight(featureIndex int, component int) float64 {
	value := uint64(featureIndex+1)*0x9e3779b185ebca87 ^ uint64(component+1)*0xc2b2ae3d27d4eb4f
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	switch value % 6 {
	case 0:
		return math.Sqrt(3)
	case 1:
		return -math.Sqrt(3)
	default:
		return 0
	}
}

func stableEigenvectorSign(vectors *mat.Dense, column int) float64 {
	largest := 0.0
	sign := 1.0
	rows, _ := vectors.Dims()
	for row := 0; row < rows; row++ {
		value := vectors.At(row, column)
		if math.Abs(value) > largest {
			largest = math.Abs(value)
			if value < 0 {
				sign = -1
			} else {
				sign = 1
			}
		}
	}
	return sign
}

func normalizeSemanticCoordinates(coordinates [][3]float64) {
	for axis := 0; axis < 3; axis++ {
		absoluteValues := make([]float64, len(coordinates))
		for index := range coordinates {
			absoluteValues[index] = math.Abs(coordinates[index][axis])
		}
		sort.Float64s(absoluteValues)
		percentileIndex := int(math.Floor(float64(max(0, len(absoluteValues)-1)) * 0.95))
		scale := absoluteValues[percentileIndex]
		if scale < 1e-9 {
			scale = 1
		}
		for index := range coordinates {
			coordinates[index][axis] = math.Max(-1, math.Min(1, coordinates[index][axis]/scale))
		}
	}
}

func sameOptionalTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
