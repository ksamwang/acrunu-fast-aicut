package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
	"github.com/ksamwang/acrunu-fast-aicut/internal/storage"
)

const (
	maxVoiceReferenceAudioBytes = 20 << 20
	maxVoiceSynthesisTextRunes  = 4000
)

var (
	ErrVoiceProfileNotFound  = errors.New("voice profile not found")
	ErrVoiceProfileDisabled  = errors.New("voice profile is disabled")
	ErrVoiceProfileNotReady  = errors.New("voice profile preview is not ready")
	ErrVoiceProfileInUse     = errors.New("voice profile is referenced by generated work")
	ErrVoiceAuditionNotFound = errors.New("voice audition not found")
	ErrVoiceoverWorkNotFound = errors.New("voiceover work not found")
	ErrInvalidVoiceInput     = errors.New("invalid voice input")
)

type voiceSynthesizer interface {
	Synthesize(context.Context, modelgateway.CosyVoiceSynthesisInput) (modelgateway.CosyVoiceSynthesisResult, error)
}

type voiceTranscriber interface {
	Transcribe(context.Context, modelgateway.FunASRTranscriptionInput) (modelgateway.ASRTranscriptionResult, error)
}

type VoiceProfile struct {
	ID                       string    `json:"id"`
	Name                     string    `json:"name"`
	Language                 string    `json:"language"`
	StyleTags                []string  `json:"style_tags"`
	ReferenceText            string    `json:"reference_text"`
	ReferenceAudioStorageKey string    `json:"-"`
	ReferenceAudioName       string    `json:"reference_audio_name"`
	ReferenceAudioMimeType   string    `json:"-"`
	ReferenceAudioSize       int64     `json:"-"`
	PreviewText              string    `json:"preview_text"`
	PreviewAudioStorageKey   string    `json:"-"`
	PreviewAudioURL          string    `json:"preview_audio_url,omitempty"`
	PreviewStatus            string    `json:"preview_status"`
	PreviewError             string    `json:"preview_error,omitempty"`
	Status                   string    `json:"status"`
	IsDefault                bool      `json:"is_default"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type VoiceProfileInput struct {
	Name          string
	Language      string
	StyleTags     []string
	ReferenceText string
	PreviewText   string
	Status        string
	IsDefault     bool
}

type VoiceReferenceAudio struct {
	Filename string
	MimeType string
	Size     int64
	Reader   io.Reader
}

type VoiceoverBeat struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	SellingPoint string `json:"selling_point"`
	VisualGoal   string `json:"visual_goal"`
	SourceType   string `json:"source_type"`
}

type VoiceoverVariantInput struct {
	Hook          string          `json:"hook"`
	ScriptText    string          `json:"script_text"`
	EditingIntent string          `json:"editing_intent"`
	Beats         []VoiceoverBeat `json:"beats"`
}

type CreateVoiceoverWorkInput struct {
	TaskID         string
	ProductID      string
	ProductName    string
	VoiceProfileID string
	VariantIndex   int
	Variant        VoiceoverVariantInput
}

type NarrationSegment struct {
	ID         string  `json:"id"`
	StartMs    int     `json:"start_ms"`
	EndMs      int     `json:"end_ms"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
}

type VoiceoverWork struct {
	ID                string             `json:"id"`
	RunID             string             `json:"run_id"`
	ProductID         string             `json:"product_id"`
	ProductName       string             `json:"product_name"`
	Title             string             `json:"title"`
	Hook              string             `json:"hook"`
	VoiceProfileID    string             `json:"voice_profile_id"`
	VoiceProfileName  string             `json:"voice_profile_name"`
	ScriptText        string             `json:"script_text"`
	DurationMs        int                `json:"duration_ms"`
	Status            string             `json:"status"`
	Progress          int                `json:"progress"`
	StageLabel        string             `json:"stage_label"`
	ErrorMessage      string             `json:"error_message,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
	EditingIntent     string             `json:"editing_intent,omitempty"`
	Beats             []VoiceoverBeat    `json:"beats,omitempty"`
	NarrationSegments []NarrationSegment `json:"narration_segments,omitempty"`
	AudioURL          string             `json:"audio_url,omitempty"`
}

type VoiceAudition struct {
	ID               string    `json:"id"`
	TaskID           string    `json:"task_id"`
	VoiceProfileID   string    `json:"voice_profile_id"`
	VoiceProfileName string    `json:"voice_profile_name"`
	Text             string    `json:"text"`
	AudioStorageKey  string    `json:"-"`
	AudioURL         string    `json:"audio_url,omitempty"`
	SampleRate       int       `json:"sample_rate,omitempty"`
	DurationMs       int       `json:"duration_ms,omitempty"`
	Status           string    `json:"status"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	referenceAudioStorageKey string
	referenceAudioFileName   string
	referenceText            string
}

type memoryVoiceoverJob struct {
	work                     VoiceoverWork
	scriptVariantID          string
	voiceoverID              string
	referenceAudioStorageKey string
	referenceAudioFileName   string
	referenceText            string
	voiceoverStatus          string
}

type VoiceoverService struct {
	queries     *db.Queries
	pool        *pgxpool.Pool
	localStore  *storage.LocalStore
	synthesizer voiceSynthesizer
	transcriber voiceTranscriber
	logger      *slog.Logger
	synthesisMu sync.Mutex

	mu              sync.RWMutex
	memoryProfiles  map[string]VoiceProfile
	memoryWorks     map[string]*memoryVoiceoverJob
	memoryAuditions map[string]*VoiceAudition
}

func NewVoiceoverService(storageRoot string, cfg config.Config, logger *slog.Logger) *VoiceoverService {
	return newVoiceoverService(storageRoot, cfg, logger, nil, nil)
}

func NewVoiceoverServiceWithPool(storageRoot string, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) *VoiceoverService {
	if pool == nil {
		return NewVoiceoverService(storageRoot, cfg, logger)
	}
	return newVoiceoverService(storageRoot, cfg, logger, db.New(pool), pool)
}

func newVoiceoverService(storageRoot string, cfg config.Config, logger *slog.Logger, queries *db.Queries, pool *pgxpool.Pool) *VoiceoverService {
	if logger == nil {
		logger = slog.Default()
	}
	return &VoiceoverService{
		queries:         queries,
		pool:            pool,
		localStore:      storage.NewLocalStore(storageRoot),
		synthesizer:     modelgateway.NewCosyVoiceClient(cfg.TTSBaseURL, cfg.TTSRequestTimeout),
		transcriber:     modelgateway.NewFunASRClient(cfg.ASRBaseURL, cfg.ASRRequestTimeout),
		logger:          logger,
		memoryProfiles:  map[string]VoiceProfile{},
		memoryWorks:     map[string]*memoryVoiceoverJob{},
		memoryAuditions: map[string]*VoiceAudition{},
	}
}

func (s *VoiceoverService) WithClients(synthesizer voiceSynthesizer, transcriber voiceTranscriber) *VoiceoverService {
	if synthesizer != nil {
		s.synthesizer = synthesizer
	}
	if transcriber != nil {
		s.transcriber = transcriber
	}
	return s
}

