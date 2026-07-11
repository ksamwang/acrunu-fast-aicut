package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ModelProviderTypeOpenAICompatible = "openai_compatible"

var ErrModelProviderNotFound = errors.New("model provider not found")

type ModelProvider struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	ProviderType     string         `json:"provider_type"`
	BaseURL          string         `json:"base_url"`
	APIKeyConfigured bool           `json:"api_key_configured"`
	Enabled          bool           `json:"enabled"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type ModelProviderInput struct {
	Name         string         `json:"name"`
	ProviderType string         `json:"provider_type"`
	BaseURL      string         `json:"base_url"`
	APIKey       string         `json:"api_key"`
	Enabled      bool           `json:"enabled"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type ModelProviderAccess struct {
	ID           string
	Name         string
	ProviderType string
	BaseURL      string
	APIKey       string
	Enabled      bool
}

type ModelProviderService struct {
	mu        sync.RWMutex
	providers map[string]ModelProviderAccess
	pool      *pgxpool.Pool
}

func NewModelProviderService() *ModelProviderService {
	return &ModelProviderService{providers: map[string]ModelProviderAccess{}}
}

func NewModelProviderServiceWithPool(pool *pgxpool.Pool) *ModelProviderService {
	return &ModelProviderService{
		providers: map[string]ModelProviderAccess{},
		pool:      pool,
	}
}

