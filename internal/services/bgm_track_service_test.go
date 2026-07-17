package services

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
)

func TestBGMTrackLifecycleAndResolution(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	service := NewBGMTrackService(root).WithProbe(func(context.Context, string) (ffmpeg.ProbeResult, error) {
		return ffmpeg.ProbeResult{HasAudio: true, DurationMs: 12_500, AudioSampleRate: 48_000, AudioChannels: 2}, nil
	})
	track, err := service.Create(context.Background(), BGMTrackUpload{
		BGMTrackInput: BGMTrackInput{Name: "轻快骑行", BPM: 118, Mood: "轻快", Tags: []string{"骑行", "轻快", "骑行"}},
		FileName:      "ride.mp3", MimeType: "audio/mpeg", Reader: bytes.NewBufferString("fake-audio"),
	}, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if track.DurationMs != 12_500 || track.SampleRate != 48_000 || len(track.Tags) != 2 || track.AudioURL == "" {
		t.Fatalf("unexpected track %#v", track)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(track.StorageKey))); err != nil {
		t.Fatalf("stored audio is missing: %v", err)
	}

	resolved, err := service.Resolve(context.Background(), BGMSelectionInput{Mode: "track", TrackID: track.ID}, nil)
	if err != nil || resolved == nil || resolved.GainDB != -12 {
		t.Fatalf("default Resolve() = %#v, %v", resolved, err)
	}
	zero := 0.0
	resolved, err = service.Resolve(context.Background(), BGMSelectionInput{Mode: "track", TrackID: track.ID, GainDB: &zero}, nil)
	if err != nil || resolved.GainDB != 0 {
		t.Fatalf("zero gain Resolve() = %#v, %v", resolved, err)
	}

	updated, err := service.Update(context.Background(), track.ID, BGMTrackInput{
		Name: "轻快骑行 2", BPM: 120, Mood: "活力", Tags: []string{"运动"}, Status: "disabled",
	}, "")
	if err != nil || updated.Status != "disabled" || updated.Name != "轻快骑行 2" {
		t.Fatalf("Update() = %#v, %v", updated, err)
	}
	if _, err := service.Resolve(context.Background(), BGMSelectionInput{Mode: "track", TrackID: track.ID}, nil); !errors.Is(err, ErrBGMTrackUnavailable) {
		t.Fatalf("disabled Resolve() error = %v", err)
	}
	archived, err := service.Archive(context.Background(), track.ID, "")
	if err != nil || archived.Status != "archived" {
		t.Fatalf("Archive() = %#v, %v", archived, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(track.StorageKey))); err != nil {
		t.Fatalf("archive removed historical audio: %v", err)
	}
}

func TestBGMRandomResolutionAvoidsUsedTrackWhenPossible(t *testing.T) {
	t.Parallel()
	service := NewBGMTrackService(t.TempDir()).WithProbe(func(context.Context, string) (ffmpeg.ProbeResult, error) {
		return ffmpeg.ProbeResult{HasAudio: true, DurationMs: 1_000}, nil
	})
	tracks := make([]BGMTrack, 0, 2)
	for _, name := range []string{"A", "B"} {
		track, err := service.Create(context.Background(), BGMTrackUpload{
			BGMTrackInput: BGMTrackInput{Name: name}, FileName: name + ".wav", Reader: bytes.NewBufferString(name),
		}, "")
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		tracks = append(tracks, track)
	}
	used := map[string]struct{}{tracks[0].ID: {}}
	resolved, err := service.Resolve(context.Background(), BGMSelectionInput{Mode: "random"}, used)
	if err != nil || resolved.TrackID != tracks[1].ID {
		t.Fatalf("Resolve(random) = %#v, %v", resolved, err)
	}
}