func (s *VoiceoverService) CreateVoiceProfile(ctx context.Context, input VoiceProfileInput, reference VoiceReferenceAudio, userID string) (VoiceProfile, error) {
	input, err := normalizeVoiceProfileInput(input)
	if err != nil {
		return VoiceProfile{}, err
	}
	profileID := uuid.NewString()
	referenceSnapshot, err := s.saveVoiceReference(profileID, reference)
	if err != nil {
		return VoiceProfile{}, err
	}

	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		isDefault := input.IsDefault && input.Status == "enabled"
		if !isDefault && input.Status == "enabled" && !hasMemoryDefaultVoice(s.memoryProfiles) {
			isDefault = true
		}
		if isDefault {
			clearMemoryDefaultVoice(s.memoryProfiles)
		}
		now := time.Now()
		profile := VoiceProfile{
			ID:                       profileID,
			Name:                     input.Name,
			Language:                 input.Language,
			StyleTags:                append([]string(nil), input.StyleTags...),
			ReferenceText:            input.ReferenceText,
			ReferenceAudioStorageKey: referenceSnapshot.storageKey,
			ReferenceAudioName:       referenceSnapshot.fileName,
			ReferenceAudioMimeType:   referenceSnapshot.mimeType,
			ReferenceAudioSize:       referenceSnapshot.size,
			PreviewText:              input.PreviewText,
			PreviewStatus:            "queued",
			Status:                   input.Status,
			IsDefault:                isDefault,
			CreatedAt:                now,
			UpdatedAt:                now,
		}
		s.memoryProfiles[profile.ID] = profile
		return profile, nil
	}

	styles, err := json.Marshal(input.StyleTags)
	if err != nil {
		return VoiceProfile{}, err
	}
	profiles, err := s.queries.ListVoiceProfiles(ctx)
	if err != nil {
		return VoiceProfile{}, err
	}
	isDefault := input.IsDefault && input.Status == "enabled"
	if !isDefault && input.Status == "enabled" && !hasDatabaseDefaultVoice(profiles) {
		isDefault = true
	}

	var created db.VoiceProfile
	err = s.withVoiceTransaction(ctx, func(queries *db.Queries) error {
		if isDefault {
			if err := queries.ClearDefaultVoiceProfiles(ctx); err != nil {
				return err
			}
		}
		row, err := queries.CreateVoiceProfile(ctx, db.CreateVoiceProfileParams{
			ID:                       mustVoiceUUID(profileID),
			Name:                     input.Name,
			Language:                 input.Language,
			StyleTags:                styles,
			ReferenceText:            input.ReferenceText,
			ReferenceAudioStorageKey: referenceSnapshot.storageKey,
			ReferenceAudioFileName:   referenceSnapshot.fileName,
			ReferenceAudioMimeType:   referenceSnapshot.mimeType,
			ReferenceAudioSize:       referenceSnapshot.size,
			PreviewText:              input.PreviewText,
			Status:                   input.Status,
			IsDefault:                isDefault,
			CreatedByUserID:          nullableUUIDParam(userID),
			UpdatedByUserID:          nullableUUIDParam(userID),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return VoiceProfile{}, err
	}
	return voiceProfileFromDB(created), nil
}

func (s *VoiceoverService) UpdateVoiceProfile(ctx context.Context, profileID string, input VoiceProfileInput, reference *VoiceReferenceAudio, userID string) (VoiceProfile, error) {
	input, err := normalizeVoiceProfileInput(input)
	if err != nil {
		return VoiceProfile{}, err
	}
	existing, err := s.GetVoiceProfile(ctx, profileID)
	if err != nil {
		return VoiceProfile{}, err
	}
	snapshot := voiceReferenceSnapshot{
		storageKey: existing.ReferenceAudioStorageKey,
		fileName:   existing.ReferenceAudioName,
		mimeType:   existing.ReferenceAudioMimeType,
		size:       existing.ReferenceAudioSize,
	}
	if reference != nil {
		snapshot, err = s.saveVoiceReference(profileID, *reference)
		if err != nil {
			return VoiceProfile{}, err
		}
	}

	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		profile, ok := s.memoryProfiles[profileID]
		if !ok {
			return VoiceProfile{}, ErrVoiceProfileNotFound
		}
		isDefault := input.IsDefault && input.Status == "enabled"
		if isDefault {
			clearMemoryDefaultVoice(s.memoryProfiles)
		}
		profile.Name = input.Name
		profile.Language = input.Language
		profile.StyleTags = append([]string(nil), input.StyleTags...)
		profile.ReferenceText = input.ReferenceText
		profile.ReferenceAudioStorageKey = snapshot.storageKey
		profile.ReferenceAudioName = snapshot.fileName
		profile.ReferenceAudioMimeType = snapshot.mimeType
		profile.ReferenceAudioSize = snapshot.size
		profile.PreviewText = input.PreviewText
		profile.PreviewAudioStorageKey = ""
		profile.PreviewAudioURL = ""
		profile.PreviewStatus = "queued"
		profile.PreviewError = ""
		profile.Status = input.Status
		profile.IsDefault = isDefault
		profile.UpdatedAt = time.Now()
		s.memoryProfiles[profileID] = profile
		return cloneVoiceProfile(profile), nil
	}

	styles, err := json.Marshal(input.StyleTags)
	if err != nil {
		return VoiceProfile{}, err
	}
	id, err := uuidParam(profileID)
	if err != nil {
		return VoiceProfile{}, ErrVoiceProfileNotFound
	}
	isDefault := input.IsDefault && input.Status == "enabled"
	var updated db.VoiceProfile
	err = s.withVoiceTransaction(ctx, func(queries *db.Queries) error {
		if isDefault {
			if err := queries.ClearDefaultVoiceProfiles(ctx); err != nil {
				return err
			}
		}
		var updateErr error
		if reference == nil {
			updated, updateErr = queries.UpdateVoiceProfileMetadata(ctx, db.UpdateVoiceProfileMetadataParams{
				ID:              id,
				Name:            input.Name,
				Language:        input.Language,
				StyleTags:       styles,
				ReferenceText:   input.ReferenceText,
				PreviewText:     input.PreviewText,
				Status:          input.Status,
				IsDefault:       isDefault,
				UpdatedByUserID: nullableUUIDParam(userID),
			})
		} else {
			updated, updateErr = queries.UpdateVoiceProfileWithReference(ctx, db.UpdateVoiceProfileWithReferenceParams{
				ID:                       id,
				Name:                     input.Name,
				Language:                 input.Language,
				StyleTags:                styles,
				ReferenceText:            input.ReferenceText,
				ReferenceAudioStorageKey: snapshot.storageKey,
				ReferenceAudioFileName:   snapshot.fileName,
				ReferenceAudioMimeType:   snapshot.mimeType,
				ReferenceAudioSize:       snapshot.size,
				PreviewText:              input.PreviewText,
				Status:                   input.Status,
				IsDefault:                isDefault,
				UpdatedByUserID:          nullableUUIDParam(userID),
			})
		}
		if updateErr != nil {
			return updateErr
		}
		return queries.QueueVoiceProfilePreview(ctx, id)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VoiceProfile{}, ErrVoiceProfileNotFound
		}
		return VoiceProfile{}, err
	}
	profile := voiceProfileFromDB(updated)
	profile.PreviewAudioStorageKey = ""
	profile.PreviewAudioURL = ""
	profile.PreviewStatus = "queued"
	profile.PreviewError = ""
	return profile, nil
}

func (s *VoiceoverService) ListVoiceProfiles(ctx context.Context) ([]VoiceProfile, error) {
	if s.queries == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		profiles := make([]VoiceProfile, 0, len(s.memoryProfiles))
		for _, profile := range s.memoryProfiles {
			profiles = append(profiles, cloneVoiceProfile(profile))
		}
		sort.SliceStable(profiles, func(i, j int) bool {
			if profiles[i].IsDefault != profiles[j].IsDefault {
				return profiles[i].IsDefault
			}
			return profiles[i].CreatedAt.Before(profiles[j].CreatedAt)
		})
		return profiles, nil
	}

	rows, err := s.queries.ListVoiceProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]VoiceProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, voiceProfileFromDB(row))
	}
	return profiles, nil
}

