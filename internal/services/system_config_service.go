package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

var ErrConfigNotFound = errors.New("system config not found")

type SystemConfig struct {
	Key         string `json:"key"`
	Value       any    `json:"value"`
	Type        string `json:"type"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description,omitempty"`
}

type SystemConfigService struct {
	mu      sync.RWMutex
	configs map[string]SystemConfig
	queries db.Querier
}

func NewSystemConfigService() *SystemConfigService {
	service := &SystemConfigService{
		configs: map[string]SystemConfig{},
	}
	service.seedDefaults()
	return service
}

func NewSystemConfigServiceWithQueries(queries db.Querier) (*SystemConfigService, error) {
	service := &SystemConfigService{
		configs: map[string]SystemConfig{},
		queries: queries,
	}
	if err := service.ensureDefaults(context.Background()); err != nil {
		return nil, err
	}
	if err := service.reload(context.Background()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *SystemConfigService) List() ([]SystemConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.configs))
	for key := range s.configs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	configs := make([]SystemConfig, 0, len(keys))
	for _, key := range keys {
		configs = append(configs, s.configs[key])
	}
	return configs, nil
}

func (s *SystemConfigService) Get(key string) (SystemConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, ok := s.configs[key]
	if !ok {
		return SystemConfig{}, ErrConfigNotFound
	}
	return config, nil
}

func (s *SystemConfigService) Upsert(config SystemConfig) (SystemConfig, error) {
	if s.queries != nil {
		stored, err := s.upsertInStore(context.Background(), config)
		if err != nil {
			return SystemConfig{}, err
		}

		s.mu.Lock()
		s.configs[stored.Key] = stored
		s.mu.Unlock()
		return stored, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.configs[config.Key]
	if config.Type == "" {
		config.Type = existing.Type
	}
	if config.Description == "" {
		config.Description = existing.Description
	}
	config.IsSecret = existing.IsSecret || config.IsSecret
	s.configs[config.Key] = config
	return config, nil
}

func (s *SystemConfigService) Snapshot() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]any, len(s.configs))
	for key, config := range s.configs {
		if config.IsSecret {
			snapshot[key] = "<secret>"
			continue
		}
		snapshot[key] = config.Value
	}
	return snapshot, nil
}

func (s *SystemConfigService) seedDefaults() {
	for _, config := range defaultSystemConfigs() {
		s.configs[config.Key] = config
	}
}

func defaultSystemConfigs() []SystemConfig {
	return []SystemConfig{
		{Key: openAIBaseURLKey, Value: "", Type: "string", Description: "OpenAI-compatible API base URL"},
		{Key: openAIAPIKeyKey, Value: "", Type: "string", IsSecret: true, Description: "OpenAI-compatible API key"},
		{Key: "llm.provider", Value: "openai_compatible", Type: "string", Description: "Default LLM provider"},
		{Key: "llm.provider_id", Value: "", Type: "string", Description: "Default LLM model provider ID"},
		{Key: "llm.model", Value: "", Type: "string", Description: "Default LLM model"},
		{Key: "llm.max_concurrency", Value: 2, Type: "number", Description: "Global LLM concurrency"},
		{Key: "vlm.provider", Value: "openai_compatible", Type: "string", Description: "Default VLM provider"},
		{Key: "vlm.provider_id", Value: "", Type: "string", Description: "Default VLM model provider ID"},
		{Key: "vlm.model", Value: "", Type: "string", Description: "Default VLM model"},
		{Key: "vlm.timeout_seconds", Value: 120, Type: "number", Description: "VLM request timeout seconds"},
		{Key: "vlm.max_retries", Value: 2, Type: "number", Description: "VLM request max retries"},
		{Key: "vlm.max_concurrency", Value: 2, Type: "number", Description: "Global VLM concurrency"},
		{Key: "asr.provider", Value: "external", Type: "string", Description: "Default ASR provider"},
		{Key: "asr.max_concurrency", Value: 2, Type: "number", Description: "Global ASR concurrency"},
		{Key: "tts.provider", Value: "external", Type: "string", Description: "Default TTS provider"},
		{Key: "tts.max_concurrency", Value: 2, Type: "number", Description: "Global TTS concurrency"},
		{Key: "embedding.provider", Value: "openai_compatible", Type: "string", Description: "Default embedding provider"},
		{Key: "embedding.provider_id", Value: "", Type: "string", Description: "Default embedding model provider ID"},
		{Key: "embedding.model", Value: "", Type: "string", Description: "Default embedding model"},
		{Key: "render.max_concurrency", Value: 1, Type: "number", Description: "Global render concurrency"},
		{Key: "task.max_queued_per_user", Value: 20, Type: "number", Description: "Max queued tasks per user"},
		{Key: "task.max_running_per_user", Value: 2, Type: "number", Description: "Max running tasks per user"},
		{Key: "storage.backend", Value: "local", Type: "string", Description: "Storage backend"},
	}
}

func (s *SystemConfigService) ensureDefaults(ctx context.Context) error {
	if s.queries == nil {
		s.seedDefaults()
		return nil
	}

	for _, config := range defaultSystemConfigs() {
		_, err := s.queries.GetSystemConfigByKey(ctx, config.Key)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := s.upsertInStore(ctx, config); err != nil {
			return err
		}
	}
	return nil
}

func (s *SystemConfigService) reload(ctx context.Context) error {
	if s.queries == nil {
		return nil
	}

	rows, err := s.queries.ListSystemConfigs(ctx)
	if err != nil {
		return err
	}

	loaded := make(map[string]SystemConfig, len(rows))
	for _, row := range rows {
		config, err := systemConfigFromDB(row)
		if err != nil {
			return err
		}
		loaded[config.Key] = config
	}

	s.mu.Lock()
	s.configs = loaded
	s.mu.Unlock()
	return nil
}

func (s *SystemConfigService) upsertInStore(ctx context.Context, config SystemConfig) (SystemConfig, error) {
	if s.queries == nil {
		return config, nil
	}

	payload, err := json.Marshal(config.Value)
	if err != nil {
		return SystemConfig{}, err
	}

	row, err := s.queries.UpsertSystemConfig(ctx, db.UpsertSystemConfigParams{
		ConfigKey:       config.Key,
		ConfigValue:     payload,
		ConfigType:      firstNonEmpty(config.Type, "json"),
		IsSecret:        config.IsSecret,
		Description:     pgTextParam(config.Description),
		UpdatedByUserID: pgtype.UUID{},
	})
	if err != nil {
		return SystemConfig{}, err
	}

	return systemConfigFromDB(row)
}

func systemConfigFromDB(row db.SystemConfig) (SystemConfig, error) {
	var value any
	if len(row.ConfigValue) > 0 {
		if err := json.Unmarshal(row.ConfigValue, &value); err != nil {
			return SystemConfig{}, err
		}
	}

	return SystemConfig{
		Key:         row.ConfigKey,
		Value:       value,
		Type:        row.ConfigType,
		IsSecret:    row.IsSecret,
		Description: systemConfigTextString(row.Description),
	}, nil
}

func pgTextParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func systemConfigTextString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
