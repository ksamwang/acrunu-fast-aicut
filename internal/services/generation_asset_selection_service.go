package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const generationAssetReservationLease = 10 * time.Minute

type AllocatePlannerSelectionInput struct {
	GenerationRunID   string
	GenerationBatchID string
	ProductID         string
	PlannerInput      modelgateway.EditPlanInput
	PreferredResult   modelgateway.EditPlanResult
}

func (s *AssetCandidateService) AllocatePlannerSelection(ctx context.Context, input AllocatePlannerSelectionInput) (modelgateway.EditPlanResult, error) {
	input.GenerationRunID = strings.TrimSpace(input.GenerationRunID)
	input.GenerationBatchID = strings.TrimSpace(input.GenerationBatchID)
	input.ProductID = strings.TrimSpace(input.ProductID)
	if input.GenerationRunID == "" || input.GenerationBatchID == "" || input.ProductID == "" {
		return modelgateway.EditPlanResult{}, fmt.Errorf("generation run, batch, and product are required for material allocation")
	}
	if err := modelgateway.ValidateEditPlanResult(input.PreferredResult, input.PlannerInput.Requirements); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	if s == nil || s.pool == nil {
		return input.PreferredResult, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, input.GenerationBatchID); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM generation_asset_selections
		WHERE state = 'reserved' AND expires_at <= now()`); err != nil {
		return modelgateway.EditPlanResult{}, err
	}

	previousReuseKeys, err := loadRunSelectionKeys(ctx, tx, input.GenerationRunID)
	if err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM generation_asset_selections
		WHERE generation_run_id = $1::uuid`, input.GenerationRunID); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	batchUseCounts, err := loadBatchSelectionCounts(ctx, tx, input.GenerationBatchID)
	if err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	recentUseCounts, err := loadRecentSelectionCounts(ctx, tx, input.ProductID, input.GenerationBatchID)
	if err != nil {
		return modelgateway.EditPlanResult{}, err
	}

	selected, err := selectDiversePlannerResult(
		input.PlannerInput,
		input.PreferredResult,
		batchUseCounts,
		recentUseCounts,
		previousReuseKeys,
	)
	if err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	expiresAt := time.Now().Add(generationAssetReservationLease)
	for _, choice := range selected.Clips {
		candidate, ok := plannerCandidateForChoice(input.PlannerInput.Requirements, choice)
		if !ok {
			return modelgateway.EditPlanResult{}, fmt.Errorf("allocated material %q is unavailable for slot %q", choice.CandidateID, choice.SlotID)
		}
		reuseKey := plannerCandidateReuseKey(candidate)
		if _, err := tx.Exec(ctx, `
			INSERT INTO generation_asset_selections (
				generation_run_id, generation_batch_id, asset_id, reuse_key, state, expires_at
			) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'reserved', $5)
			ON CONFLICT (generation_run_id, reuse_key) DO UPDATE SET
				generation_batch_id = EXCLUDED.generation_batch_id,
				asset_id = EXCLUDED.asset_id,
				state = 'reserved',
				expires_at = EXCLUDED.expires_at,
				updated_at = now()`,
			input.GenerationRunID,
			input.GenerationBatchID,
			candidate.AssetID,
			reuseKey,
			expiresAt,
		); err != nil {
			return modelgateway.EditPlanResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	return selected, nil
}

func loadRunSelectionKeys(ctx context.Context, tx pgx.Tx, runID string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT reuse_key
		FROM generation_asset_selections
		WHERE generation_run_id = $1::uuid`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var reuseKey string
		if err := rows.Scan(&reuseKey); err != nil {
			return nil, err
		}
		result[reuseKey] = true
	}
	return result, rows.Err()
}

func loadBatchSelectionCounts(ctx context.Context, tx pgx.Tx, batchID string) (map[string]int, error) {
	rows, err := tx.Query(ctx, `
		SELECT reuse_key, COUNT(*)::int
		FROM generation_asset_selections
		WHERE generation_batch_id = $1::uuid
		  AND (
			state = 'committed'
			OR (state = 'reserved' AND expires_at > now())
		  )
		GROUP BY reuse_key`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var reuseKey string
		var count int
		if err := rows.Scan(&reuseKey, &count); err != nil {
			return nil, err
		}
		result[reuseKey] = count
	}
	return result, rows.Err()
}

func loadRecentSelectionCounts(ctx context.Context, tx pgx.Tx, productID string, batchID string) (map[string]int, error) {
	rows, err := tx.Query(ctx, `
		SELECT selections.reuse_key, COUNT(*)::int
		FROM generation_asset_selections selections
		JOIN generation_runs runs ON runs.id = selections.generation_run_id
		WHERE runs.product_id = $1::uuid
		  AND selections.generation_batch_id <> $2::uuid
		  AND selections.state = 'committed'
		  AND selections.created_at >= now() - INTERVAL '30 days'
		GROUP BY selections.reuse_key`, productID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var reuseKey string
		var count int
		if err := rows.Scan(&reuseKey, &count); err != nil {
			return nil, err
		}
		result[reuseKey] = count
	}
	return result, rows.Err()
}

type plannerMaterialEdge struct {
	to          int
	reverse     int
	capacity    int
	cost        int64
	candidateID string
}

func selectDiversePlannerResult(
	input modelgateway.EditPlanInput,
	preferred modelgateway.EditPlanResult,
	batchUseCounts map[string]int,
	recentUseCounts map[string]int,
	previousReuseKeys map[string]bool,
) (modelgateway.EditPlanResult, error) {
	if err := modelgateway.ValidateEditPlanResult(preferred, input.Requirements); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	slots := flattenPlannerSlots(input.Requirements)
	if len(slots) == 0 {
		return modelgateway.EditPlanResult{}, fmt.Errorf("planner slots are required")
	}
	preferredBySlot := make(map[string]string, len(preferred.Clips))
	for _, choice := range preferred.Clips {
		preferredBySlot[choice.SlotID] = choice.CandidateID
	}

	materialIndexes := map[string]int{}
	materialKeys := make([]string, 0)
	for _, slot := range slots {
		for _, candidate := range slot.Candidates {
			reuseKey := plannerCandidateReuseKey(candidate)
			if _, exists := materialIndexes[reuseKey]; exists {
				continue
			}
			materialIndexes[reuseKey] = len(materialKeys)
			materialKeys = append(materialKeys, reuseKey)
		}
	}
	if len(materialKeys) < len(slots) {
		return modelgateway.EditPlanResult{}, fmt.Errorf("no globally unique material assignment is available")
	}

	sourceNode := 0
	firstSlotNode := 1
	firstMaterialNode := firstSlotNode + len(slots)
	sinkNode := firstMaterialNode + len(materialKeys)
	graph := make([][]plannerMaterialEdge, sinkNode+1)
	addPlannerMaterialEdge := func(from int, to int, capacity int, cost int64, candidateID string) {
		forward := plannerMaterialEdge{to: to, reverse: len(graph[to]), capacity: capacity, cost: cost, candidateID: candidateID}
		reverse := plannerMaterialEdge{to: from, reverse: len(graph[from]), capacity: 0, cost: -cost}
		graph[from] = append(graph[from], forward)
		graph[to] = append(graph[to], reverse)
	}
	for slotIndex := range slots {
		addPlannerMaterialEdge(sourceNode, firstSlotNode+slotIndex, 1, 0, "")
	}
	for materialIndex := range materialKeys {
		addPlannerMaterialEdge(firstMaterialNode+materialIndex, sinkNode, 1, 0, "")
	}
	for slotIndex, slot := range slots {
		bestSemanticScore := math.Inf(-1)
		for _, candidate := range slot.Candidates {
			if candidate.SemanticScore > bestSemanticScore {
				bestSemanticScore = candidate.SemanticScore
			}
		}
		for candidateIndex, candidate := range slot.Candidates {
			reuseKey := plannerCandidateReuseKey(candidate)
			effectiveBatchUse := batchUseCounts[reuseKey]
			if previousReuseKeys[reuseKey] {
				effectiveBatchUse++
			}
			recentUse := recentUseCounts[reuseKey]
			if candidate.RecentUseCount > recentUse {
				recentUse = candidate.RecentUseCount
			}
			if recentUse > 50 {
				recentUse = 50
			}
			llmPenalty := int64(250)
			if preferredBySlot[slot.ID] == candidate.ID {
				llmPenalty = 0
			}
			semanticLoss := int64(0)
			if !math.IsInf(bestSemanticScore, -1) && bestSemanticScore > candidate.SemanticScore {
				semanticLoss = int64(math.Round((bestSemanticScore - candidate.SemanticScore) * 10_000))
			}
			semanticFallbackPenalty := int64(0)
			if !candidate.SemanticQualified {
				semanticFallbackPenalty = 10_000_000
			}
			cost := semanticFallbackPenalty + int64(effectiveBatchUse)*1_000_000 +
				int64(recentUse)*100 + llmPenalty + semanticLoss + int64(candidateIndex)
			addPlannerMaterialEdge(
				firstSlotNode+slotIndex,
				firstMaterialNode+materialIndexes[reuseKey],
				1,
				cost,
				candidate.ID,
			)
		}
	}

	flow := 0
	for flow < len(slots) {
		distance := make([]int64, len(graph))
		previousNode := make([]int, len(graph))
		previousEdge := make([]int, len(graph))
		for index := range distance {
			distance[index] = math.MaxInt64
			previousNode[index] = -1
			previousEdge[index] = -1
		}
		distance[sourceNode] = 0
		for iteration := 0; iteration < len(graph)-1; iteration++ {
			updated := false
			for from := range graph {
				if distance[from] == math.MaxInt64 {
					continue
				}
				for edgeIndex, edge := range graph[from] {
					if edge.capacity <= 0 || distance[from]+edge.cost >= distance[edge.to] {
						continue
					}
					distance[edge.to] = distance[from] + edge.cost
					previousNode[edge.to] = from
					previousEdge[edge.to] = edgeIndex
					updated = true
				}
			}
			if !updated {
				break
			}
		}
		if previousNode[sinkNode] < 0 {
			return modelgateway.EditPlanResult{}, fmt.Errorf("no globally unique material assignment is available")
		}
		for node := sinkNode; node != sourceNode; node = previousNode[node] {
			from := previousNode[node]
			edgeIndex := previousEdge[node]
			reverseIndex := graph[from][edgeIndex].reverse
			graph[from][edgeIndex].capacity--
			graph[node][reverseIndex].capacity++
		}
		flow++
	}

	result := modelgateway.EditPlanResult{Clips: make([]modelgateway.EditPlanClipChoice, 0, len(slots))}
	for slotIndex, slot := range slots {
		candidateID := ""
		for _, edge := range graph[firstSlotNode+slotIndex] {
			if edge.candidateID != "" && edge.capacity == 0 {
				candidateID = edge.candidateID
				break
			}
		}
		if candidateID == "" {
			return modelgateway.EditPlanResult{}, fmt.Errorf("slot %q was not assigned a material", slot.ID)
		}
		result.Clips = append(result.Clips, modelgateway.EditPlanClipChoice{SlotID: slot.ID, CandidateID: candidateID})
	}
	if err := modelgateway.ValidateEditPlanResult(result, input.Requirements); err != nil {
		return modelgateway.EditPlanResult{}, err
	}
	return result, nil
}

func flattenPlannerSlots(requirements []modelgateway.EditPlanRequirement) []modelgateway.EditPlanSlot {
	result := make([]modelgateway.EditPlanSlot, 0)
	for _, requirement := range requirements {
		result = append(result, requirement.Slots...)
	}
	return result
}

func plannerCandidateForChoice(requirements []modelgateway.EditPlanRequirement, choice modelgateway.EditPlanClipChoice) (modelgateway.EditPlanCandidate, bool) {
	for _, requirement := range requirements {
		for _, slot := range requirement.Slots {
			if slot.ID != choice.SlotID {
				continue
			}
			for _, candidate := range slot.Candidates {
				if candidate.ID == choice.CandidateID {
					return candidate, true
				}
			}
		}
	}
	return modelgateway.EditPlanCandidate{}, false
}

func plannerCandidateReuseKey(candidate modelgateway.EditPlanCandidate) string {
	if reuseKey := strings.TrimSpace(candidate.ReuseKey); reuseKey != "" {
		return strings.ToLower(reuseKey)
	}
	if assetID := strings.TrimSpace(candidate.AssetID); assetID != "" {
		return strings.ToLower(assetID)
	}
	return strings.ToLower(strings.TrimSpace(candidate.ID))
}