func (s *VoiceoverService) GetVoiceProfile(ctx context.Context, profileID string) (VoiceProfile, error) {
	if s.queries == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		profile, ok := s.memoryProfiles[profileID]
		if !ok {
			return VoiceProfile{}, ErrVoiceProfileNotFound
		}
		return cloneVoiceProfile(profile), nil
	}

	id, err := uuidParam(profileID)
	if err != nil {
		return VoiceProfile{}, ErrVoiceProfileNotFound
	}
	row, err := s.queries.GetVoiceProfileByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return VoiceProfile{}, ErrVoiceProfileNotFound
	}
	if err != nil {
		return VoiceProfile{}, err
	}
	return voiceProfileFromDB(row), nil
}

func (s *VoiceoverService) DeleteVoiceProfile(ctx context.Context, profileID string) error {
	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.memoryProfiles[profileID]; !ok {
			return ErrVoiceProfileNotFound
		}
		for _, work := range s.memoryWorks {
			if work.work.VoiceProfileID == profileID {
				return ErrVoiceProfileInUse
			}
		}
		delete(s.memoryProfiles, profileID)
		return nil
	}

	id, err := uuidParam(profileID)
	if err != nil {
		return ErrVoiceProfileNotFound
	}
	if _, err := s.queries.GetVoiceProfileByID(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return ErrVoiceProfileNotFound
	} else if err != nil {
		return err
	}
	if err := s.queries.DeleteVoiceProfile(ctx, id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrVoiceProfileInUse
		}
		return err
	}
	return nil
}

func (s *VoiceoverService) SetDefaultVoiceProfile(ctx context.Context, profileID string, userID string) (VoiceProfile, error) {
	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		profile, ok := s.memoryProfiles[profileID]
		if !ok {
			return VoiceProfile{}, ErrVoiceProfileNotFound
		}
		if profile.Status != "enabled" {
			return VoiceProfile{}, ErrVoiceProfileDisabled
		}
		clearMemoryDefaultVoice(s.memoryProfiles)
		profile.IsDefault = true
		profile.UpdatedAt = time.Now()
		s.memoryProfiles[profileID] = profile
		return cloneVoiceProfile(profile), nil
	}

	id, err := uuidParam(profileID)
	if err != nil {
		return VoiceProfile{}, ErrVoiceProfileNotFound
	}
	var updated db.VoiceProfile
	err = s.withVoiceTransaction(ctx, func(queries *db.Queries) error {
		if err := queries.ClearDefaultVoiceProfiles(ctx); err != nil {
			return err
		}
		row, err := queries.SetDefaultVoiceProfile(ctx, db.SetDefaultVoiceProfileParams{
			ID:              id,
			UpdatedByUserID: nullableUUIDParam(userID),
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		profile, getErr := s.GetVoiceProfile(ctx, profileID)
		if getErr != nil {
			return VoiceProfile{}, getErr
		}
		if profile.Status != "enabled" {
			return VoiceProfile{}, ErrVoiceProfileDisabled
		}
		return VoiceProfile{}, ErrVoiceProfileNotFound
	}
	if err != nil {
		return VoiceProfile{}, err
	}
	return voiceProfileFromDB(updated), nil
}

func (s *VoiceoverService) CreateVoiceAudition(ctx context.Context, taskID string, profileID string, userID string, text string) (VoiceAudition, error) {
	text, err := normalizeVoiceText(text)
	if err != nil {
		return VoiceAudition{}, err
	}
	profile, err := s.requireReadyVoiceProfile(ctx, profileID)
	if err != nil {
		return VoiceAudition{}, err
	}

	if s.queries == nil {
		now := time.Now()
		audition := VoiceAudition{
			ID:                       uuid.NewString(),
			TaskID:                   taskID,
			VoiceProfileID:           profile.ID,
			VoiceProfileName:         profile.Name,
			Text:                     text,
			Status:                   "queued",
			CreatedAt:                now,
			UpdatedAt:                now,
			referenceAudioStorageKey: profile.ReferenceAudioStorageKey,
			referenceAudioFileName:   profile.ReferenceAudioName,
			referenceText:            profile.ReferenceText,
		}
		s.mu.Lock()
		s.memoryAuditions[audition.ID] = &audition
		s.mu.Unlock()
		return publicVoiceAudition(audition), nil
	}

	taskUUID, err := uuidParam(taskID)
	if err != nil {
		return VoiceAudition{}, fmt.Errorf("invalid task id")
	}
	profileUUID, err := uuidParam(profile.ID)
	if err != nil {
		return VoiceAudition{}, ErrVoiceProfileNotFound
	}
	row, err := s.queries.CreateVoiceAudition(ctx, db.CreateVoiceAuditionParams{
		GenerationTaskID:         taskUUID,
		VoiceProfileID:           profileUUID,
		VoiceProfileName:         profile.Name,
		ReferenceAudioStorageKey: profile.ReferenceAudioStorageKey,
		ReferenceAudioFileName:   profile.ReferenceAudioName,
		ReferenceText:            profile.ReferenceText,
		Text:                     text,
		CreatedByUserID:          nullableUUIDParam(userID),
	})
	if err != nil {
		return VoiceAudition{}, err
	}
	return voiceAuditionFromDB(row), nil
}

func (s *VoiceoverService) GetVoiceAudition(ctx context.Context, auditionID string) (VoiceAudition, error) {
	if s.queries == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		audition, ok := s.memoryAuditions[auditionID]
		if !ok {
			return VoiceAudition{}, ErrVoiceAuditionNotFound
		}
		return publicVoiceAudition(*audition), nil
	}

	id, err := uuidParam(auditionID)
	if err != nil {
		return VoiceAudition{}, ErrVoiceAuditionNotFound
	}
	row, err := s.queries.GetVoiceAuditionByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return VoiceAudition{}, ErrVoiceAuditionNotFound
	}
	if err != nil {
		return VoiceAudition{}, err
	}
	return voiceAuditionFromDB(row), nil
}

