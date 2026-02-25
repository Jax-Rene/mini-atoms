package config

import "testing"

func TestLoadFromLookup_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromLookup(func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.AppEnv != defaultAppEnv {
		t.Fatalf("AppEnv = %q, want %q", cfg.AppEnv, defaultAppEnv)
	}
	if cfg.HTTPAddr != defaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, defaultHTTPAddr)
	}
	if cfg.DatabasePath != defaultDatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, defaultDatabasePath)
	}
	if cfg.SessionSecret != defaultSessionKey {
		t.Fatalf("SessionSecret = %q, want default", cfg.SessionSecret)
	}
}

func TestLoadFromLookup_Overrides(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"APP_ENV":        "staging",
		"APP_ADDR":       ":9090",
		"DATABASE_PATH":  "./tmp/test.db",
		"SESSION_SECRET": "secret-123",
		"APP_BASE_URL":   "https://example.com",
	}

	cfg, err := loadFromLookup(func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.AppEnv != "staging" || cfg.HTTPAddr != ":9090" || cfg.DatabasePath != "./tmp/test.db" || cfg.SessionSecret != "secret-123" || cfg.AppBaseURL != "https://example.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadFromLookup_ProductionRequiresExplicitSecret(t *testing.T) {
	t.Parallel()

	_, err := loadFromLookup(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "production", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("expected error for production default session secret")
	}
}
