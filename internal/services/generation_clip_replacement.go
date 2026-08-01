package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

var (
	ErrEditPlanConflict       = errors.New("edit plan changed")
	ErrEditPlanClipNotFound   = errors.New("edit plan clip not found")
	ErrClipReplacementInvalid = errors.New("clip replacement is invalid")
)

type EditPlanClipReplacement struct {
	ClipID     string `json:"clip_id"`
	AssetID    string `json:"asset_id"`
	SourceInMs int    `json:"source_in_ms"`
}

type preparedEditPlanClipReplacement struct {
	EditPlanClipReplacement
	ClipIndex int
}

func MaterializeClipReplacementPlan(
	run GenerationRun,
	plan EditPlan,
	replacements []EditPlanClipReplacement,
	assets *ProductAssetService,
) (EditPlan, error) {
	if assets == nil || len(replacements) == 0 {
		return EditPlan{}, fmt.Errorf("%w: replacements are required", ErrClipReplacementInvalid)
	}
	if plan.Status != "ready" || len(plan.Clips) == 0 {
		return EditPlan{}, fmt.Errorf("%w: ready edit plan is required", ErrClipReplacementInvalid)
	}

	result := cloneEditPlan(plan)
	clipIndexes := make(map[string]int, len(result.Clips))
	for index, clip := range result.Clips {
		clipIndexes[normalizeID(clip.ID)] = index
	}

	seenClipIDs := make(map[string]struct{}, len(replacements))
	prepared := make([]preparedEditPlanClipReplacement, 0, len(replacements))
	for _, replacement := range replacements {
		clipID := normalizeID(replacement.ClipID)
		assetID := normalizeID(replacement.AssetID)
		if clipID == "" || assetID == "" || replacement.SourceInMs < 0 {
			return EditPlan{}, fmt.Errorf("%w: clip id, asset id, and source range are required", ErrClipReplacementInvalid)
		}
		if _, exists := seenClipIDs[clipID]; exists {
			return EditPlan{}, fmt.Errorf("%w: clip %q is repeated", ErrClipReplacementInvalid, clipID)
		}
		seenClipIDs[clipID] = struct{}{}
		clipIndex, exists := clipIndexes[clipID]
		if !exists {
			return EditPlan{}, fmt.Errorf("%w: %s", ErrEditPlanClipNotFound, clipID)
		}
		prepared = append(prepared, preparedEditPlanClipReplacement{
			EditPlanClipReplacement: EditPlanClipReplacement{
				ClipID: clipID, AssetID: assetID, SourceInMs: replacement.SourceInMs,
			},
			ClipIndex: clipIndex,
		})
	}
	sort.SliceStable(prepared, func(i, j int) bool { return prepared[i].ClipIndex > prepared[j].ClipIndex })

	changed := false
	for _, replacement := range prepared {
		replacementChanged, err := applyEditPlanClipReplacement(&result, replacement, run.ProductID, assets)
		if err != nil {
			return EditPlan{}, err
		}
		changed = changed || replacementChanged
	}
	if !changed {
		return EditPlan{}, fmt.Errorf("%w: no clip changed", ErrClipReplacementInvalid)
	}
	if err := validateEditPlanForStorage(result); err != nil {
		return EditPlan{}, fmt.Errorf("%w: %v", ErrClipReplacementInvalid, err)
	}
	planJSON, err := editPlanJSONWithClips(result.PlanJSON, result.Clips)
	if err != nil {
		return EditPlan{}, fmt.Errorf("%w: update plan JSON: %v", ErrClipReplacementInvalid, err)
	}
	result.PlanJSON = planJSON
	return result, nil
}