func (s *VoiceoverService) CreateVoiceoverWork(ctx context.Context, input CreateVoiceoverWorkInput) (VoiceoverWork, string, string, error) {
	if input.TaskID == "" || input.ProductID == "" || strings.TrimSpace(input.ProductName) == "" {
		return VoiceoverWork{}, "", "", errors.New("task and product are required")
	}
	scriptText, err := normalizeVoiceText(input.Variant.ScriptText)
	if err != nil {
		return VoiceoverWork{}, "", "", err
	}
	input.Variant.ScriptText = scriptText
	if input.VariantIndex <= 0 {
		input.VariantIndex = 1
	}
	profile, err := s.requireReadyVoiceProfile(ctx, input.VoiceProfileID)
	if err != nil {
		return VoiceoverWork{}, "", "", err
	}
	beats, err := json.Marshal(normalizeVoiceoverBeats(input.Variant.Beats))
	if err != nil {
		return VoiceoverWork{}, "", "", err
	}

	if s.queries == nil {
		now := time.Now()
		variantID := uuid.NewString()
		voiceoverID := uuid.NewString()
		job := &memoryVoiceoverJob{
			work: VoiceoverWork{
				ID:               input.TaskID,
				RunID:            input.TaskID,
				ProductID:        input.ProductID,
				ProductName:      input.ProductName,
				Title:            firstVoiceText(input.Variant.Hook, input.Variant.ScriptText),
				Hook:             input.Variant.Hook,
				VoiceProfileID:   profile.ID,
				VoiceProfileName: profile.Name,
				ScriptText:       input.Variant.ScriptText,
				Status:           "generating",
				Progress:         8,
				StageLabel:       "等待生成",
				CreatedAt:        now,
				EditingIntent:    input.Variant.EditingIntent,
				Beats:            normalizeVoiceoverBeats(input.Variant.Beats),
			},
			scriptVariantID:          variantID,
			voiceoverID:              voiceoverID,
			referenceAudioStorageKey: profile.ReferenceAudioStorageKey,
			referenceAudioFileName:   profile.ReferenceAudioName,
			referenceText:            profile.ReferenceText,
			voiceoverStatus:          "pending",
		}
		s.mu.Lock()
		s.memoryWorks[input.TaskID] = job
		s.mu.Unlock()
		return cloneVoiceoverWork(job.work), variantID, voiceoverID, nil
	}

	taskUUID, err := uuidParam(input.TaskID)
	if err != nil {
		return VoiceoverWork{}, "", "", errors.New("invalid task id")
	}
	productUUID, err := uuidParam(input.ProductID)
	if err != nil {
		return VoiceoverWork{}, "", "", errors.New("invalid product id")
	}
	profileUUID, err := uuidParam(profile.ID)
	if err != nil {
		return VoiceoverWork{}, "", "", ErrVoiceProfileNotFound
	}
	variant, err := s.queries.CreateScriptVariant(ctx, db.CreateScriptVariantParams{
		GenerationTaskID:         taskUUID,
		ProductID:                productUUID,
		VariantIndex:             int32(input.VariantIndex),
		Hook:                     strings.TrimSpace(input.Variant.Hook),
		ScriptText:               input.Variant.ScriptText,
		EditingIntent:            strings.TrimSpace(input.Variant.EditingIntent),
		Beats:                    beats,
		VoiceProfileID:           profileUUID,
		VoiceProfileName:         profile.Name,
		ReferenceAudioStorageKey: profile.ReferenceAudioStorageKey,
		ReferenceAudioFileName:   profile.ReferenceAudioName,
		ReferenceText:            profile.ReferenceText,
	})
	if err != nil {
		return VoiceoverWork{}, "", "", err
	}
	voiceover, err := s.queries.CreateVoiceover(ctx, db.CreateVoiceoverParams{
		ScriptVariantID: variant.ID,
		VoiceProvider:   "cosyvoice",
		VoiceModel:      "",
		VoiceName:       profile.Name,
	})
	if err != nil {
		return VoiceoverWork{}, "", "", err
	}
	work := voiceoverWorkFromRows(db.GenerationTask{
		ID:        taskUUID,
		ProductID: productUUID,
		Status:    "queued",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}, input.ProductName, variant, voiceover, nil)
	return work, uuidString(variant.ID), uuidString(voiceover.ID), nil
}

func (s *VoiceoverService) ListVoiceoverWorks(ctx context.Context) ([]VoiceoverWork, error) {
	if s.queries == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		works := make([]VoiceoverWork, 0, len(s.memoryWorks))
		for _, job := range s.memoryWorks {
			works = append(works, cloneVoiceoverWork(job.work))
		}
		sort.SliceStable(works, func(i, j int) bool { return works[i].CreatedAt.After(works[j].CreatedAt) })
		return works, nil
	}

	tasks, err := s.queries.ListVoiceoverGenerationTasks(ctx)
	if err != nil {
		return nil, err
	}
	works := make([]VoiceoverWork, 0, len(tasks))
	for _, task := range tasks {
		work, err := s.voiceoverWorkFromTask(ctx, task)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	return works, nil
}

func (s *VoiceoverService) GetVoiceoverWork(ctx context.Context, taskID string) (VoiceoverWork, error) {
	if s.queries == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		job, ok := s.memoryWorks[taskID]
		if !ok {
			return VoiceoverWork{}, ErrVoiceoverWorkNotFound
		}
		return cloneVoiceoverWork(job.work), nil
	}

	id, err := uuidParam(taskID)
	if err != nil {
		return VoiceoverWork{}, ErrVoiceoverWorkNotFound
	}
	task, err := s.queries.GetGenerationTaskByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && task.TaskType != "voiceover_generate") {
		return VoiceoverWork{}, ErrVoiceoverWorkNotFound
	}
	if err != nil {
		return VoiceoverWork{}, err
	}
	work, err := s.voiceoverWorkFromTask(ctx, task)
	if errors.Is(err, pgx.ErrNoRows) {
		return VoiceoverWork{}, ErrVoiceoverWorkNotFound
	}
	return work, err
}

func (s *VoiceoverService) ProcessVoiceProfilePreview(ctx context.Context, profileID string) error {
	profile, err := s.GetVoiceProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if profile.PreviewStatus == "ready" {
		return nil
	}
	if err := s.setVoiceProfilePreviewStatus(ctx, profileID, "processing", "", ""); err != nil {
		return err
	}
	storageKey := path.Join("voice-profiles", profileID, "preview.wav")
	_, _, err = s.synthesizeAndStore(ctx, profile.PreviewText, profile.ReferenceAudioStorageKey, profile.ReferenceAudioName, profile.ReferenceText, storageKey)
	if err != nil {
		_ = s.setVoiceProfilePreviewStatus(context.Background(), profileID, "failed", "", err.Error())
		return err
	}
	if err := s.setVoiceProfilePreviewStatus(ctx, profileID, "ready", storageKey, ""); err != nil {
		return err
	}
	return nil
}

func (s *VoiceoverService) MarkVoiceProfilePreviewFailed(ctx context.Context, profileID string, cause error) error {
	if cause == nil {
		return nil
	}
	return s.setVoiceProfilePreviewStatus(ctx, profileID, "failed", "", cause.Error())
}

