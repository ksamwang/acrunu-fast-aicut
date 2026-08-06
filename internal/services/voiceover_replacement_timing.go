package services

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func RetimeEditPlanForVoiceover(
	plan EditPlan,
	oldWork VoiceoverWork,
	newWork VoiceoverWork,
	newScriptVariantID string,
	newVoiceoverID string,
	assets *ProductAssetService,
) (EditPlan, error) {
	if assets == nil || plan.Status != "ready" || len(plan.VisualBeats) == 0 || len(plan.Clips) == 0 {
		return EditPlan{}, fmt.Errorf("ready edit plan and assets are required")
	}
	if strings.Join(strings.Fields(oldWork.ScriptText), "") != strings.Join(strings.Fields(newWork.ScriptText), "") {
		return EditPlan{}, fmt.Errorf("replacement voiceover script differs from the finished work")
	}
	if len(oldWork.NarrationSegments) == 0 || len(oldWork.NarrationSegments) != len(newWork.NarrationSegments) {
		return EditPlan{}, fmt.Errorf("replacement voiceover narration structure changed")
	}
	for index := range oldWork.NarrationSegments {
		if strings.TrimSpace(oldWork.NarrationSegments[index].Text) != strings.TrimSpace(newWork.NarrationSegments[index].Text) {
			return EditPlan{}, fmt.Errorf("replacement voiceover narration segment %d changed", index+1)
		}
	}

	newPauses, err := remapNarrationPauses(plan.NarrationPauses, oldWork.NarrationSegments, newWork.NarrationSegments)
	if err != nil {
		return EditPlan{}, err
	}
	newNarration := shiftNarrationSegments(newWork.NarrationSegments, newPauses)
	oldNarration := plan.NarrationSegments
	if len(oldNarration) != len(newNarration) {
		return EditPlan{}, fmt.Errorf("edit plan narration structure changed")
	}
	newTimelineDuration := newWork.DurationMs
	for _, pause := range newPauses {
		newTimelineDuration += pause.DurationMs
	}

	oldNarrationIndex := make(map[string]int, len(oldNarration))
	for index, segment := range oldNarration {
		oldNarrationIndex[segment.ID] = index
	}
	newPlan := cloneEditPlan(plan)
	newPlan.ScriptVariantID = normalizeID(newScriptVariantID)
	newPlan.VoiceoverID = normalizeID(newVoiceoverID)
	newPlan.SourceDurationMs = newWork.DurationMs
	newPlan.TimelineDurationMs = newTimelineDuration
	newPlan.NarrationSegments = cloneNarrationSegments(newNarration)
	newPlan.NarrationPauses = cloneNarrationPauses(newPauses)

	previousBeatEnd := 0
	for index := range newPlan.VisualBeats {
		oldBeat := plan.VisualBeats[index]
		newStart := mapReplacementTimelinePoint(oldBeat.StartMs, oldNarration, newNarration, plan.TimelineDurationMs, newTimelineDuration)
		newEnd := mapReplacementTimelinePoint(oldBeat.EndMs, oldNarration, newNarration, plan.TimelineDurationMs, newTimelineDuration)
		if index == 0 {
			newStart = 0
		} else {
			newStart = previousBeatEnd
		}
		if index == len(newPlan.VisualBeats)-1 {
			newEnd = newTimelineDuration
		}
		if newEnd <= newStart {
			return EditPlan{}, fmt.Errorf("replacement voiceover makes visual beat %d empty", index+1)
		}
		newPlan.VisualBeats[index].StartMs = newStart
		newPlan.VisualBeats[index].EndMs = newEnd
		if narrationIndex, ok := oldNarrationIndex[oldBeat.NarrationSegmentID]; ok {
			newPlan.VisualBeats[index].NarrationSegmentID = newNarration[narrationIndex].ID
		} else {
			return EditPlan{}, fmt.Errorf("visual beat %d narration segment is missing", index+1)
		}
		if !isVisualBeatDurationValid(newPlan.VisualBeats[index].DurationClass, newEnd-newStart) {
			if newPlan.VisualBeats[index].DurationClass == VisualBeatDurationAction {
				return EditPlan{}, fmt.Errorf("new voiceover leaves action beat %d shorter than 2800ms; use full regeneration", index+1)
			}
			newPlan.VisualBeats[index].DurationClass = unpaddedVisualBeatDurationClass(newEnd - newStart)
		}
		previousBeatEnd = newEnd
	}

	clipsByBeat := make(map[string][]int, len(newPlan.VisualBeats))
	for index, clip := range plan.Clips {
		clipsByBeat[clip.VisualBeatID] = append(clipsByBeat[clip.VisualBeatID], index)
	}
	newClips := cloneEditPlanClips(plan.Clips)
	clipStart := 0
	for beatIndex, beat := range newPlan.VisualBeats {
		clipIndexes := clipsByBeat[beat.ID]
		if len(clipIndexes) == 0 {
			return EditPlan{}, fmt.Errorf("visual beat %d has no existing material", beatIndex+1)
		}
		durations, err := allocateReplacementClipDurations(beat, clipIndexes, plan.Clips, assets)
		if err != nil {
			return EditPlan{}, fmt.Errorf("visual beat %d: %w; use full regeneration", beatIndex+1, err)
		}
		for offset, clipIndex := range clipIndexes {
			oldClip := plan.Clips[clipIndex]
			asset, ok := assets.GetAsset(oldClip.AssetID)
			if !ok || asset.DurationMs < durations[offset] {
				return EditPlan{}, fmt.Errorf("visual beat %d material is too short; use full regeneration", beatIndex+1)
			}
			sourceCenter := (oldClip.SourceInMs + oldClip.SourceOutMs) / 2
			sourceIn := sourceCenter - durations[offset]/2
			if sourceIn < 0 {
				sourceIn = 0
			}
			if sourceIn+durations[offset] > asset.DurationMs {
				sourceIn = asset.DurationMs - durations[offset]
			}
			newClips[clipIndex].NarrationSegmentID = beat.NarrationSegmentID
			newClips[clipIndex].StartMs = clipStart
			newClips[clipIndex].EndMs = clipStart + durations[offset]
			newClips[clipIndex].TimelineDurationMs = durations[offset]
			newClips[clipIndex].SourceInMs = sourceIn
			newClips[clipIndex].SourceOutMs = sourceIn + durations[offset]
			clipStart += durations[offset]
		}
	}
	if clipStart != newTimelineDuration {
		return EditPlan{}, fmt.Errorf("replacement clip timeline ends at %dms, want %dms", clipStart, newTimelineDuration)
	}
	newPlan.Clips = newClips
	if err := updateVoiceoverReplacementPlanArtifacts(&newPlan); err != nil {
		return EditPlan{}, err
	}
	if err := validateEditPlanForStorage(newPlan); err != nil {
		return EditPlan{}, err
	}
	return newPlan, nil
}

