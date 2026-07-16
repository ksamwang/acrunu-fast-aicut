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
	if work.NarrationSegments[0].Text != "第一句。" || work.NarrationSegments[1].Text != "第二句。" {
		t.Fatalf("expected narration text to come from the approved script, got %#v", work.NarrationSegments)
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

func TestNormalizeNarrationSegmentsUsesApprovedScriptSentenceBoundaries(t *testing.T) {
	script := "骑车时裤脚总被链条蹭脏，又难洗又尴尬？今天给大家推荐这款束裤带，它采用防蹭链条设计，高弹力材质，可自由调节，魔术贴一粘即合，牢固不脱落。夜间反光条，让后车在黑暗中也能清晰看到你，安全升级。再也不用担心裤脚问题，骑行更轻松。快来试试吧！"
	segments := normalizeNarrationSegments([]modelgateway.ASRTranscriptSegment{
		{StartMs: 0, EndMs: 970, Text: "骑车时"},
		{StartMs: 970, EndMs: 3060, Text: "裤脚总被链条蹭脏"},
		{StartMs: 3060, EndMs: 4590, Text: "又难洗又尴尬"},
		{StartMs: 4590, EndMs: 14280, Text: "今天给大家推荐这款束裤带它采用防蹭链条设计高弹力材质可自由调节魔术贴一粘即合"},
		{StartMs: 14280, EndMs: 15600, Text: "牢固不脱落"},
		{StartMs: 15600, EndMs: 22140, Text: "夜间反光条让后车在黑暗中也能清晰看到你裤夹安全升级"},
		{StartMs: 22140, EndMs: 25920, Text: "再也不用担心裤脚问题骑行更轻松"},
		{StartMs: 25920, EndMs: 27320, Text: "快来试试吧"},
	}, script, 27_320)
	wantTexts := []string{
		"骑车时裤脚总被链条蹭脏，又难洗又尴尬？",
		"今天给大家推荐这款束裤带，它采用防蹭链条设计，高弹力材质，可自由调节，魔术贴一粘即合，牢固不脱落。",
		"夜间反光条，让后车在黑暗中也能清晰看到你，安全升级。",
		"再也不用担心裤脚问题，骑行更轻松。",
		"快来试试吧！",
	}
	if len(segments) != len(wantTexts) {
		t.Fatalf("expected %d script sentences, got %#v", len(wantTexts), segments)
	}
	for index, want := range wantTexts {
		if segments[index].Text != want {
			t.Fatalf("segment %d text = %q, want %q", index+1, segments[index].Text, want)
		}
		if index > 0 && segments[index].StartMs != segments[index-1].EndMs {
			t.Fatalf("segments are not continuous %#v", segments)
		}
	}
	if segments[0].StartMs != 0 || segments[len(segments)-1].EndMs != 27_320 {
		t.Fatalf("unexpected narration bounds %#v", segments)
	}
}

func TestNormalizeNarrationSegmentsFallsBackToProportionalTimelineOnLowAlignment(t *testing.T) {
	segments := normalizeNarrationSegments([]modelgateway.ASRTranscriptSegment{{
		StartMs: 0,
		EndMs:   900,
		Text:    "完全不匹配的识别结果",
	}}, "甲。乙乙。", 900)
	if len(segments) != 2 {
		t.Fatalf("expected two script sentences, got %#v", segments)
	}
	if segments[0].StartMs != 0 || segments[0].EndMs != 300 || segments[1].StartMs != 300 || segments[1].EndMs != 900 {
		t.Fatalf("expected proportional fallback bounds, got %#v", segments)
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
