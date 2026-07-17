package services

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/storage"
)

const maxBGMFileBytes int64 = 100 << 20

var (
	ErrBGMTrackNotFound    = errors.New("bgm track not found")
	ErrBGMTrackInvalid     = errors.New("invalid bgm track")
	ErrBGMTrackUnavailable = errors.New("bgm track unavailable")
)

type BGMTrack struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	FileName        string    `json:"file_name"`
	StorageKey      string    `json:"-"`
	AudioURL        string    `json:"audio_url"`
	MimeType        string    `json:"mime_type"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	DurationMs      int       `json:"duration_ms"`
	SampleRate      int       `json:"sample_rate"`
	Channels        int       `json:"channels"`
	BPM             int       `json:"bpm,omitempty"`
	Mood            string    `json:"mood,omitempty"`
	Tags            []string  `json:"tags"`
	Status          string    `json:"status"`
	CreatedByUserID string    `json:"created_by_user_id,omitempty"`
	UpdatedByUserID string    `json:"updated_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type BGMTrackInput struct {
	Name   string   `json:"name"`
	BPM    int      `json:"bpm"`
	Mood   string   `json:"mood"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
}

type BGMTrackUpload struct {
	BGMTrackInput
	FileName string
	MimeType string
	Reader   io.Reader
}

type BGMSelectionInput struct {
	Mode    string   `json:"mode"`
	TrackID string   `json:"track_id,omitempty"`
	GainDB  *float64 `json:"gain_db,omitempty"`
}

type ResolvedBGMConfig struct {
	TrackID    string  `json:"track_id"`
	Name       string  `json:"name"`
	StorageKey string  `json:"storage_key"`
	GainDB     float64 `json:"gain_db"`
	FadeInMs   int     `json:"fade_in_ms"`
	FadeOutMs  int     `json:"fade_out_ms"`
}

type bgmProbeFunc func(context.Context, string) (ffmpeg.ProbeResult, error)

type BGMTrackService struct {
	pool  *pgxpool.Pool
	store *storage.LocalStore
	probe bgmProbeFunc

	mu     sync.RWMutex
	memory map[string]BGMTrack
}

func NewBGMTrackService(storageRoot string) *BGMTrackService {
	return newBGMTrackService(nil, storageRoot)
}

func NewBGMTrackServiceWithPool(pool *pgxpool.Pool, storageRoot string) *BGMTrackService {
	if pool == nil {
		return NewBGMTrackService(storageRoot)
	}
	return newBGMTrackService(pool, storageRoot)
}

func (s *BGMTrackService) WithProbe(probe func(context.Context, string) (ffmpeg.ProbeResult, error)) *BGMTrackService {
	if probe != nil {
		s.probe = probe
	}
	return s
}

func newBGMTrackService(pool *pgxpool.Pool, storageRoot string) *BGMTrackService {
	return &BGMTrackService{
		pool: pool, store: storage.NewLocalStore(storageRoot), probe: ffmpeg.Probe,
		memory: map[string]BGMTrack{},
	}
}

func (s *BGMTrackService) List(ctx context.Context, includeInactive bool) ([]BGMTrack, error) {
	if s.pool == nil {
		s.mu.RLock()
		items := make([]BGMTrack, 0, len(s.memory))
		for _, track := range s.memory {
			if includeInactive || track.Status == "enabled" {
				items = append(items, cloneBGMTrack(track))
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		return items, nil
	}
	query := bgmTrackSelect + " WHERE status = 'enabled' ORDER BY created_at DESC"
	if includeInactive {
		query = bgmTrackSelect + " ORDER BY created_at DESC"
	}
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BGMTrack{}
	for rows.Next() {
		track, scanErr := scanBGMTrack(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, track)
	}
	return items, rows.Err()
}

func (s *BGMTrackService) Get(ctx context.Context, trackID string) (BGMTrack, error) {
	trackID = normalizeID(trackID)
	if trackID == "" {
		return BGMTrack{}, ErrBGMTrackNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		track, ok := s.memory[trackID]
		s.mu.RUnlock()
		if !ok {
			return BGMTrack{}, ErrBGMTrackNotFound
		}
		return cloneBGMTrack(track), nil
	}
	track, err := scanBGMTrack(s.pool.QueryRow(ctx, bgmTrackSelect+" WHERE id = $1::uuid", trackID))
	if errors.Is(err, pgx.ErrNoRows) {
		return BGMTrack{}, ErrBGMTrackNotFound
	}
	return track, err
}

func (s *BGMTrackService) Create(ctx context.Context, upload BGMTrackUpload, userID string) (BGMTrack, error) {
	input, err := normalizeBGMTrackInput(upload.BGMTrackInput, false)
	if err != nil {
		return BGMTrack{}, err
	}
	if upload.Reader == nil || strings.TrimSpace(upload.FileName) == "" {
		return BGMTrack{}, fmt.Errorf("%w: audio file is required", ErrBGMTrackInvalid)
	}
	extension := strings.ToLower(filepath.Ext(upload.FileName))
	if !supportedBGMExtension(extension) {
		return BGMTrack{}, fmt.Errorf("%w: unsupported audio format", ErrBGMTrackInvalid)
	}
	trackID := uuid.NewString()
	storageKey := path.Join("bgm", trackID, "source"+extension)
	fullPath, err := s.store.Save(storageKey, io.LimitReader(upload.Reader, maxBGMFileBytes+1))
	if err != nil {
		return BGMTrack{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.store.Delete(storageKey)
		}
	}()
	info, err := os.Stat(fullPath)
	if err != nil {
		return BGMTrack{}, err
	}
	if info.Size() <= 0 || info.Size() > maxBGMFileBytes {
		return BGMTrack{}, fmt.Errorf("%w: audio file must be between 1 byte and 100 MiB", ErrBGMTrackInvalid)
	}
	probe, err := s.probe(ctx, fullPath)
	if err != nil || !probe.HasAudio || probe.DurationMs <= 0 {
		return BGMTrack{}, fmt.Errorf("%w: file does not contain a valid audio stream", ErrBGMTrackInvalid)
	}
	now := time.Now()
	track := BGMTrack{
		ID: trackID, Name: input.Name, FileName: filepath.Base(upload.FileName), StorageKey: storageKey,
		AudioURL: publicStorageURL(storageKey), MimeType: firstNonEmptyString(upload.MimeType, "application/octet-stream"),
		FileSizeBytes: info.Size(), DurationMs: probe.DurationMs, SampleRate: probe.AudioSampleRate,
		Channels: probe.AudioChannels, BPM: input.BPM, Mood: input.Mood, Tags: input.Tags, Status: input.Status,
		CreatedByUserID: normalizeID(userID), UpdatedByUserID: normalizeID(userID), CreatedAt: now, UpdatedAt: now,
	}
	if s.pool == nil {
		s.mu.Lock()
		s.memory[track.ID] = track
		s.mu.Unlock()
		committed = true
		return cloneBGMTrack(track), nil
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO bgm_tracks (
			id, name, file_name, storage_key, mime_type, file_size_bytes, duration_ms,
			sample_rate, channels, bpm, mood, tags, status, created_by_user_id, updated_by_user_id
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, 0), $11, $12, $13,
			NULLIF($14, '')::uuid, NULLIF($14, '')::uuid
		)`, track.ID, track.Name, track.FileName, track.StorageKey, track.MimeType, track.FileSizeBytes,
		track.DurationMs, track.SampleRate, track.Channels, track.BPM, track.Mood, track.Tags, track.Status, track.CreatedByUserID)
	if err != nil {
		return BGMTrack{}, err
	}
	committed = true
	return s.Get(ctx, track.ID)
}

