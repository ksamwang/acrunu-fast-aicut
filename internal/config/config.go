package config

import "os"

type Config struct {
	AppEnv        string
	APIAddr       string
	AdminUsername string
	AdminPassword string
}

func Load() Config {
	return Config{
		AppEnv:        env("APP_ENV", "development"),
		APIAddr:       env("API_ADDR", ":8080"),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "admin"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
