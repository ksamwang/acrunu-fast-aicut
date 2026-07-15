package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

type recordingVoiceSynthesizer struct {
	inputs []modelgateway.CosyVoiceSynthesisInput
	audio  []byte
}

func (s *recordingVoiceSynthesizer) Synthesize(_ context.Context, input modelgateway.CosyVoiceSynthesisInput) (modelgateway.CosyVoiceSynthesisResult, error) {
	s.inputs = append(s.inputs, input)
	return modelgateway.CosyVoiceSynthesisResult{Audio: s.audio, Model: "test-cosyvoice", SampleRate: 1000}, nil
}

type recordingVoiceTranscriber struct {
	inputs []modelgateway.FunASRTranscriptionInput
}

func (t *recordingVoiceTranscriber) Transcribe(_ context.Context, input modelgateway.FunASRTranscriptionInput) (modelgateway.ASRTranscriptionResult, error) {
	t.inputs = append(t.inputs, input)
	return modelgateway.ASRTranscriptionResult{
		Text: "第一句第二句",
		Segments: []modelgateway.ASRTranscriptSegment{
			{StartMs: 120, EndMs: 800, Text: "第一句"},
			{StartMs: 1000, EndMs: 1850, Text: "第二句"},
		},
	}, nil
}

func TestVoiceoverServiceGeneratesPreviewAuditionAndNarration(t *testing.T) {
	storageRoot := t.TempDir()
	synthesizer := &recordingVoiceSynthesizer{audio: testVoiceWAV(1000, 2_000)}
	transcriber := &recordingVoiceTranscriber{}
	service := NewVoiceoverService(storageRoot, config.Config{StorageRoot: storageRoot}, nil).WithClients(synthesizer, transcriber)

	profile, err := service.CreateVoiceProfile(context.Background(), VoiceProfileInput{
		Name:          "测试女声",
		Language:      "中文",
		StyleTags:     []string{"自然", "亲和"},
		ReferenceText: "这是参考文本。",
		PreviewText:   "这是固定样音。",
		Status:        "enabled",
		IsDefault:     true,
	}, VoiceReferenceAudio{
		Filename: "reference.wav",
		MimeType: "audio/wav",
		Size:     int64(len(testVoiceWAV(1000, 300))),
		Reader:   bytes.NewReader(testVoiceWAV(1000, 300)),
	}, "user-1")
	if err != nil {
		t.Fatalf("create voice profile: %v", err)
	}
	if profile.PreviewStatus != "queued" || !profile.IsDefault {
		t.Fatalf("unexpected newly created profile %#v", profile)
	}

	if err := service.ProcessVoiceProfilePreview(context.Background(), profile.ID); err != nil {
		t.Fatalf("generate profile preview: %v", err)
	}
	profile, err = service.GetVoiceProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("get voice profile: %v", err)
	}
	if profile.PreviewStatus != "ready" || profile.PreviewAudioURL == "" {
		t.Fatalf("expected ready preview, got %#v", profile)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, filepath.FromSlash(profile.PreviewAudioStorageKey))); err != nil {
		t.Fatalf("expected preview audio file: %v", err)
	}

	audition, err := service.CreateVoiceAudition(context.Background(), "audition-task", profile.ID, "user-1", "用当前文案试听。")
	if err != nil {
		t.Fatalf("create audition: %v", err)
	}
	if err := service.ProcessVoiceAudition(context.Background(), audition.ID); err != nil {
		t.Fatalf("process audition: %v", err)
	}
	audition, err = service.GetVoiceAudition(context.Background(), audition.ID)
	if err != nil {
		t.Fatalf("get audition: %v", err)
	}
	if audition.Status != "completed" || audition.DurationMs != 2000 || audition.AudioURL == "" {
		t.Fatalf("unexpected completed audition %#v", audition)
	}

	work, variantID, voiceoverID, err := service.CreateVoiceoverWork(context.Background(), CreateVoiceoverWorkInput{
		TaskID:         "voiceover-task",
		ProductID:      "product-1",
		ProductName:    "束裤带",
		VoiceProfileID: profile.ID,
		VariantIndex:   1,
		Variant: VoiceoverVariantInput{
			Hook:          "裤脚不蹭链条",
			ScriptText:    "第一句。第二句。",
			EditingIntent: "用使用场景说明产品价值。",
			Beats: []VoiceoverBeat{{
				ID:           "beat-1",
				Label:        "开头",
				SellingPoint: "不蹭链条",
				VisualGoal:   "展示裤脚固定效果。",
				SourceType:   "visual_only",
			}},
		},
	})
	if err != nil {
		t.Fatalf("create voiceover work: %v", err)
	}
	if work.Status != "generating" {
		t.Fatalf("unexpected initial work %#v", work)
	}
	if err := service.ProcessVoiceoverGenerate(context.Background(), queue.VoiceoverGeneratePayload{
		TaskID:          work.ID,
		ScriptVariantID: variantID,
		VoiceoverID:     voiceoverID,
	}); err != nil {
		t.Fatalf("process voiceover: %v", err)
	}
	work, err = service.GetVoiceoverWork(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("get voiceover work: %v", err)
	}
	if work.Status != "completed" || work.DurationMs != 2000 || work.AudioURL == "" {
		t.Fatalf("unexpected completed work %#v", work)
	}
	if len(work.NarrationSegments) != 2 {
		t.Fatalf("expected two narration segments, got %#v", work.NarrationSegments)
	}
	if first := work.NarrationSegments[0]; first.StartMs != 0 || first.EndMs != work.NarrationSegments[1].StartMs {
		t.Fatalf("expected continuous first segment, got %#v", work.NarrationSegments)
	}
	if last := work.NarrationSegments[1]; last.EndMs != work.DurationMs {
		t.Fatalf("expected final segment to end at duration, got %#v", work.NarrationSegments)
	}
	if len(synthesizer.inputs) != 3 {
		t.Fatalf("expected preview, audition, and voiceover synthesis calls, got %d", len(synthesizer.inputs))
	}
	if len(transcriber.inputs) != 1 || transcriber.inputs[0].DurationMs != 2000 {
		t.Fatalf("unexpected transcription calls %#v", transcriber.inputs)
	}
}

