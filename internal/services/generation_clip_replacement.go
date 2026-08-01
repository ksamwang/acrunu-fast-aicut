package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	changed := false
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

		clip := result.Clips[clipIndex]
		if clip.SourceType != "visual_only" {
			return EditPlan{}, fmt.Errorf("%w: clip %q is not a TTS visual clip", ErrClipReplacementInvalid, clipID)
		}
		asset, exists := assets.GetAsset(assetID)
		if !exists || asset.ProductID != run.ProductID || strings.TrimSpace(asset.StorageKey) == "" {
			return EditPlan{}, fmt.Errorf("%w: asset %q is unavailable for this product", ErrClipReplacementInvalid, assetID)
		}
		if asset.Status != "ready" || asset.AnalysisStatus != "ready" ||
			(asset.UsabilityStatus != "usable" && asset.UsabilityStatus != "needs_review") || asset.SourceType != "visual_only" {
			return EditPlan{}, fmt.Errorf("%w: asset %q is not eligible", ErrClipReplacementInvalid, assetID)
		}
		durationMs := clip.TimelineDurationMs
		if durationMs <= 0 {
			durationMs = clip.EndMs - clip.StartMs
		}
		sourceOutMs := replacement.SourceInMs + durationMs
		if durationMs <= 0 || asset.DurationMs <= 0 || sourceOutMs > asset.DurationMs {
			return EditPlan{}, fmt.Errorf("%w: asset %q cannot cover the %dms clip from %dms", ErrClipReplacementInvalid, assetID, durationMs, replacement.SourceInMs)
		}

		if clip.AssetID != assetID || clip.SourceInMs != replacement.SourceInMs || clip.SourceOutMs != sourceOutMs || clip.UseOriginalAudio {
			changed = true
		}
		clip.AssetID = assetID
		clip.SpeechSegmentID = ""
		clip.SourceInMs = replacement.SourceInMs
		clip.SourceOutMs = sourceOutMs
		clip.SourceType = "visual_only"
		clip.UseOriginalAudio = false
		clip.AudioGainDB = 0
		result.Clips[clipIndex] = clip
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
