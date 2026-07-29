package services

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	minimumSemanticSynthesisRunes  = 4
	targetSemanticSynthesisRunes   = 30
	maximumSemanticSynthesisRunes  = 35
	maximumNarrationSynthesisUnits = 512

	commaSynthesisPauseMs     = 120
	colonSynthesisPauseMs     = 160
	semicolonSynthesisPauseMs = 180
	sentenceSynthesisPauseMs  = 260
	lengthSynthesisPauseMs    = 80
)

type narrationSynthesisUnit struct {
	Text              string
	PauseAfterMs      int
	CaptionStartIndex int
	CaptionEndIndex   int
}

type synthesizedNarrationUnit struct {
	Text              string
	CaptionStartIndex int
	CaptionEndIndex   int
	StartMs           int
	SpeechEndMs       int
	EndMs             int
}

type synthesisBoundaryKind int

const (
	synthesisBoundaryNone synthesisBoundaryKind = iota
	synthesisBoundaryComma
	synthesisBoundaryColon
	synthesisBoundarySemicolon
	synthesisBoundarySentence
)

func splitNarrationSynthesisUnits(script string) []narrationSynthesisUnit {
	captions := splitNarrationSentences(script)
	if len(captions) == 0 {
		return nil
	}

	units := make([]narrationSynthesisUnit, 0, len(captions))
	startIndex := 0
	var current strings.Builder
	for index, caption := range captions {
		current.WriteString(caption)
		kind := synthesisBoundary(caption)
		contentRunes := synthesisContentRuneCount(current.String())
		nextRunes := nextSynthesisClauseRunes(captions, index+1)
		shouldSplit := false
		switch kind {
		case synthesisBoundarySentence:
			shouldSplit = true
		case synthesisBoundarySemicolon, synthesisBoundaryColon:
			shouldSplit = contentRunes >= minimumSemanticSynthesisRunes
		case synthesisBoundaryComma:
			shouldSplit = contentRunes >= minimumSemanticSynthesisRunes &&
				nextRunes >= minimumSemanticSynthesisRunes &&
				!hasDependentClauseEnding(current.String())
		}
		if contentRunes >= targetSemanticSynthesisRunes && kind != synthesisBoundaryNone {
			shouldSplit = true
		}
		if contentRunes >= maximumSemanticSynthesisRunes {
			shouldSplit = true
		}
		if index == len(captions)-1 {
			shouldSplit = true
		}
		if !shouldSplit {
			continue
		}

		text := strings.TrimSpace(current.String())
		if text != "" {
			units = append(units, narrationSynthesisUnit{
				Text:              text,
				PauseAfterMs:      synthesisPauseForBoundary(kind),
				CaptionStartIndex: startIndex,
				CaptionEndIndex:   index + 1,
			})
		}
		current.Reset()
		startIndex = index + 1
	}
	units = compactNarrationSynthesisUnits(units, maximumNarrationSynthesisUnits)
	if len(units) > 0 {
		units[len(units)-1].PauseAfterMs = 0
	}
	return units
}

func synthesisBoundary(text string) synthesisBoundaryKind {
	runes := []rune(text)
	for index := len(runes) - 1; index >= 0; index-- {
		value := runes[index]
		if unicode.IsSpace(value) || isClosingNarrationPunctuation(value) {
			continue
		}
		switch value {
		case '。', '！', '？', '…', '．', '.', '!', '?':
			return synthesisBoundarySentence
		case '；', ';':
			return synthesisBoundarySemicolon
		case '：', ':':
			return synthesisBoundaryColon
		case '，', ',':
			return synthesisBoundaryComma
		default:
			return synthesisBoundaryNone
		}
	}
	return synthesisBoundaryNone
}

func compactNarrationSynthesisUnits(input []narrationSynthesisUnit, limit int) []narrationSynthesisUnit {
	if limit <= 0 || len(input) <= limit {
		return input
	}
	result := make([]narrationSynthesisUnit, 0, limit)
	for start := 0; start < len(input); {
		remainingUnits := len(input) - start
		remainingSlots := limit - len(result)
		groupSize := (remainingUnits + remainingSlots - 1) / remainingSlots
		end := minInt(start+groupSize, len(input))
		var text strings.Builder
		for index := start; index < end; index++ {
			text.WriteString(input[index].Text)
		}
		result = append(result, narrationSynthesisUnit{
			Text:              text.String(),
			PauseAfterMs:      input[end-1].PauseAfterMs,
			CaptionStartIndex: input[start].CaptionStartIndex,
			CaptionEndIndex:   input[end-1].CaptionEndIndex,
		})
		start = end
	}
	return result
}

func isClosingNarrationPunctuation(value rune) bool {
	switch value {
	case '”', '’', '）', ')', '】', ']', '》', '>', '」', '』':
		return true
	default:
		return false
	}
}

func synthesisContentRuneCount(text string) int {
	return len(normalizedNarrationRunes(text))
}

func nextSynthesisClauseRunes(captions []string, start int) int {
	count := 0
	for index := start; index < len(captions); index++ {
		count += synthesisContentRuneCount(captions[index])
		if count > 0 && synthesisBoundary(captions[index]) != synthesisBoundaryNone {
			break
		}
	}
	return count
}