func TestVoiceoverServiceSetsDefaultWithoutRequeueingPreview(t *testing.T) {
	storageRoot := t.TempDir()
	service := NewVoiceoverService(storageRoot, config.Config{StorageRoot: storageRoot}, nil).WithClients(
		&recordingVoiceSynthesizer{audio: testVoiceWAV(1000, 800)},
		&recordingVoiceTranscriber{},
	)
	first := createTestVoiceProfile(t, service, "第一音色", true)
	second := createTestVoiceProfile(t, service, "第二音色", false)

	updated, err := service.SetDefaultVoiceProfile(context.Background(), second.ID, "user-1")
	if err != nil {
		t.Fatalf("set default voice profile: %v", err)
	}
	if !updated.IsDefault || updated.PreviewStatus != "queued" {
		t.Fatalf("unexpected updated profile %#v", updated)
	}
	first, err = service.GetVoiceProfile(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("get first profile: %v", err)
	}
	if first.IsDefault {
		t.Fatalf("expected first profile to no longer be default %#v", first)
	}
}

func createTestVoiceProfile(t *testing.T, service *VoiceoverService, name string, isDefault bool) VoiceProfile {
	t.Helper()
	profile, err := service.CreateVoiceProfile(context.Background(), VoiceProfileInput{
		Name:          name,
		Language:      "中文",
		StyleTags:     []string{"自然"},
		ReferenceText: "这是参考文本。",
		PreviewText:   "这是样音文本。",
		Status:        "enabled",
		IsDefault:     isDefault,
	}, VoiceReferenceAudio{
		Filename: "reference.wav",
		MimeType: "audio/wav",
		Size:     int64(len(testVoiceWAV(1000, 300))),
		Reader:   bytes.NewReader(testVoiceWAV(1000, 300)),
	}, "user-1")
	if err != nil {
		t.Fatalf("create voice profile: %v", err)
	}
	return profile
}

func TestNormalizeNarrationSegmentsFallsBackToOriginalScript(t *testing.T) {
	segments := normalizeNarrationSegments(nil, "原始口播文本", 1_500)
	if len(segments) != 1 || segments[0].StartMs != 0 || segments[0].EndMs != 1500 || segments[0].Text != "原始口播文本" {
		t.Fatalf("unexpected fallback narration segments %#v", segments)
	}
}

func testVoiceWAV(sampleRate int, durationMs int) []byte {
	byteRate := sampleRate * 2
	dataSize := byteRate * durationMs / 1000
	payload := make([]byte, 44+dataSize)
	copy(payload[0:4], "RIFF")
	binary.LittleEndian.PutUint32(payload[4:8], uint32(36+dataSize))
	copy(payload[8:12], "WAVE")
	copy(payload[12:16], "fmt ")
	binary.LittleEndian.PutUint32(payload[16:20], 16)
	binary.LittleEndian.PutUint16(payload[20:22], 1)
	binary.LittleEndian.PutUint16(payload[22:24], 1)
	binary.LittleEndian.PutUint32(payload[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(payload[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(payload[32:34], 2)
	binary.LittleEndian.PutUint16(payload[34:36], 16)
	copy(payload[36:40], "data")
	binary.LittleEndian.PutUint32(payload[40:44], uint32(dataSize))
	return payload
}

func TestWAVAudioMetadataRejectsInvalidPayload(t *testing.T) {
	if _, _, err := wavAudioMetadata([]byte("not-a-wav")); err == nil {
		t.Fatal("expected invalid wav error")
	}
	if _, _, err := wavAudioMetadata(testVoiceWAV(1000, 500)); err != nil {
		t.Fatalf("expected valid wav metadata: %v", err)
	}
}