func (s *ModelProviderService) List(ctx context.Context) ([]ModelProvider, error) {
	if s == nil {
		return nil, fmt.Errorf("model provider service is nil")
	}
	if s.pool != nil {
		rows, err := s.pool.Query(ctx, `
			SELECT id::text, name, provider_type, base_url, api_key <> '', enabled, metadata, created_at, updated_at
			FROM model_providers
			ORDER BY created_at DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		providers := []ModelProvider{}
		for rows.Next() {
			provider, err := scanModelProviderPublic(rows)
			if err != nil {
				return nil, err
			}
			providers = append(providers, provider)
		}
		return providers, rows.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	providers := make([]ModelProvider, 0, len(s.providers))
	for _, access := range s.providers {
		providers = append(providers, providerPublicFromAccess(access))
	}
	return providers, nil
}

func (s *ModelProviderService) Get(ctx context.Context, id string) (ModelProvider, error) {
	access, err := s.GetAccess(ctx, id)
	if err != nil {
		return ModelProvider{}, err
	}
	return providerPublicFromAccess(access), nil
}

func (s *ModelProviderService) GetAccess(ctx context.Context, id string) (ModelProviderAccess, error) {
	if s == nil {
		return ModelProviderAccess{}, fmt.Errorf("model provider service is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ModelProviderAccess{}, ErrModelProviderNotFound
	}
	if s.pool != nil {
		var access ModelProviderAccess
		err := s.pool.QueryRow(ctx, `
			SELECT id::text, name, provider_type, base_url, api_key, enabled
			FROM model_providers
			WHERE id = $1`, id).Scan(
			&access.ID,
			&access.Name,
			&access.ProviderType,
			&access.BaseURL,
			&access.APIKey,
			&access.Enabled,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ModelProviderAccess{}, ErrModelProviderNotFound
		}
		if err != nil {
			return ModelProviderAccess{}, err
		}
		return access, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	access, ok := s.providers[id]
	if !ok {
		return ModelProviderAccess{}, ErrModelProviderNotFound
	}
	return access, nil
}

func (s *ModelProviderService) Create(ctx context.Context, input ModelProviderInput) (ModelProvider, error) {
	if s == nil {
		return ModelProvider{}, fmt.Errorf("model provider service is nil")
	}
	normalized, err := normalizeModelProviderInput(input, true)
	if err != nil {
		return ModelProvider{}, err
	}
	if s.pool != nil {
		metadata, err := json.Marshal(normalized.Metadata)
		if err != nil {
			return ModelProvider{}, err
		}
		row := s.pool.QueryRow(ctx, `
			INSERT INTO model_providers (name, provider_type, base_url, api_key, enabled, metadata)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id::text, name, provider_type, base_url, api_key <> '', enabled, metadata, created_at, updated_at`,
			normalized.Name,
			normalized.ProviderType,
			normalized.BaseURL,
			strings.TrimSpace(input.APIKey),
			normalized.Enabled,
			metadata,
		)
		return scanModelProviderPublic(row)
	}

	now := time.Now()
	access := ModelProviderAccess{
		ID:           uuid.NewString(),
		Name:         normalized.Name,
		ProviderType: normalized.ProviderType,
		BaseURL:      normalized.BaseURL,
		APIKey:       strings.TrimSpace(input.APIKey),
		Enabled:      normalized.Enabled,
	}
	s.mu.Lock()
	s.providers[access.ID] = access
	s.mu.Unlock()
	return ModelProvider{
		ID:               access.ID,
		Name:             access.Name,
		ProviderType:     access.ProviderType,
		BaseURL:          access.BaseURL,
		APIKeyConfigured: access.APIKey != "",
		Enabled:          access.Enabled,
		Metadata:         normalized.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *ModelProviderService) Update(ctx context.Context, id string, input ModelProviderInput) (ModelProvider, error) {
	if s == nil {
		return ModelProvider{}, fmt.Errorf("model provider service is nil")
	}
	id = strings.TrimSpace(id)
	normalized, err := normalizeModelProviderInput(input, false)
	if err != nil {
		return ModelProvider{}, err
	}
	if s.pool != nil {
		metadata, err := json.Marshal(normalized.Metadata)
		if err != nil {
			return ModelProvider{}, err
		}
		row := s.pool.QueryRow(ctx, `
			UPDATE model_providers
			SET name = $2,
			    provider_type = $3,
			    base_url = $4,
			    api_key = CASE WHEN $5 = '' THEN api_key ELSE $5 END,
			    enabled = $6,
			    metadata = $7,
			    updated_at = now()
			WHERE id = $1
			RETURNING id::text, name, provider_type, base_url, api_key <> '', enabled, metadata, created_at, updated_at`,
			id,
			normalized.Name,
			normalized.ProviderType,
			normalized.BaseURL,
			strings.TrimSpace(input.APIKey),
			normalized.Enabled,
			metadata,
		)
		provider, err := scanModelProviderPublic(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return ModelProvider{}, ErrModelProviderNotFound
		}
		return provider, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.providers[id]
	if !ok {
		return ModelProvider{}, ErrModelProviderNotFound
	}
	current.Name = normalized.Name
	current.ProviderType = normalized.ProviderType
	current.BaseURL = normalized.BaseURL
	if strings.TrimSpace(input.APIKey) != "" {
		current.APIKey = strings.TrimSpace(input.APIKey)
	}
	current.Enabled = normalized.Enabled
	s.providers[id] = current
	return providerPublicFromAccess(current), nil
}

func (s *ModelProviderService) Delete(ctx context.Context, id string) error {
	if s == nil {
		return fmt.Errorf("model provider service is nil")
	}
	id = strings.TrimSpace(id)
	if s.pool != nil {
		tag, err := s.pool.Exec(ctx, `DELETE FROM model_providers WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrModelProviderNotFound
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[id]; !ok {
		return ErrModelProviderNotFound
	}
	delete(s.providers, id)
	return nil
}

func normalizeModelProviderInput(input ModelProviderInput, create bool) (ModelProviderInput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ModelProviderInput{}, fmt.Errorf("provider name is required")
	}
	providerType := strings.TrimSpace(input.ProviderType)
	if providerType == "" {
		providerType = ModelProviderTypeOpenAICompatible
	}
	if providerType != ModelProviderTypeOpenAICompatible {
		return ModelProviderInput{}, fmt.Errorf("provider_type only supports openai_compatible")
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		return ModelProviderInput{}, fmt.Errorf("base_url is required")
	}
	if _, err := validateBaseURL(baseURL); err != nil {
		return ModelProviderInput{}, err
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return ModelProviderInput{
		Name:         name,
		ProviderType: providerType,
		BaseURL:      baseURL,
		Enabled:      input.Enabled,
		Metadata:     metadata,
	}, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanModelProviderPublic(row rowScanner) (ModelProvider, error) {
	var provider ModelProvider
	var metadata []byte
	if err := row.Scan(
		&provider.ID,
		&provider.Name,
		&provider.ProviderType,
		&provider.BaseURL,
		&provider.APIKeyConfigured,
		&provider.Enabled,
		&metadata,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	); err != nil {
		return ModelProvider{}, err
	}
	provider.Metadata = map[string]any{}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &provider.Metadata)
	}
	return provider, nil
}

func providerPublicFromAccess(access ModelProviderAccess) ModelProvider {
	now := time.Now()
	return ModelProvider{
		ID:               access.ID,
		Name:             access.Name,
		ProviderType:     access.ProviderType,
		BaseURL:          access.BaseURL,
		APIKeyConfigured: strings.TrimSpace(access.APIKey) != "",
		Enabled:          access.Enabled,
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}