func hasDependentClauseEnding(text string) bool {
	text = strings.TrimSpace(strings.TrimRightFunc(text, func(value rune) bool {
		return unicode.IsPunct(value) || unicode.IsSpace(value)
	}))
	for _, suffix := range []string{
		"如果", "虽然", "因为", "为了", "通过", "采用", "搭配", "支持", "让", "把", "将", "不仅", "除了", "无论", "只要",
	} {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

func synthesisPauseForBoundary(kind synthesisBoundaryKind) int {
	switch kind {
	case synthesisBoundarySentence:
		return sentenceSynthesisPauseMs
	case synthesisBoundarySemicolon:
		return semicolonSynthesisPauseMs
	case synthesisBoundaryColon:
		return colonSynthesisPauseMs
	case synthesisBoundaryComma:
		return commaSynthesisPauseMs
	default:
		return lengthSynthesisPauseMs
	}
}

func materializeSynthesizedNarrationUnits(
	planned []narrationSynthesisUnit,
	result []modelgateway.CosyVoiceSynthesisUnitResult,
	sampleRate int,
	durationMs int,
) ([]synthesizedNarrationUnit, error) {
	if len(planned) == 0 || len(result) != len(planned) || sampleRate <= 0 || durationMs <= 0 {
		return nil, fmt.Errorf("segmented synthesis timing is incomplete")
	}
	units := make([]synthesizedNarrationUnit, len(planned))
	cumulativeSamples := int64(0)
	for index := range planned {
		if result[index].SpeechSamples <= 0 || result[index].TotalSamples < result[index].SpeechSamples {
			return nil, fmt.Errorf("segmented synthesis unit %d timing is invalid", index+1)
		}
		startSamples := cumulativeSamples
		speechEndSamples := startSamples + int64(result[index].SpeechSamples)
		cumulativeSamples += int64(result[index].TotalSamples)
		units[index] = synthesizedNarrationUnit{
			Text:              planned[index].Text,
			CaptionStartIndex: planned[index].CaptionStartIndex,
			CaptionEndIndex:   planned[index].CaptionEndIndex,
			StartMs:           samplesToMilliseconds(startSamples, sampleRate),
			SpeechEndMs:       samplesToMilliseconds(speechEndSamples, sampleRate),
			EndMs:             samplesToMilliseconds(cumulativeSamples, sampleRate),
		}
	}
	if difference := units[len(units)-1].EndMs - durationMs; difference < -2 || difference > 2 {
		return nil, fmt.Errorf("segmented synthesis timing does not match WAV duration")
	}
	units[len(units)-1].EndMs = durationMs
	if units[len(units)-1].SpeechEndMs > durationMs {
		units[len(units)-1].SpeechEndMs = durationMs
	}
	return units, nil
}

func samplesToMilliseconds(samples int64, sampleRate int) int {
	return int((samples*1000 + int64(sampleRate)/2) / int64(sampleRate))
}

func normalizeNarrationSegmentsWithSynthesisUnits(
	input []modelgateway.ASRTranscriptSegment,
	script string,
	durationMs int,
	units []synthesizedNarrationUnit,
) ([]NarrationSegment, error) {
	targets := splitNarrationSentences(script)
	if len(targets) == 0 || len(units) == 0 {
		return nil, fmt.Errorf("narration synthesis units are required")
	}
	result := make([]NarrationSegment, 0, len(targets))
	expectedCaptionStart := 0
	expectedUnitStartMs := 0
	for unitIndex, unit := range units {
		if unit.CaptionStartIndex != expectedCaptionStart || unit.CaptionEndIndex <= unit.CaptionStartIndex || unit.CaptionEndIndex > len(targets) {
			return nil, fmt.Errorf("narration synthesis unit %d caption range is invalid", unitIndex+1)
		}
		if unit.StartMs != expectedUnitStartMs || unit.SpeechEndMs <= unit.StartMs || unit.EndMs < unit.SpeechEndMs || unit.EndMs > durationMs {
			return nil, fmt.Errorf("narration synthesis unit %d timeline is invalid", unitIndex+1)
		}
		unitTargets := targets[unit.CaptionStartIndex:unit.CaptionEndIndex]
		speechDurationMs := unit.SpeechEndMs - unit.StartMs
		bounds := alignNarrationSentenceBounds(unitTargets, transcriptForSynthesisUnit(input, unit.StartMs, unit.SpeechEndMs), speechDurationMs)
		for targetIndex, target := range unitTargets {
			startMs := unit.StartMs + bounds[targetIndex]
			endMs := unit.StartMs + bounds[targetIndex+1]
			if targetIndex == len(unitTargets)-1 {
				endMs = unit.EndMs
			}
			unitIndexCopy := unitIndex
			result = append(result, NarrationSegment{
				ID:                 uuid.NewString(),
				StartMs:            startMs,
				EndMs:              endMs,
				Text:               target,
				SynthesisUnitIndex: &unitIndexCopy,
			})
		}
		expectedCaptionStart = unit.CaptionEndIndex
		expectedUnitStartMs = unit.EndMs
	}
	if expectedCaptionStart != len(targets) || expectedUnitStartMs != durationMs {
		return nil, fmt.Errorf("narration synthesis units do not cover the approved script")
	}
	return result, nil
}

func transcriptForSynthesisUnit(input []modelgateway.ASRTranscriptSegment, startMs int, endMs int) []modelgateway.ASRTranscriptSegment {
	result := make([]modelgateway.ASRTranscriptSegment, 0)
	for _, segment := range input {
		if strings.TrimSpace(segment.Text) == "" || segment.EndMs <= segment.StartMs {
			continue
		}
		midpoint := segment.StartMs + (segment.EndMs-segment.StartMs)/2
		if midpoint < startMs || midpoint >= endMs {
			continue
		}
		segment.StartMs = max(segment.StartMs, startMs) - startMs
		segment.EndMs = minInt(segment.EndMs, endMs) - startMs
		if segment.EndMs > segment.StartMs {
			result = append(result, segment)
		}
	}
	return result
}