func (s *VoiceoverService) ProcessVoiceAudition(ctx context.Context, auditionID string) error {
	audition, err := s.GetVoiceAudition(ctx, auditionID)
	if err != nil {
		return err
	}
	if audition.Status == "completed" {
		return nil
	}
	if err := s.setVoiceAuditionStatus(ctx, auditionID, "synthesizing", "", 0, 0, ""); err != nil {
		return err
	}
	storageKey := path.Join("voice-auditions", auditionID+".wav")
	sampleRate, durationMs, err := s.synthesizeAndStore(ctx, audition.Text, audition.referenceAudioStorageKey, audition.referenceAudioFileName, audition.referenceText, storageKey)
	if err != nil {
		_ = s.setVoiceAuditionStatus(context.Background(), auditionID, "failed", "", 0, 0, err.Error())
		return err
	}
	if err := s.setVoiceAuditionStatus(ctx, auditionID, "completed", storageKey, sampleRate, durationMs, ""); err != nil {
		return err
	}
	return nil
}

func (s *VoiceoverService) MarkVoiceAuditionFailed(ctx context.Context, auditionID string, cause error) error {
	if cause == nil {
		return nil
	}
	return s.setVoiceAuditionStatus(ctx, auditionID, "failed", "", 0, 0, cause.Error())
}

func (s *VoiceoverService) ProcessVoiceoverGenerate(ctx context.Context, payload queue.VoiceoverGeneratePayload) error {
	if payload.TaskID == "" || payload.ScriptVariantID == "" || payload.VoiceoverID == "" {
		return errors.New("voiceover payload is incomplete")
	}
	if s.queries == nil {
		return s.processMemoryVoiceover(ctx, payload)
	}

	variantID, err := uuidParam(payload.ScriptVariantID)
	if err != nil {
		return errors.New("invalid script variant id")
	}
	voiceoverID, err := uuidParam(payload.VoiceoverID)
	if err != nil {
		return errors.New("invalid voiceover id")
	}
	variant, err := s.queries.GetScriptVariantByID(ctx, variantID)
	if err != nil {
		return err
	}
	voiceover, err := s.queries.GetVoiceoverByID(ctx, voiceoverID)
	if err != nil {
		return err
	}
	if uuidString(variant.GenerationTaskID) != payload.TaskID || uuidString(voiceover.ScriptVariantID) != payload.ScriptVariantID {
		return errors.New("voiceover payload does not match persisted work")
	}
	if voiceover.Status == "completed" {
		return nil
	}
	if err := s.queries.MarkVoiceoverSynthesizing(ctx, voiceover.ID); err != nil {
		return err
	}

	storageKey := path.Join("voiceovers", payload.TaskID, payload.VoiceoverID+".wav")
	sampleRate, durationMs, err := s.synthesizeAndStore(ctx, variant.ScriptText, variant.ReferenceAudioStorageKey, variant.ReferenceAudioFileName, variant.ReferenceText, storageKey)
	if err != nil {
		s.failVoiceover(context.Background(), variant.ID, voiceover.ID, err)
		return err
	}
	if err := s.queries.MarkVoiceoverTranscribing(ctx, voiceover.ID); err != nil {
		s.failVoiceover(context.Background(), variant.ID, voiceover.ID, err)
		return err
	}

	audio, err := os.ReadFile(s.localStore.FullPath(storageKey))
	if err != nil {
		s.failVoiceover(context.Background(), variant.ID, voiceover.ID, err)
		return err
	}
	transcript, err := s.transcriber.Transcribe(ctx, modelgateway.FunASRTranscriptionInput{
		Filename:   "voiceover.wav",
		Audio:      bytes.NewReader(audio),
		DurationMs: durationMs,
	})
	if err != nil {
		s.failVoiceover(context.Background(), variant.ID, voiceover.ID, err)
		return err
	}
	segments := normalizeNarrationSegments(transcript.Segments, variant.ScriptText, durationMs)
	if err := s.persistNarrationSegments(ctx, variant.ID, voiceover.ID, segments); err != nil {
		s.failVoiceover(context.Background(), variant.ID, voiceover.ID, err)
		return err
	}
	if err := s.queries.MarkVoiceoverCompleted(ctx, db.MarkVoiceoverCompletedParams{
		ID:         voiceover.ID,
		StorageKey: pgtype.Text{String: storageKey, Valid: true},
		SampleRate: pgtype.Int4{Int32: int32(sampleRate), Valid: sampleRate > 0},
		DurationMs: pgtype.Int4{Int32: int32(durationMs), Valid: durationMs > 0},
	}); err != nil {
		return err
	}
	return s.queries.MarkScriptVariantVoiceoverReady(ctx, variant.ID)
}

func (s *VoiceoverService) processMemoryVoiceover(ctx context.Context, payload queue.VoiceoverGeneratePayload) error {
	s.mu.Lock()
	job, ok := s.memoryWorks[payload.TaskID]
	if !ok || job.scriptVariantID != payload.ScriptVariantID || job.voiceoverID != payload.VoiceoverID {
		s.mu.Unlock()
		return ErrVoiceoverWorkNotFound
	}
	if job.voiceoverStatus == "completed" {
		s.mu.Unlock()
		return nil
	}
	job.voiceoverStatus = "synthesizing"
	job.work.StageLabel = "生成旁白"
	job.work.Progress = 42
	s.mu.Unlock()

	storageKey := path.Join("voiceovers", payload.TaskID, payload.VoiceoverID+".wav")
	_, durationMs, err := s.synthesizeAndStore(ctx, job.work.ScriptText, job.referenceAudioStorageKey, job.referenceAudioFileName, job.referenceText, storageKey)
	if err != nil {
		s.failMemoryVoiceover(payload.TaskID, err)
		return err
	}
	s.mu.Lock()
	job.voiceoverStatus = "transcribing"
	job.work.StageLabel = "识别旁白"
	job.work.Progress = 72
	s.mu.Unlock()

	audio, err := os.ReadFile(s.localStore.FullPath(storageKey))
	if err != nil {
		s.failMemoryVoiceover(payload.TaskID, err)
		return err
	}
	transcript, err := s.transcriber.Transcribe(ctx, modelgateway.FunASRTranscriptionInput{
		Filename:   "voiceover.wav",
		Audio:      bytes.NewReader(audio),
		DurationMs: durationMs,
	})
	if err != nil {
		s.failMemoryVoiceover(payload.TaskID, err)
		return err
	}
	segments := normalizeNarrationSegments(transcript.Segments, job.work.ScriptText, durationMs)
	s.mu.Lock()
	job.voiceoverStatus = "completed"
	job.work.DurationMs = durationMs
	job.work.AudioURL = publicStorageURL(storageKey)
	job.work.NarrationSegments = segments
	job.work.Status = "completed"
	job.work.Progress = 100
	job.work.StageLabel = "已完成"
	now := time.Now()
	job.work.CompletedAt = &now
	s.mu.Unlock()
	return nil
}

