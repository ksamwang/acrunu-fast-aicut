package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrUploadTokenInvalid = errors.New("upload token invalid")

type UploadToken struct {
	Token     string    `json:"token"`
	ProductID string    `json:"product_id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type uploadTokenRecord struct {
	Hash      string
	ProductID string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type UploadTokenService struct {
	mu     sync.Mutex
	tokens map[string]uploadTokenRecord
}

func NewUploadTokenService() *UploadTokenService {
	return &UploadTokenService{tokens: map[string]uploadTokenRecord{}}
}

func (s *UploadTokenService) Create(productID string, userID string, ttl time.Duration) (UploadToken, error) {
	token, err := randomToken()
	if err != nil {
		return UploadToken{}, err
	}

	record := uploadTokenRecord{
		Hash:      hashToken(token),
		ProductID: productID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[record.Hash] = record

	return UploadToken{
		Token:     token,
		ProductID: productID,
		UserID:    userID,
		ExpiresAt: record.ExpiresAt,
	}, nil
}

func (s *UploadTokenService) Consume(token string) (UploadToken, error) {
	hash := hashToken(token)

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.tokens[hash]
	if !ok || record.UsedAt != nil || time.Now().After(record.ExpiresAt) {
		return UploadToken{}, ErrUploadTokenInvalid
	}

	now := time.Now()
	record.UsedAt = &now
	s.tokens[hash] = record

	return UploadToken{
		Token:     token,
		ProductID: record.ProductID,
		UserID:    record.UserID,
		ExpiresAt: record.ExpiresAt,
	}, nil
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
