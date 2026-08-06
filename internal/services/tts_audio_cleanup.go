package services

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	voiceoverTailKeepMs = 20
	voiceoverFadeOutMs  = 8
)

type pcm16WAV struct {
	SampleRate int
	Channels   int
	Samples    []int16
}

func cleanSynthesizedNarration(
	audio []byte,
	planned []narrationSynthesisUnit,
	unitResults []modelgateway.CosyVoiceSynthesisUnitResult,
	sampleRate int,
	transcript modelgateway.ASRTranscriptionResult,
) ([]byte, []modelgateway.CosyVoiceSynthesisUnitResult, bool, error) {
	if len(planned) == 0 || len(planned) != len(unitResults) {
		return nil, nil, false, errors.New("narration cleanup timing is incomplete")
	}
	wav, err := parsePCM16WAV(audio)
	if err != nil {
		return nil, nil, false, err
	}
	if wav.SampleRate != sampleRate || wav.Channels != 1 {
		return nil, nil, false, errors.New("narration cleanup requires mono PCM16 WAV audio")
	}

	cutSamples := make([]int, len(planned))
	cumulative := 0
	changed := false
	for index, result := range unitResults {
		if result.SpeechSamples <= 0 || result.TotalSamples < result.SpeechSamples || cumulative+result.TotalSamples > len(wav.Samples) {
			return nil, nil, false, fmt.Errorf("narration synthesis unit %d sample range is invalid", index+1)
		}
		startSample := cumulative
		speechEndSample := startSample + result.SpeechSamples
		startMs := samplesToMilliseconds(int64(startSample), sampleRate)
		speechEndMs := samplesToMilliseconds(int64(speechEndSample), sampleRate)
		unitTokens := transcriptTokensInRange(transcript.Tokens, startMs, speechEndMs)
		cutMs, shouldCut, validateErr := trailingNarrationCut(planned[index].Text, unitTokens)
		if validateErr != nil {
			return nil, nil, false, fmt.Errorf("narration synthesis unit %d: %w", index+1, validateErr)
		}
		cutSamples[index] = result.SpeechSamples
		if shouldCut {
			absoluteCut := millisecondsToSamples(cutMs+voiceoverTailKeepMs, sampleRate)
			if absoluteCut <= startSample || absoluteCut >= speechEndSample {
				return nil, nil, false, fmt.Errorf("narration synthesis unit %d cutoff is unreliable", index+1)
			}
			cutSamples[index] = absoluteCut - startSample
			changed = true
		}
		cumulative += result.TotalSamples
	}
	if cumulative != len(wav.Samples) {
		return nil, nil, false, errors.New("narration synthesis timing does not match WAV samples")
	}
	if !changed {
		return audio, append([]modelgateway.CosyVoiceSynthesisUnitResult(nil), unitResults...), false, nil
	}

	newResults := make([]modelgateway.CosyVoiceSynthesisUnitResult, len(planned))
	output := make([]int16, 0, len(wav.Samples))
	cumulative = 0
	for index, result := range unitResults {
		startSample := cumulative
		speech := append([]int16(nil), wav.Samples[startSample:startSample+cutSamples[index]]...)
		if cutSamples[index] < result.SpeechSamples {
			applyPCM16FadeOut(speech, millisecondsToSamples(voiceoverFadeOutMs, sampleRate))
		}
		output = append(output, speech...)
		pauseSamples := millisecondsToSamples(planned[index].PauseAfterMs, sampleRate)
		output = append(output, make([]int16, pauseSamples)...)
		newResults[index] = modelgateway.CosyVoiceSynthesisUnitResult{
			SpeechSamples: len(speech),
			TotalSamples:  len(speech) + pauseSamples,
		}
		cumulative += result.TotalSamples
	}
	encoded, err := encodePCM16WAV(pcm16WAV{SampleRate: sampleRate, Channels: 1, Samples: output})
	if err != nil {
		return nil, nil, false, err
	}
	return encoded, newResults, true, nil
}

func transcriptTokensInRange(input []modelgateway.ASRTranscriptToken, startMs int, endMs int) []modelgateway.ASRTranscriptToken {
	result := make([]modelgateway.ASRTranscriptToken, 0)
	for _, token := range input {
		midpoint := token.StartMs + (token.EndMs-token.StartMs)/2
		if midpoint >= startMs && midpoint < endMs && token.EndMs > token.StartMs {
			result = append(result, token)
		}
	}
	return result
}

