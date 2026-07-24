package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

type mutableSystemConfigQuerier struct {
	db.Querier
	rows []db.SystemConfig
	err  error
}

func (q *mutableSystemConfigQuerier) ListSystemConfigs(context.Context) ([]db.SystemConfig, error) {
	if q.err != nil {
		return nil, q.err
	}
	return append([]db.SystemConfig(nil), q.rows...), nil
}

func TestSystemConfigServiceSeedsVLMRuntimeDefaults(t *testing.T) {
	service := NewSystemConfigService()

	configs, err := service.List()
	if err != nil {
		t.Fatalf("list configs failed: %v", err)
	}
	if len(configs) == 0 {
		t.Fatalf("expected seeded configs")
	}

	timeoutConfig, err := service.Get("vlm.timeout_seconds")
	if err != nil {
		t.Fatalf("expected vlm.timeout_seconds config, got %v", err)
	}
	if timeout, ok := timeoutConfig.Value.(int); !ok || timeout != 120 {
		t.Fatalf("expected default timeout 120, got %#v", timeoutConfig.Value)
	}

	retryConfig, err := service.Get("vlm.max_retries")
	if err != nil {
		t.Fatalf("expected vlm.max_retries config, got %v", err)
	}
	if retries, ok := retryConfig.Value.(int); !ok || retries != 2 {
		t.Fatalf("expected default retries 2, got %#v", retryConfig.Value)
	}
}

func TestSystemConfigServiceUpsertAndSnapshot(t *testing.T) {
	service := NewSystemConfigService()

	if _, err := service.Upsert(SystemConfig{
		Key:      "vlm.provider",
		Value:    "mock",
		Type:     "string",
		IsSecret: false,
	}); err != nil {
		t.Fatalf("upsert config failed: %v", err)
	}

	if _, err := service.Upsert(SystemConfig{
		Key:      "provider.api_key",
		Value:    "secret-value",
		Type:     "string",
		IsSecret: true,
	}); err != nil {
		t.Fatalf("upsert secret config failed: %v", err)
	}

	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if snapshot["vlm.provider"] != "mock" {
		t.Fatalf("expected snapshot to include updated provider, got %#v", snapshot["vlm.provider"])
	}
	if snapshot["provider.api_key"] != "<secret>" {
		t.Fatalf("expected secret to be masked, got %#v", snapshot["provider.api_key"])
	}
}

func TestSystemConfigServiceRefreshesSharedDatabaseValues(t *testing.T) {
	queries := &mutableSystemConfigQuerier{rows: []db.SystemConfig{
		{ConfigKey: "llm.model", ConfigValue: []byte(`"gpt-5.6-luna"`), ConfigType: "string"},
	}}
	service := &SystemConfigService{
		configs: map[string]SystemConfig{
			"llm.model": {Key: "llm.model", Value: "stale-model", Type: "string"},
		},
		queries: queries,
	}

	if err := service.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh config: %v", err)
	}
	config, err := service.Get("llm.model")
	if err != nil {
		t.Fatalf("get refreshed config: %v", err)
	}
	if config.Value != "gpt-5.6-luna" {
		t.Fatalf("expected refreshed model, got %#v", config.Value)
	}

	queries.err = errors.New("database unavailable")
	if err := service.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}
	config, err = service.Get("llm.model")
	if err != nil || config.Value != "gpt-5.6-luna" {
		t.Fatalf("expected last known config after refresh failure, got %#v, err=%v", config, err)
	}
}