func (s *VoiceoverService) synthesizeAndStore(ctx context.Context, text string, referenceStorageKey string, referenceFileName string, referenceText string, storageKey string) (int, int, error) {
	if s.synthesizer == nil {
		return 0, 0, errors.New("CosyVoice client is not configured")
	}
	referenceFile, err := os.Open(s.localStore.FullPath(referenceStorageKey))
	if err != nil {
		return 0, 0, fmt.Errorf("open voice reference audio: %w", err)
	}
	defer referenceFile.Close()

	s.synthesisMu.Lock()
	result, err := s.synthesizer.Synthesize(ctx, modelgateway.CosyVoiceSynthesisInput{
		Text:                text,
		PromptAudio:         referenceFile,
		PromptAudioFilename: referenceFileName,
		PromptText:          referenceText,
	})
	s.synthesisMu.Unlock()
	if err != nil {
		return 0, 0, err
	}
	sampleRate, durationMs, err := wavAudioMetadata(result.Audio)
	if err != nil {
		return 0, 0, err
	}
	if result.SampleRate > 0 {
		sampleRate = result.SampleRate
	}
	if _, err := s.localStore.Save(storageKey, bytes.NewReader(result.Audio)); err != nil {
		return 0, 0, fmt.Errorf("save synthesized audio: %w", err)
	}
	return sampleRate, durationMs, nil
}