func updateVoiceoverReplacementPlanArtifacts(plan *EditPlan) error {
	for _, target := range []*json.RawMessage{&plan.CandidateSnapshot, &plan.PlanJSON} {
		payload := map[string]any{}
		if len(*target) > 0 {
			if err := json.Unmarshal(*target, &payload); err != nil {
				var legacy any
				if legacyErr := json.Unmarshal(*target, &legacy); legacyErr != nil {
					return fmt.Errorf("decode edit plan artifacts: %w", err)
				}
				payload = map[string]any{"candidate_sets": legacy}
			}
		}
		payload["source_duration_ms"] = plan.SourceDurationMs
		payload["timeline_duration_ms"] = plan.TimelineDurationMs
		payload["narration_segments"] = plan.NarrationSegments
		payload["narration_pauses"] = plan.NarrationPauses
		payload["visual_beats"] = plan.VisualBeats
		payload["clips"] = plan.Clips
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		*target = encoded
	}
	return nil
}

func remapNarrationPauses(pauses []NarrationPause, oldSegments []NarrationSegment, newSegments []NarrationSegment) ([]NarrationPause, error) {
	result := make([]NarrationPause, 0, len(pauses))
	for _, pause := range pauses {
		index := sort.Search(len(oldSegments), func(index int) bool { return oldSegments[index].EndMs >= pause.AfterMs })
		if index >= len(oldSegments) || absInt(oldSegments[index].EndMs-pause.AfterMs) > 50 {
			return nil, fmt.Errorf("editorial pause no longer maps to narration")
		}
		result = append(result, NarrationPause{AfterMs: newSegments[index].EndMs, DurationMs: pause.DurationMs})
	}
	return result, nil
}