func applyEditPlanClipReplacement(
	plan *EditPlan,
	replacement preparedEditPlanClipReplacement,
	productID string,
	assets *ProductAssetService,
) (bool, error) {
	clip := plan.Clips[replacement.ClipIndex]
	if clip.SourceType != "visual_only" {
		return false, fmt.Errorf("%w: clip %q is not a TTS visual clip", ErrClipReplacementInvalid, replacement.ClipID)
	}
	asset, exists := assets.GetAsset(replacement.AssetID)
	if !exists || asset.ProductID != productID || strings.TrimSpace(asset.StorageKey) == "" {
		return false, fmt.Errorf("%w: asset %q is unavailable for this product", ErrClipReplacementInvalid, replacement.AssetID)
	}
	if asset.Status != "ready" || asset.AnalysisStatus != "ready" ||
		(asset.UsabilityStatus != "usable" && asset.UsabilityStatus != "needs_review") || asset.SourceType != "visual_only" {
		return false, fmt.Errorf("%w: asset %q is not eligible", ErrClipReplacementInvalid, replacement.AssetID)
	}
	durationMs := clip.TimelineDurationMs
	if durationMs <= 0 {
		durationMs = clip.EndMs - clip.StartMs
	}
	availableDurationMs := asset.DurationMs - replacement.SourceInMs
	if durationMs <= 0 || availableDurationMs <= 0 {
		return false, fmt.Errorf("%w: asset %q has no usable source range from %dms", ErrClipReplacementInvalid, replacement.AssetID, replacement.SourceInMs)
	}
	shortfallMs := max(0, durationMs-availableDurationMs)
	if shortfallMs > 0 {
		if replacement.ClipIndex == len(plan.Clips)-1 {
			return false, fmt.Errorf("%w: last clip %q requires the full %dms source duration", ErrClipReplacementInvalid, replacement.ClipID, durationMs)
		}
		if shortfallMs > modelgateway.MaximumEditPlanEarlyTransitionMs {
			return false, fmt.Errorf("%w: asset %q is %dms short; maximum early transition is %dms", ErrClipReplacementInvalid, replacement.AssetID, shortfallMs, modelgateway.MaximumEditPlanEarlyTransitionMs)
		}
		if durationMs-shortfallMs < modelgateway.MinimumEditPlanClipDurationMs {
			return false, fmt.Errorf("%w: clip %q would be shorter than %dms", ErrClipReplacementInvalid, replacement.ClipID, modelgateway.MinimumEditPlanClipDurationMs)
		}
		if err := moveEditPlanBoundaryEarlier(plan, replacement.ClipIndex, shortfallMs, assets); err != nil {
			return false, err
		}
		clip = plan.Clips[replacement.ClipIndex]
		durationMs = clip.TimelineDurationMs
	}
	sourceOutMs := replacement.SourceInMs + durationMs
	if sourceOutMs > asset.DurationMs {
		return false, fmt.Errorf("%w: asset %q cannot cover the %dms clip from %dms", ErrClipReplacementInvalid, replacement.AssetID, durationMs, replacement.SourceInMs)
	}
	changed := clip.AssetID != replacement.AssetID || clip.SourceInMs != replacement.SourceInMs ||
		clip.SourceOutMs != sourceOutMs || clip.UseOriginalAudio || shortfallMs > 0
	clip.AssetID = replacement.AssetID
	clip.SpeechSegmentID = ""
	clip.SourceInMs = replacement.SourceInMs
	clip.SourceOutMs = sourceOutMs
	clip.SourceType = "visual_only"
	clip.UseOriginalAudio = false
	clip.AudioGainDB = 0
	plan.Clips[replacement.ClipIndex] = clip
	return changed, nil
}

func moveEditPlanBoundaryEarlier(plan *EditPlan, clipIndex int, shortfallMs int, assets *ProductAssetService) error {
	current := plan.Clips[clipIndex]
	next := plan.Clips[clipIndex+1]
	if next.TimelineDurationMs+shortfallMs > modelgateway.MaximumEditPlanClipDurationMs {
		return fmt.Errorf("%w: next clip %q cannot absorb a %dms early transition", ErrClipReplacementInvalid, next.ID, shortfallMs)
	}
	nextAsset, exists := assets.GetAsset(next.AssetID)
	if !exists || nextAsset.DurationMs <= 0 || strings.TrimSpace(nextAsset.StorageKey) == "" {
		return fmt.Errorf("%w: next clip %q source asset is unavailable", ErrClipReplacementInvalid, next.ID)
	}
	prependMs := min(shortfallMs, next.SourceInMs)
	appendMs := shortfallMs - prependMs
	if next.SourceOutMs+appendMs > nextAsset.DurationMs {
		return fmt.Errorf("%w: next clip %q has no source range for a %dms early transition", ErrClipReplacementInvalid, next.ID, shortfallMs)
	}

	current.EndMs -= shortfallMs
	current.TimelineDurationMs -= shortfallMs
	next.StartMs -= shortfallMs
	next.TimelineDurationMs += shortfallMs
	next.SourceInMs -= prependMs
	next.SourceOutMs += appendMs
	plan.Clips[clipIndex] = current
	plan.Clips[clipIndex+1] = next
	return nil
}

func editPlanJSONWithClips(raw json.RawMessage, clips []EditPlanClip) (json.RawMessage, error) {
	payload := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	payload["clips"] = clips
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}
