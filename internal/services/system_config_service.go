package services

import (
	"errors"
	"sort"
	"sync"
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
}

func NewSystemConfigService() *SystemConfigService {
	service := &SystemConfigService{
		configs: map[string]SystemConfig{},
	}
	service.seedDefaults()
	return service
}

func (s *SystemConfigService) List() []SystemConfig {
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
	return configs
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

func (s *SystemConfigService) Upsert(config SystemConfig) SystemConfig {
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
	return config
}

func (s *SystemConfigService) Snapshot() map[string]any {
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
	return snapshot
}

func (s *SystemConfigService) seedDefaults() {
	defaults := []SystemConfig{
		{Key: "llm.provider", Value: "openai_compatible", Type: "string", Description: "Default LLM provider"},
		{Key: "llm.model", Value: "", Type: "string", Description: "Default LLM model"},
		{Key: "llm.max_concurrency", Value: 2, Type: "number", Description: "Global LLM concurrency"},
		{Key: "vlm.provider", Value: "openai_compatible", Type: "string", Description: "Default VLM provider"},
		{Key: "vlm.model", Value: "", Type: "string", Description: "Default VLM model"},
		{Key: "vlm.max_concurrency", Value: 2, Type: "number", Description: "Global VLM concurrency"},
		{Key: "asr.provider", Value: "external", Type: "string", Description: "Default ASR provider"},
		{Key: "asr.max_concurrency", Value: 2, Type: "number", Description: "Global ASR concurrency"},
		{Key: "tts.provider", Value: "external", Type: "string", Description: "Default TTS provider"},
		{Key: "tts.max_concurrency", Value: 2, Type: "number", Description: "Global TTS concurrency"},
		{Key: "embedding.provider", Value: "openai_compatible", Type: "string", Description: "Default embedding provider"},
		{Key: "embedding.model", Value: "", Type: "string", Description: "Default embedding model"},
		{Key: "render.max_concurrency", Value: 1, Type: "number", Description: "Global render concurrency"},
		{Key: "task.max_queued_per_user", Value: 20, Type: "number", Description: "Max queued tasks per user"},
		{Key: "task.max_running_per_user", Value: 2, Type: "number", Description: "Max running tasks per user"},
		{Key: "storage.backend", Value: "local", Type: "string", Description: "Storage backend"},
	}

	for _, config := range defaults {
		s.configs[config.Key] = config
	}
}
