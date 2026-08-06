package services

import (
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func TestCleanSynthesizedNarrationTrimsTrailingHallucination(t *testing.T) {
	sampleRate := 1000
	audio := testVoiceWAV(sampleRate, 1500)
	planned := []narrationSynthesisUnit{{Text: "它能一键拆卸，", PauseAfterMs: 120}}
	results := []modelgateway.CosyVoiceSynthesisUnitResult{{SpeechSamples: 1380, TotalSamples: 1500}}
	transcript := modelgateway.ASRTranscriptionResult{Tokens: timedTestTokens(
		[]rune("它能一键拆卸它直逼"),
		[]int{0, 120, 240, 360, 480, 600, 720, 840, 980, 1110},
	)}

	cleaned, cleanedResults, changed, err := cleanSynthesizedNarration(audio, planned, results, sampleRate, transcript)
	if err != nil {
		t.Fatalf("clean narration: %v", err)
	}
	if !changed {
		t.Fatal("expected trailing speech to be trimmed")
	}
	if cleanedResults[0].SpeechSamples != 740 || cleanedResults[0].TotalSamples != 860 {
		t.Fatalf("unexpected cleaned timing %#v", cleanedResults)
	}
	_, durationMs, err := wavAudioMetadata(cleaned)
	if err != nil || durationMs != 860 {
		t.Fatalf("unexpected cleaned WAV duration %d: %v", durationMs, err)
	}
}

func TestTrailingNarrationCutAllowsSameLengthRecognitionDifference(t *testing.T) {
	tokens := timedTestTokens([]rune("它能一键拆线"), []int{0, 100, 200, 300, 400, 500})
	if _, changed, err := trailingNarrationCut("它能一键拆卸", tokens); err != nil || changed {
		t.Fatalf("same-length recognition difference must not be trimmed: changed=%v err=%v", changed, err)
	}
}

func TestTrailingNarrationCutRejectsInternalSpeech(t *testing.T) {
	tokens := timedTestTokens([]rune("它能胡言一键拆卸"), []int{0, 100, 200, 300, 400, 500, 600, 700})
	if _, _, err := trailingNarrationCut("它能一键拆卸", tokens); err == nil {
		t.Fatal("expected internal unexpected speech to fail validation")
	}
}

func TestTrailingNarrationCutRejectsMissingSpeech(t *testing.T) {
	tokens := timedTestTokens([]rune("它能"), []int{0, 100})
	if _, _, err := trailingNarrationCut("它能一键拆卸", tokens); err == nil {
		t.Fatal("expected missing narration to fail validation")
	}
}

func timedTestTokens(values []rune, starts []int) []modelgateway.ASRTranscriptToken {
	result := make([]modelgateway.ASRTranscriptToken, len(values))
	for index, value := range values {
		result[index] = modelgateway.ASRTranscriptToken{Text: string(value), StartMs: starts[index], EndMs: starts[index] + 120}
	}
	return result
}