func mapReplacementTimelinePoint(value int, oldSegments []NarrationSegment, newSegments []NarrationSegment, oldDuration int, newDuration int) int {
	if value <= 0 {
		return 0
	}
	if value >= oldDuration {
		return newDuration
	}
	for index, oldSegment := range oldSegments {
		newSegment := newSegments[index]
		if value <= oldSegment.EndMs {
			if value <= oldSegment.StartMs || oldSegment.EndMs == oldSegment.StartMs {
				return newSegment.StartMs
			}
			ratio := float64(value-oldSegment.StartMs) / float64(oldSegment.EndMs-oldSegment.StartMs)
			return newSegment.StartMs + int(math.Round(ratio*float64(newSegment.EndMs-newSegment.StartMs)))
		}
	}
	return newDuration
}

func allocateReplacementClipDurations(beat VisualBeat, clipIndexes []int, clips []EditPlanClip, assets *ProductAssetService) ([]int, error) {
	target := beat.EndMs - beat.StartMs
	minimums := make([]int, len(clipIndexes))
	maximums := make([]int, len(clipIndexes))
	weights := make([]int, len(clipIndexes))
	actionPrimary := -1
	for offset, clipIndex := range clipIndexes {
		clip := clips[clipIndex]
		asset, ok := assets.GetAsset(clip.AssetID)
		if !ok {
			return nil, fmt.Errorf("material %s is unavailable", clip.AssetID)
		}
		minimums[offset] = modelgateway.MinimumEditPlanClipDurationMs
		maximums[offset] = min(modelgateway.MaximumEditPlanClipDurationMs, asset.DurationMs)
		weights[offset] = max(1, clip.EndMs-clip.StartMs)
		if actionPrimary < 0 || weights[offset] > weights[actionPrimary] {
			actionPrimary = offset
		}
	}
	if beat.DurationClass == VisualBeatDurationAction {
		minimums[actionPrimary] = modelgateway.MinimumActionEditPlanClipDurationMs
	}
	minimumTotal, maximumTotal := 0, 0
	for index := range minimums {
		if maximums[index] < minimums[index] {
			return nil, fmt.Errorf("material %d cannot satisfy its minimum duration", index+1)
		}
		minimumTotal += minimums[index]
		maximumTotal += maximums[index]
	}
	if target < minimumTotal || target > maximumTotal {
		return nil, fmt.Errorf("existing materials support %d-%dms but the new beat requires %dms", minimumTotal, maximumTotal, target)
	}
	result := append([]int(nil), minimums...)
	remaining := target - minimumTotal
	for remaining > 0 {
		weightTotal := 0
		for index := range result {
			if result[index] < maximums[index] {
				weightTotal += weights[index]
			}
		}
		if weightTotal == 0 {
			return nil, fmt.Errorf("existing material duration capacity is exhausted")
		}
		allocated := 0
		for index := range result {
			capacity := maximums[index] - result[index]
			if capacity <= 0 {
				continue
			}
			addition := max(1, remaining*weights[index]/weightTotal)
			addition = min(addition, min(capacity, remaining-allocated))
			if addition <= 0 {
				continue
			}
			result[index] += addition
			allocated += addition
			if allocated == remaining {
				break
			}
		}
		remaining -= allocated
	}
	return result, nil
}
