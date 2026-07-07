package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv        string
	APIAddr       string
	LocalAgentAddr string
	RedisAddr      string
	WorkerConcurrency int
	AdminUsername string
	AdminPassword string
	StorageRoot   string
}

func Load() Config {
	return Config{
		AppEnv:        env("APP_ENV", "development"),
		APIAddr:       env("API_ADDR", ":8080"),
		LocalAgentAddr: env("LOCAL_AGENT_ADDR", "127.0.0.1:58721"),
		RedisAddr:      env("REDIS_ADDR", "localhost:6379"),
		WorkerConcurrency: envInt("WORKER_CONCURRENCY", 4),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "admin"),
		StorageRoot:   env("STORAGE_LOCAL_ROOT", "./storage"),
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