func (s *VoiceoverService) persistNarrationSegments(ctx context.Context, scriptVariantID pgtype.UUID, voiceoverID pgtype.UUID, segments []NarrationSegment) error {
	if len(segments) == 0 {
		return errors.New("narration segments are required")
	}
	if err := s.queries.DeleteNarrationSegmentsByVoiceoverID(ctx, voiceoverID); err != nil {
		return err
	}
	for index, segment := range segments {
		if _, err := s.queries.CreateNarrationSegment(ctx, db.CreateNarrationSegmentParams{
			ScriptVariantID: scriptVariantID,
			VoiceoverID:     voiceoverID,
			SegmentIndex:    int32(index),
			Text:            segment.Text,
			StartMs:         int32(segment.StartMs),
			EndMs:           int32(segment.EndMs),
			Confidence:      pgtype.Numeric{},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *VoiceoverService) failVoiceover(ctx context.Context, scriptVariantID pgtype.UUID, voiceoverID pgtype.UUID, cause error) {
	if cause == nil {
		return
	}
	message := textParam(cause.Error())
	if err := s.queries.MarkVoiceoverFailed(ctx, db.MarkVoiceoverFailedParams{ID: voiceoverID, ErrorMessage: message}); err != nil {
		s.logger.Error("failed to persist voiceover failure", "error", err)
	}
	if err := s.queries.MarkScriptVariantFailed(ctx, db.MarkScriptVariantFailedParams{ID: scriptVariantID, ErrorMessage: message}); err != nil {
		s.logger.Error("failed to persist script variant failure", "error", err)
	}
}

func (s *VoiceoverService) failMemoryVoiceover(taskID string, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.memoryWorks[taskID]; ok {
		job.voiceoverStatus = "failed"
		job.work.Status = "failed"
		job.work.Progress = 100
		job.work.StageLabel = "生成失败"
		job.work.ErrorMessage = cause.Error()
	}
}

func (s *VoiceoverService) setVoiceProfilePreviewStatus(ctx context.Context, profileID string, status string, storageKey string, failure string) error {
	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		profile, ok := s.memoryProfiles[profileID]
		if !ok {
			return ErrVoiceProfileNotFound
		}
		profile.PreviewStatus = status
		profile.PreviewError = failure
		if storageKey != "" {
			profile.PreviewAudioStorageKey = storageKey
			profile.PreviewAudioURL = publicStorageURL(storageKey)
		}
		profile.UpdatedAt = time.Now()
		s.memoryProfiles[profileID] = profile
		return nil
	}

	id, err := uuidParam(profileID)
	if err != nil {
		return ErrVoiceProfileNotFound
	}
	switch status {
	case "processing":
		return s.queries.MarkVoiceProfilePreviewSynthesizing(ctx, id)
	case "ready":
		return s.queries.MarkVoiceProfilePreviewReady(ctx, db.MarkVoiceProfilePreviewReadyParams{
			ID:                     id,
			PreviewAudioStorageKey: pgtype.Text{String: storageKey, Valid: true},
		})
	case "failed":
		return s.queries.MarkVoiceProfilePreviewFailed(ctx, db.MarkVoiceProfilePreviewFailedParams{
			ID:           id,
			PreviewError: textParam(failure),
		})
	default:
		return s.queries.QueueVoiceProfilePreview(ctx, id)
	}
}

func (s *VoiceoverService) setVoiceAuditionStatus(ctx context.Context, auditionID string, status string, storageKey string, sampleRate int, durationMs int, failure string) error {
	if s.queries == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		audition, ok := s.memoryAuditions[auditionID]
		if !ok {
			return ErrVoiceAuditionNotFound
		}
		audition.Status = status
		audition.ErrorMessage = failure
		if storageKey != "" {
			audition.AudioStorageKey = storageKey
			audition.AudioURL = publicStorageURL(storageKey)
			audition.SampleRate = sampleRate
			audition.DurationMs = durationMs
		}
		audition.UpdatedAt = time.Now()
		return nil
	}

	id, err := uuidParam(auditionID)
	if err != nil {
		return ErrVoiceAuditionNotFound
	}
	switch status {
	case "synthesizing":
		return s.queries.MarkVoiceAuditionSynthesizing(ctx, id)
	case "completed":
		return s.queries.MarkVoiceAuditionCompleted(ctx, db.MarkVoiceAuditionCompletedParams{
			ID:              id,
			AudioStorageKey: pgtype.Text{String: storageKey, Valid: true},
			SampleRate:      pgtype.Int4{Int32: int32(sampleRate), Valid: sampleRate > 0},
			DurationMs:      pgtype.Int4{Int32: int32(durationMs), Valid: durationMs > 0},
		})
	case "failed":
		return s.queries.MarkVoiceAuditionFailed(ctx, db.MarkVoiceAuditionFailedParams{ID: id, ErrorMessage: textParam(failure)})
	default:
		return fmt.Errorf("unsupported voice audition status %q", status)
	}
}

func (s *VoiceoverService) voiceoverWorkFromTask(ctx context.Context, task db.GenerationTask) (VoiceoverWork, error) {
	variant, err := s.queries.GetScriptVariantByGenerationTaskID(ctx, task.ID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	voiceover, err := s.queries.GetVoiceoverByScriptVariantID(ctx, variant.ID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	product, err := s.queries.GetProductByID(ctx, variant.ProductID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	segments, err := s.queries.ListNarrationSegmentsByVoiceoverID(ctx, voiceover.ID)
	if err != nil {
		return VoiceoverWork{}, err
	}
	return voiceoverWorkFromRows(task, product.Name, variant, voiceover, segments), nil
}

func voiceoverWorkFromRows(task db.GenerationTask, productName string, variant db.ScriptVariant, voiceover db.Voiceover, narrationRows []db.NarrationSegment) VoiceoverWork {
	beats := []VoiceoverBeat{}
	_ = json.Unmarshal(variant.Beats, &beats)
	narrationSegments := make([]NarrationSegment, 0, len(narrationRows))
	for _, row := range narrationRows {
		narrationSegments = append(narrationSegments, NarrationSegment{
			ID:      uuidString(row.ID),
			StartMs: int(row.StartMs),
			EndMs:   int(row.EndMs),
			Text:    row.Text,
		})
	}
	status, progress, stageLabel := voiceoverWorkStage(task.Status, voiceover.Status)
	if task.Status == "failed" || voiceover.Status == "failed" || variant.Status == "failed" {
		status, progress, stageLabel = "failed", 100, "生成失败"
	}
	errorMessage := firstVoiceText(task.ErrorMessage.String, voiceover.ErrorMessage.String, variant.ErrorMessage.String)
	work := VoiceoverWork{
		ID:                uuidString(task.ID),
		RunID:             uuidString(task.ID),
		ProductID:         uuidString(variant.ProductID),
		ProductName:       productName,
		Title:             firstVoiceText(variant.Hook, variant.ScriptText),
		Hook:              variant.Hook,
		VoiceProfileID:    uuidString(variant.VoiceProfileID),
		VoiceProfileName:  variant.VoiceProfileName,
		ScriptText:        variant.ScriptText,
		DurationMs:        int(voiceover.DurationMs.Int32),
		Status:            status,
		Progress:          progress,
		StageLabel:        stageLabel,
		ErrorMessage:      errorMessage,
		CreatedAt:         timeFromTimestamptz(task.CreatedAt),
		EditingIntent:     variant.EditingIntent,
		Beats:             beats,
		NarrationSegments: narrationSegments,
	}
	if task.FinishedAt.Valid && task.Status == "completed" {
		completedAt := task.FinishedAt.Time
		work.CompletedAt = &completedAt
	}
	if voiceover.StorageKey.Valid {
		work.AudioURL = publicStorageURL(voiceover.StorageKey.String)
	}
	return work
}

func voiceoverWorkStage(taskStatus string, voiceoverStatus string) (string, int, string) {
	switch taskStatus {
	case "completed":
		return "completed", 100, "已完成"
	case "failed":
		return "failed", 100, "生成失败"
	case "queued":
		return "generating", 8, "等待生成"
	}
	switch voiceoverStatus {
	case "transcribing":
		return "generating", 72, "识别旁白"
	case "completed":
		return "generating", 90, "完成处理中"
	case "failed":
		return "failed", 100, "生成失败"
	default:
		return "generating", 42, "生成旁白"
	}
}

func (s *VoiceoverService) requireReadyVoiceProfile(ctx context.Context, profileID string) (VoiceProfile, error) {
	profile, err := s.GetVoiceProfile(ctx, profileID)
	if err != nil {
		return VoiceProfile{}, err
	}
	if profile.Status != "enabled" {
		return VoiceProfile{}, ErrVoiceProfileDisabled
	}
	if profile.PreviewStatus != "ready" || profile.ReferenceAudioStorageKey == "" {
		return VoiceProfile{}, ErrVoiceProfileNotReady
	}
	return profile, nil
}

type voiceReferenceSnapshot struct {
	storageKey string
	fileName   string
	mimeType   string
	size       int64
}

func (s *VoiceoverService) saveVoiceReference(profileID string, reference VoiceReferenceAudio) (voiceReferenceSnapshot, error) {
	if reference.Reader == nil || reference.Size <= 0 || reference.Size > maxVoiceReferenceAudioBytes {
		return voiceReferenceSnapshot{}, fmt.Errorf("%w: reference audio must be between 1 byte and %d MiB", ErrInvalidVoiceInput, maxVoiceReferenceAudioBytes>>20)
	}
	ext := strings.ToLower(filepath.Ext(reference.Filename))
	if !isSupportedVoiceReferenceExtension(ext) {
		return voiceReferenceSnapshot{}, fmt.Errorf("%w: unsupported reference audio format", ErrInvalidVoiceInput)
	}
	mimeType := strings.TrimSpace(reference.MimeType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	storageKey := path.Join("voice-profiles", profileID, "reference"+ext)
	if _, err := s.localStore.Save(storageKey, io.LimitReader(reference.Reader, maxVoiceReferenceAudioBytes+1)); err != nil {
		return voiceReferenceSnapshot{}, fmt.Errorf("save reference audio: %w", err)
	}
	if info, err := os.Stat(s.localStore.FullPath(storageKey)); err != nil {
		return voiceReferenceSnapshot{}, fmt.Errorf("stat reference audio: %w", err)
	} else if info.Size() <= 0 || info.Size() > maxVoiceReferenceAudioBytes {
		return voiceReferenceSnapshot{}, fmt.Errorf("%w: reference audio exceeds the allowed size", ErrInvalidVoiceInput)
	}
	return voiceReferenceSnapshot{
		storageKey: storageKey,
		fileName:   safeVoiceFilename(reference.Filename),
		mimeType:   mimeType,
		size:       reference.Size,
	}, nil
}

func (s *VoiceoverService) withVoiceTransaction(ctx context.Context, run func(*db.Queries) error) error {
	if s.pool == nil {
		return run(s.queries)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := run(s.queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func normalizeVoiceProfileInput(input VoiceProfileInput) (VoiceProfileInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Language = strings.TrimSpace(input.Language)
	input.ReferenceText = strings.TrimSpace(input.ReferenceText)
	input.PreviewText = strings.TrimSpace(input.PreviewText)
	input.StyleTags = normalizeStringSlice(input.StyleTags)
	if input.Name == "" || input.Language == "" || input.ReferenceText == "" || input.PreviewText == "" {
		return VoiceProfileInput{}, fmt.Errorf("%w: name, language, reference text, and preview text are required", ErrInvalidVoiceInput)
	}
	if _, err := normalizeVoiceText(input.PreviewText); err != nil {
		return VoiceProfileInput{}, fmt.Errorf("invalid preview text: %w", err)
	}
	if input.Status != "disabled" {
		input.Status = "enabled"
	}
	return input, nil
}

func normalizeVoiceText(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: voice text is required", ErrInvalidVoiceInput)
	}
	if utf8.RuneCountInString(value) > maxVoiceSynthesisTextRunes {
		return "", fmt.Errorf("%w: voice text must not exceed %d characters", ErrInvalidVoiceInput, maxVoiceSynthesisTextRunes)
	}
	return value, nil
}

func normalizeVoiceoverBeats(input []VoiceoverBeat) []VoiceoverBeat {
	beats := make([]VoiceoverBeat, 0, len(input))
	for _, beat := range input {
		beat.Label = strings.TrimSpace(beat.Label)
		beat.SellingPoint = strings.TrimSpace(beat.SellingPoint)
		beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
		if beat.Label == "" && beat.SellingPoint == "" && beat.VisualGoal == "" {
			continue
		}
		beats = append(beats, beat)
	}
	return beats
}

func normalizeStringSlice(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isSupportedVoiceReferenceExtension(ext string) bool {
	switch ext {
	case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".webm":
		return true
	default:
		return false
	}
}

func safeVoiceFilename(value string) string {
	value = filepath.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if value == "" || value == "." || value == ".." {
		return "reference.wav"
	}
	return value
}

func hasMemoryDefaultVoice(profiles map[string]VoiceProfile) bool {
	for _, profile := range profiles {
		if profile.IsDefault && profile.Status == "enabled" {
			return true
		}
	}
	return false
}

func clearMemoryDefaultVoice(profiles map[string]VoiceProfile) {
	for id, profile := range profiles {
		if profile.IsDefault {
			profile.IsDefault = false
			profile.UpdatedAt = time.Now()
			profiles[id] = profile
		}
	}
}

func hasDatabaseDefaultVoice(profiles []db.VoiceProfile) bool {
	for _, profile := range profiles {
		if profile.IsDefault && profile.Status == "enabled" {
			return true
		}
	}
	return false
}

func voiceProfileFromDB(row db.VoiceProfile) VoiceProfile {
	styles := []string{}
	_ = json.Unmarshal(row.StyleTags, &styles)
	previewKey := ""
	if row.PreviewAudioStorageKey.Valid {
		previewKey = row.PreviewAudioStorageKey.String
	}
	profile := VoiceProfile{
		ID:                       uuidString(row.ID),
		Name:                     row.Name,
		Language:                 row.Language,
		StyleTags:                styles,
		ReferenceText:            row.ReferenceText,
		ReferenceAudioStorageKey: row.ReferenceAudioStorageKey,
		ReferenceAudioName:       row.ReferenceAudioFileName,
		ReferenceAudioMimeType:   row.ReferenceAudioMimeType,
		ReferenceAudioSize:       row.ReferenceAudioSize,
		PreviewText:              row.PreviewText,
		PreviewAudioStorageKey:   previewKey,
		PreviewStatus:            row.PreviewStatus,
		PreviewError:             row.PreviewError.String,
		Status:                   row.Status,
		IsDefault:                row.IsDefault,
		CreatedAt:                timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:                timeFromTimestamptz(row.UpdatedAt),
	}
	if previewKey != "" {
		profile.PreviewAudioURL = publicStorageURL(previewKey)
	}
	return profile
}

func voiceAuditionFromDB(row db.VoiceAudition) VoiceAudition {
	audition := VoiceAudition{
		ID:                       uuidString(row.ID),
		TaskID:                   uuidString(row.GenerationTaskID),
		VoiceProfileID:           uuidString(row.VoiceProfileID),
		VoiceProfileName:         row.VoiceProfileName,
		Text:                     row.Text,
		AudioStorageKey:          row.AudioStorageKey.String,
		SampleRate:               int(row.SampleRate.Int32),
		DurationMs:               int(row.DurationMs.Int32),
		Status:                   row.Status,
		ErrorMessage:             row.ErrorMessage.String,
		CreatedAt:                timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:                timeFromTimestamptz(row.UpdatedAt),
		referenceAudioStorageKey: row.ReferenceAudioStorageKey,
		referenceAudioFileName:   row.ReferenceAudioFileName,
		referenceText:            row.ReferenceText,
	}
	if audition.AudioStorageKey != "" {
		audition.AudioURL = publicStorageURL(audition.AudioStorageKey)
	}
	return audition
}

func publicVoiceAudition(audition VoiceAudition) VoiceAudition {
	audition.referenceAudioStorageKey = ""
	audition.referenceAudioFileName = ""
	audition.referenceText = ""
	return audition
}

func cloneVoiceProfile(profile VoiceProfile) VoiceProfile {
	profile.StyleTags = append([]string(nil), profile.StyleTags...)
	return profile
}

func cloneVoiceoverWork(work VoiceoverWork) VoiceoverWork {
	work.Beats = append([]VoiceoverBeat(nil), work.Beats...)
	work.NarrationSegments = append([]NarrationSegment(nil), work.NarrationSegments...)
	if work.CompletedAt != nil {
		completedAt := *work.CompletedAt
		work.CompletedAt = &completedAt
	}
	return work
}

func normalizeNarrationSegments(input []modelgateway.ASRTranscriptSegment, fallbackText string, durationMs int) []NarrationSegment {
	if durationMs <= 0 {
		return nil
	}
	segments := make([]modelgateway.ASRTranscriptSegment, 0, len(input))
	for _, segment := range input {
		segment.Text = strings.TrimSpace(segment.Text)
		if segment.Text == "" || segment.EndMs <= segment.StartMs {
			continue
		}
		if segment.StartMs < 0 {
			segment.StartMs = 0
		}
		if segment.EndMs > durationMs {
			segment.EndMs = durationMs
		}
		if segment.EndMs > segment.StartMs {
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		text := strings.TrimSpace(fallbackText)
		if text == "" {
			text = "旁白"
		}
		return []NarrationSegment{{ID: uuid.NewString(), StartMs: 0, EndMs: durationMs, Text: text}}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].StartMs < segments[j].StartMs })
	if len(segments) > durationMs {
		segments = segments[:durationMs]
	}
	bounds := make([]int, len(segments)+1)
	bounds[0] = 0
	for index := 1; index < len(segments); index++ {
		boundary := (segments[index-1].EndMs + segments[index].StartMs) / 2
		minimum := bounds[index-1] + 1
		maximum := durationMs - (len(segments) - index)
		if boundary < minimum {
			boundary = minimum
		}
		if boundary > maximum {
			boundary = maximum
		}
		bounds[index] = boundary
	}
	bounds[len(segments)] = durationMs
	result := make([]NarrationSegment, 0, len(segments))
	for index, segment := range segments {
		if bounds[index+1] <= bounds[index] {
			continue
		}
		result = append(result, NarrationSegment{
			ID:      uuid.NewString(),
			StartMs: bounds[index],
			EndMs:   bounds[index+1],
			Text:    segment.Text,
		})
	}
	if len(result) == 0 {
		return []NarrationSegment{{ID: uuid.NewString(), StartMs: 0, EndMs: durationMs, Text: strings.TrimSpace(fallbackText)}}
	}
	return result
}

func wavAudioMetadata(audio []byte) (int, int, error) {
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return 0, 0, errors.New("CosyVoice did not return a RIFF/WAVE payload")
	}
	var sampleRate int
	var byteRate int
	var dataSize int64
	for offset := 12; offset+8 <= len(audio); {
		chunkID := string(audio[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
		offset += 8
		if chunkSize < 0 || offset+chunkSize > len(audio) {
			return 0, 0, errors.New("invalid WAV chunk size")
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return 0, 0, errors.New("invalid WAV format chunk")
			}
			sampleRate = int(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
			byteRate = int(binary.LittleEndian.Uint32(audio[offset+8 : offset+12]))
		case "data":
			dataSize += int64(chunkSize)
		}
		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	if sampleRate <= 0 || byteRate <= 0 || dataSize <= 0 {
		return 0, 0, errors.New("WAV sample rate or audio data is missing")
	}
	durationMs := int((dataSize*1000 + int64(byteRate)/2) / int64(byteRate))
	if durationMs <= 0 {
		return 0, 0, errors.New("WAV duration is invalid")
	}
	return sampleRate, durationMs, nil
}

func publicStorageURL(storageKey string) string {
	storageKey = strings.TrimPrefix(path.Clean("/"+storageKey), "/")
	if storageKey == "" || storageKey == "." {
		return ""
	}
	return "/storage/" + storageKey
}

func firstVoiceText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mustVoiceUUID(value string) pgtype.UUID {
	id, _ := uuidParam(value)
	return id
}
