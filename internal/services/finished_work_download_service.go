package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	maxFinishedWorkDownloadCount = 50
	finishedWorkDownloadTTL      = 10 * time.Minute
)

var (
	ErrFinishedWorkDownloadInvalid     = errors.New("invalid finished work download")
	ErrFinishedWorkDownloadUnavailable = errors.New("finished work download unavailable")
	ErrFinishedWorkDownloadToken       = errors.New("finished work download token not found")
)

type FinishedWorkDownloadBatch struct {
	Token     string    `json:"-"`
	FileName  string    `json:"file_name"`
	FileCount int       `json:"file_count"`
	ExpiresAt time.Time `json:"expires_at"`
}

type FinishedWorkDownloadFile struct {
	Path     string
	FileName string
	ModTime  time.Time
}

type storedFinishedWorkDownload struct {
	batch FinishedWorkDownloadBatch
	files []FinishedWorkDownloadFile
}

type FinishedWorkDownloadService struct {
	runs        *GenerationRunService
	storageRoot string
	now         func() time.Time

	mu      sync.Mutex
	batches map[string]storedFinishedWorkDownload
}

func NewFinishedWorkDownloadService(runs *GenerationRunService, storageRoot string) *FinishedWorkDownloadService {
	return &FinishedWorkDownloadService{
		runs: runs, storageRoot: storageRoot, now: time.Now,
		batches: map[string]storedFinishedWorkDownload{},
	}
}

func (s *FinishedWorkDownloadService) Create(ctx context.Context, workIDs []string) (FinishedWorkDownloadBatch, error) {
	if s == nil || s.runs == nil || strings.TrimSpace(s.storageRoot) == "" {
		return FinishedWorkDownloadBatch{}, ErrFinishedWorkDownloadUnavailable
	}
	uniqueIDs := make([]string, 0, len(workIDs))
	seenIDs := map[string]struct{}{}
	for _, workID := range workIDs {
		workID = normalizeID(workID)
		if workID == "" {
			continue
		}
		if _, exists := seenIDs[workID]; exists {
			continue
		}
		seenIDs[workID] = struct{}{}
		uniqueIDs = append(uniqueIDs, workID)
	}
	if len(uniqueIDs) == 0 || len(uniqueIDs) > maxFinishedWorkDownloadCount {
		return FinishedWorkDownloadBatch{}, fmt.Errorf("%w: select between 1 and %d works", ErrFinishedWorkDownloadInvalid, maxFinishedWorkDownloadCount)
	}

	files := make([]FinishedWorkDownloadFile, 0, len(uniqueIDs))
	usedNames := map[string]int{}
	for _, workID := range uniqueIDs {
		run, err := s.runs.Get(ctx, workID)
		if err != nil || run.Status != generationRunStatusCompleted || strings.TrimSpace(run.OutputStorageKey) == "" {
			return FinishedWorkDownloadBatch{}, fmt.Errorf("%w: work %s is not completed", ErrFinishedWorkDownloadUnavailable, workID)
		}
		work, err := s.runs.GetWork(ctx, workID)
		if err != nil {
			return FinishedWorkDownloadBatch{}, fmt.Errorf("%w: work %s metadata is unavailable", ErrFinishedWorkDownloadUnavailable, workID)
		}
		fullPath, err := safeStoragePath(s.storageRoot, run.OutputStorageKey)
		if err != nil {
			return FinishedWorkDownloadBatch{}, ErrFinishedWorkDownloadUnavailable
		}
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return FinishedWorkDownloadBatch{}, fmt.Errorf("%w: work %s file is unavailable", ErrFinishedWorkDownloadUnavailable, workID)
		}
		extension := strings.ToLower(filepath.Ext(run.OutputStorageKey))
		if extension == "" {
			extension = ".mp4"
		}
		baseName := fmt.Sprintf("%s_%s_%s_%s", sanitizeDownloadFilePart(work.ProductName), sanitizeDownloadFilePart(work.Title), run.CreatedAt.Format("20060102-1504"), shortDownloadID(run.ID))
		fileName := uniqueDownloadFileName(baseName+extension, usedNames)
		files = append(files, FinishedWorkDownloadFile{Path: fullPath, FileName: fileName, ModTime: info.ModTime()})
	}

	token, err := randomDownloadToken()
	if err != nil {
		return FinishedWorkDownloadBatch{}, err
	}
	now := s.now()
	batch := FinishedWorkDownloadBatch{
		Token: token, FileName: fmt.Sprintf("AICut_成品_%s.zip", now.Format("20060102-150405")),
		FileCount: len(files), ExpiresAt: now.Add(finishedWorkDownloadTTL),
	}
	s.mu.Lock()
	s.removeExpiredLocked(now)
	s.batches[token] = storedFinishedWorkDownload{batch: batch, files: files}
	s.mu.Unlock()
	return batch, nil
}

func (s *FinishedWorkDownloadService) Consume(token string) (FinishedWorkDownloadBatch, []FinishedWorkDownloadFile, error) {
	if s == nil {
		return FinishedWorkDownloadBatch{}, nil, ErrFinishedWorkDownloadToken
	}
	token = strings.TrimSpace(token)
	now := s.now()
	s.mu.Lock()
	s.removeExpiredLocked(now)
	stored, exists := s.batches[token]
	if exists {
		delete(s.batches, token)
	}
	s.mu.Unlock()
	if !exists {
		return FinishedWorkDownloadBatch{}, nil, ErrFinishedWorkDownloadToken
	}
	for _, file := range stored.files {
		info, err := os.Stat(file.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return FinishedWorkDownloadBatch{}, nil, ErrFinishedWorkDownloadUnavailable
		}
	}
	return stored.batch, append([]FinishedWorkDownloadFile(nil), stored.files...), nil
}

func (s *FinishedWorkDownloadService) removeExpiredLocked(now time.Time) {
	for token, stored := range s.batches {
		if !stored.batch.ExpiresAt.After(now) {
			delete(s.batches, token)
		}
	}
}

func randomDownloadToken() (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func sanitizeDownloadFilePart(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	count := 0
	for _, character := range value {
		if count >= 48 {
			break
		}
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\|?*`, character) {
			character = '_'
		}
		builder.WriteRune(character)
		count++
	}
	result := strings.Trim(builder.String(), " ._")
	if result == "" {
		return "成品"
	}
	return result
}

func shortDownloadID(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) > 8 {
		return value[:8]
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func uniqueDownloadFileName(fileName string, used map[string]int) string {
	extension := filepath.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, extension)
	for index := 1; ; index++ {
		candidate := fileName
		if index > 1 {
			candidate = baseName + fmt.Sprintf("_%d", index) + extension
		}
		key := strings.ToLower(candidate)
		if used[key] == 0 {
			used[key] = 1
			return candidate
		}
	}
}