func (s *BGMTrackService) Update(ctx context.Context, trackID string, input BGMTrackInput, userID string) (BGMTrack, error) {
	trackID = normalizeID(trackID)
	normalized, err := normalizeBGMTrackInput(input, true)
	if err != nil {
		return BGMTrack{}, err
	}
	if s.pool == nil {
		s.mu.Lock()
		track, ok := s.memory[trackID]
		if !ok {
			s.mu.Unlock()
			return BGMTrack{}, ErrBGMTrackNotFound
		}
		track.Name, track.BPM, track.Mood, track.Tags, track.Status = normalized.Name, normalized.BPM, normalized.Mood, normalized.Tags, normalized.Status
		track.UpdatedByUserID, track.UpdatedAt = normalizeID(userID), time.Now()
		s.memory[trackID] = track
		s.mu.Unlock()
		return cloneBGMTrack(track), nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE bgm_tracks SET name = $2, bpm = NULLIF($3, 0), mood = $4, tags = $5, status = $6,
			updated_by_user_id = NULLIF($7, '')::uuid, updated_at = now()
		WHERE id = $1::uuid`, trackID, normalized.Name, normalized.BPM, normalized.Mood, normalized.Tags, normalized.Status, normalizeID(userID))
	if err != nil {
		return BGMTrack{}, err
	}
	if command.RowsAffected() == 0 {
		return BGMTrack{}, ErrBGMTrackNotFound
	}
	return s.Get(ctx, trackID)
}

func (s *BGMTrackService) Archive(ctx context.Context, trackID string, userID string) (BGMTrack, error) {
	track, err := s.Get(ctx, trackID)
	if err != nil {
		return BGMTrack{}, err
	}
	return s.Update(ctx, trackID, BGMTrackInput{
		Name: track.Name, BPM: track.BPM, Mood: track.Mood, Tags: track.Tags, Status: "archived",
	}, userID)
}

func (s *BGMTrackService) Resolve(ctx context.Context, selection BGMSelectionInput, excluded map[string]struct{}) (*ResolvedBGMConfig, error) {
	mode := strings.TrimSpace(selection.Mode)
	if mode == "" || mode == "none" {
		return nil, nil
	}
	gainDB := -12.0
	if selection.GainDB != nil {
		gainDB = *selection.GainDB
	}
	if gainDB < -30 || gainDB > 0 {
		return nil, fmt.Errorf("%w: bgm gain must be between -30 and 0 dB", ErrBGMTrackInvalid)
	}
	var track BGMTrack
	var err error
	switch mode {
	case "track":
		track, err = s.Get(ctx, selection.TrackID)
	case "random":
		tracks, listErr := s.List(ctx, false)
		if listErr != nil {
			return nil, listErr
		}
		candidates := tracks
		if len(excluded) > 0 {
			unused := make([]BGMTrack, 0, len(tracks))
			for _, candidate := range tracks {
				if _, used := excluded[candidate.ID]; !used {
					unused = append(unused, candidate)
				}
			}
			if len(unused) > 0 {
				candidates = unused
			}
		}
		if len(candidates) == 0 {
			return nil, ErrBGMTrackUnavailable
		}
		index, randomErr := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(candidates))))
		if randomErr != nil {
			return nil, randomErr
		}
		track = candidates[index.Int64()]
	default:
		return nil, fmt.Errorf("%w: invalid bgm selection mode", ErrBGMTrackInvalid)
	}
	if err != nil {
		return nil, err
	}
	if track.Status != "enabled" {
		return nil, ErrBGMTrackUnavailable
	}
	return &ResolvedBGMConfig{
		TrackID: track.ID, Name: track.Name, StorageKey: track.StorageKey,
		GainDB: gainDB, FadeInMs: 300, FadeOutMs: 500,
	}, nil
}

func normalizeBGMTrackInput(input BGMTrackInput, allowArchived bool) (BGMTrackInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Mood = strings.TrimSpace(input.Mood)
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "enabled"
	}
	if input.Name == "" || len([]rune(input.Name)) > 120 || len([]rune(input.Mood)) > 40 {
		return BGMTrackInput{}, fmt.Errorf("%w: name or mood is invalid", ErrBGMTrackInvalid)
	}
	if input.BPM != 0 && (input.BPM < 20 || input.BPM > 300) {
		return BGMTrackInput{}, fmt.Errorf("%w: bpm must be between 20 and 300", ErrBGMTrackInvalid)
	}
	if input.Status != "enabled" && input.Status != "disabled" && (!allowArchived || input.Status != "archived") {
		return BGMTrackInput{}, fmt.Errorf("%w: invalid status", ErrBGMTrackInvalid)
	}
	input.Tags = normalizeBGMTrackTags(input.Tags)
	return input, nil
}

func normalizeBGMTrackTags(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > 30 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 20 {
			break
		}
	}
	return result
}

func supportedBGMExtension(extension string) bool {
	switch extension {
	case ".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg":
		return true
	default:
		return false
	}
}

func cloneBGMTrack(track BGMTrack) BGMTrack {
	track.Tags = append([]string(nil), track.Tags...)
	return track
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

const bgmTrackSelect = `
	SELECT id::text, name, file_name, storage_key, mime_type, file_size_bytes, duration_ms,
		sample_rate, channels, COALESCE(bpm, 0), mood, tags, status,
		COALESCE(created_by_user_id::text, ''), COALESCE(updated_by_user_id::text, ''), created_at, updated_at
	FROM bgm_tracks`

type bgmTrackScanner interface{ Scan(...any) error }

func scanBGMTrack(scanner bgmTrackScanner) (BGMTrack, error) {
	var track BGMTrack
	err := scanner.Scan(
		&track.ID, &track.Name, &track.FileName, &track.StorageKey, &track.MimeType, &track.FileSizeBytes,
		&track.DurationMs, &track.SampleRate, &track.Channels, &track.BPM, &track.Mood, &track.Tags, &track.Status,
		&track.CreatedByUserID, &track.UpdatedByUserID, &track.CreatedAt, &track.UpdatedAt,
	)
	if err == nil {
		track.AudioURL = publicStorageURL(track.StorageKey)
	}
	return track, err
}
