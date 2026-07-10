package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	AppEnv              string
	APIAddr             string
	LocalAgentAddr      string
	DatabaseURL         string
	QueueBackend        string
	RedisAddr           string
	WorkerConcurrency   int
	AdminUsername       string
	AdminPassword       string
	StorageRoot         string
	VLMProvider         string
	VLMModel            string
	VLMBaseURL          string
	VLMAPIKey           string
	VLMMaxTokens        int
	ModelGatewayTimeout time.Duration
	VLMMaxRetries       int
}

func Load() Config {
	return Config{
		AppEnv:              env("APP_ENV", "development"),
		APIAddr:             env("API_ADDR", ":8080"),
		LocalAgentAddr:      env("LOCAL_AGENT_ADDR", "127.0.0.1:58721"),
		DatabaseURL:         env("DATABASE_URL", ""),
		QueueBackend:        env("QUEUE_BACKEND", "redis"),
		RedisAddr:           env("REDIS_ADDR", "localhost:6379"),
		WorkerConcurrency:   envInt("WORKER_CONCURRENCY", 4),
		AdminUsername:       env("ADMIN_USERNAME", "admin"),
		AdminPassword:       env("ADMIN_PASSWORD", "admin"),
		StorageRoot:         env("STORAGE_LOCAL_ROOT", "./storage"),
		VLMProvider:         env("VLM_PROVIDER", "mock"),
		VLMModel:            env("VLM_MODEL", ""),
		VLMBaseURL:          env("VLM_BASE_URL", env("OPENAI_COMPATIBLE_BASE_URL", "")),
		VLMAPIKey:           env("VLM_API_KEY", env("OPENAI_COMPATIBLE_API_KEY", "")),
		VLMMaxTokens:        envInt("VLM_MAX_TOKENS", 8192),
		ModelGatewayTimeout: time.Duration(envInt("MODEL_GATEWAY_TIMEOUT_SECONDS", 120)) * time.Second,
		VLMMaxRetries:       envInt("VLM_MAX_RETRIES", 2),
	}
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