func trailingNarrationCut(expectedText string, tokens []modelgateway.ASRTranscriptToken) (int, bool, error) {
	expected := normalizedNarrationRunes(expectedText)
	sourceRunes := make([]rune, 0, len(tokens))
	sourceTokens := make([]modelgateway.ASRTranscriptToken, 0, len(tokens))
	for _, token := range tokens {
		for _, value := range []rune(token.Text) {
			if unicode.IsSpace(value) || unicode.IsPunct(value) {
				continue
			}
			sourceRunes = append(sourceRunes, value)
			sourceTokens = append(sourceTokens, token)
		}
	}
	if len(expected) == 0 || len(sourceRunes) == 0 || len(sourceRunes) < len(expected)*2/3 {
		return 0, false, errors.New("expected narration is missing")
	}
	if len(sourceRunes) <= len(expected) {
		if runeSimilarity(expected, sourceRunes) < 0.65 {
			return 0, false, errors.New("recognized narration does not match expected text")
		}
		return 0, false, nil
	}

	prefix := sourceRunes[:len(expected)]
	if runeSimilarity(expected, prefix) < 0.75 {
		return 0, false, errors.New("unexpected speech appears inside the narration")
	}
	lastExpected := sourceTokens[len(expected)-1]
	firstExtra := sourceTokens[len(expected)]
	if firstExtra.StartMs < lastExpected.StartMs || lastExpected.EndMs <= 0 {
		return 0, false, errors.New("narration cutoff is unreliable")
	}
	return lastExpected.EndMs, true, nil
}

func runeSimilarity(left []rune, right []rune) float64 {
	maximum := max(len(left), len(right))
	if maximum == 0 {
		return 1
	}
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftValue := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightValue := range right {
			cost := 1
			if leftValue == rightValue {
				cost = 0
			}
			current[rightIndex+1] = minInt(
				current[rightIndex]+1,
				minInt(previous[rightIndex+1]+1, previous[rightIndex]+cost),
			)
		}
		previous = current
	}
	return 1 - float64(previous[len(right)])/float64(maximum)
}

func parsePCM16WAV(audio []byte) (pcm16WAV, error) {
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return pcm16WAV{}, errors.New("CosyVoice did not return a RIFF/WAVE payload")
	}
	format, channels, sampleRate, bitsPerSample := 0, 0, 0, 0
	var data []byte
	for offset := 12; offset+8 <= len(audio); {
		chunkID := string(audio[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
		offset += 8
		if chunkSize < 0 || offset+chunkSize > len(audio) {
			return pcm16WAV{}, errors.New("invalid WAV chunk size")
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return pcm16WAV{}, errors.New("invalid WAV format chunk")
			}
			format = int(binary.LittleEndian.Uint16(audio[offset : offset+2]))
			channels = int(binary.LittleEndian.Uint16(audio[offset+2 : offset+4]))
			sampleRate = int(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(audio[offset+14 : offset+16]))
		case "data":
			data = append(data, audio[offset:offset+chunkSize]...)
		}
		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	if format != 1 || channels <= 0 || sampleRate <= 0 || bitsPerSample != 16 || len(data) == 0 || len(data)%2 != 0 {
		return pcm16WAV{}, errors.New("narration cleanup requires PCM16 WAV audio")
	}
	samples := make([]int16, len(data)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(data[index*2 : index*2+2]))
	}
	return pcm16WAV{SampleRate: sampleRate, Channels: channels, Samples: samples}, nil
}

func encodePCM16WAV(input pcm16WAV) ([]byte, error) {
	if input.SampleRate <= 0 || input.Channels <= 0 || len(input.Samples) == 0 {
		return nil, errors.New("PCM16 WAV input is invalid")
	}
	dataSize := len(input.Samples) * 2
	buffer := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buffer.WriteString("RIFF")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(36+dataSize))
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(input.Channels))
	_ = binary.Write(buffer, binary.LittleEndian, uint32(input.SampleRate))
	byteRate := input.SampleRate * input.Channels * 2
	_ = binary.Write(buffer, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(input.Channels*2))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(dataSize))
	for _, sample := range input.Samples {
		_ = binary.Write(buffer, binary.LittleEndian, sample)
	}
	return buffer.Bytes(), nil
}

func applyPCM16FadeOut(samples []int16, fadeSamples int) {
	if fadeSamples <= 0 || len(samples) == 0 {
		return
	}
	if fadeSamples > len(samples) {
		fadeSamples = len(samples)
	}
	start := len(samples) - fadeSamples
	for index := 0; index < fadeSamples; index++ {
		remaining := fadeSamples - index - 1
		samples[start+index] = int16(int(samples[start+index]) * remaining / fadeSamples)
	}
}

func millisecondsToSamples(durationMs int, sampleRate int) int {
	if durationMs <= 0 || sampleRate <= 0 {
		return 0
	}
	return (durationMs*sampleRate + 500) / 1000
}
