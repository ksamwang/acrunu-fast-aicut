package config

import "os"

type Config struct {
	AppEnv        string
	APIAddr       string
	LocalAgentAddr string
	AdminUsername string
	AdminPassword string
	StorageRoot   string
}

func Load() Config {
	return Config{
		AppEnv:        env("APP_ENV", "development"),
		APIAddr:       env("API_ADDR", ":8080"),
		LocalAgentAddr: env("LOCAL_AGENT_ADDR", "127.0.0.1:58721"),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "admin"),
		StorageRoot:   env("STORAGE_LOCAL_ROOT", "./storage"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
