package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	defaultAppEnv       = "development"
	defaultHTTPAddr     = ":8080"
	defaultDatabasePath = "./data/mini-atoms-gorm.db"
	defaultSessionKey   = "dev-session-secret-change-me"
	defaultAppBaseURL   = "http://localhost:8080"
	defaultDebugToken   = "dev-debug-reset-token-change-me"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabasePath           string
	SessionSecret          string
	DeepSeekAPIKey         string
	AppBaseURL             string
	DebugResetAllDataToken string
}

type lookupEnvFunc func(string) (string, bool)

func Load() (Config, error) {
	return loadFromLookup(os.LookupEnv)
}

func loadFromLookup(lookup lookupEnvFunc) (Config, error) {
	cfg := Config{
		AppEnv:                 getOrDefault(lookup, "APP_ENV", defaultAppEnv),
		HTTPAddr:               getOrDefault(lookup, "APP_ADDR", defaultHTTPAddr),
		DatabasePath:           getOrDefault(lookup, "DATABASE_PATH", defaultDatabasePath),
		SessionSecret:          getOrDefault(lookup, "SESSION_SECRET", defaultSessionKey),
		DeepSeekAPIKey:         getOrDefault(lookup, "DEEPSEEK_API_KEY", ""),
		AppBaseURL:             getOrDefault(lookup, "APP_BASE_URL", defaultAppBaseURL),
		DebugResetAllDataToken: getOrDefault(lookup, "DEBUG_RESET_ALL_DATA_TOKEN", defaultDebugToken),
	}

	cfg.AppEnv = strings.TrimSpace(cfg.AppEnv)
	cfg.HTTPAddr = strings.TrimSpace(cfg.HTTPAddr)
	cfg.DatabasePath = strings.TrimSpace(cfg.DatabasePath)
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)
	cfg.DeepSeekAPIKey = strings.TrimSpace(cfg.DeepSeekAPIKey)
	cfg.AppBaseURL = strings.TrimSpace(cfg.AppBaseURL)
	cfg.DebugResetAllDataToken = strings.TrimSpace(cfg.DebugResetAllDataToken)

	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("APP_ADDR cannot be empty")
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("DATABASE_PATH cannot be empty")
	}
	if cfg.SessionSecret == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET cannot be empty")
	}
	if strings.EqualFold(cfg.AppEnv, "production") && cfg.SessionSecret == defaultSessionKey {
		return Config{}, fmt.Errorf("SESSION_SECRET must be explicitly set in production")
	}

	return cfg, nil
}

func getOrDefault(lookup lookupEnvFunc, key, def string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return def
}
