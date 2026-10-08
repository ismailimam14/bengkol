package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
)

func TestConfigLoad_Defaults(t *testing.T) {
	// Clear relevant env vars
	os.Unsetenv("APP_ENV")
	os.Unsetenv("APP_PORT")
	os.Unsetenv("DB_HOST")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}

	if cfg.AppPort != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.AppPort)
	}

	if cfg.Database.Host != "localhost" {
		t.Errorf("expected default db host localhost, got %s", cfg.Database.Host)
	}

	if cfg.Database.ConnMaxLifetime() != 15*time.Minute {
		t.Errorf("expected 15 min lifetime, got %v", cfg.Database.ConnMaxLifetime())
	}

	if !cfg.IsDevelop() {
		t.Errorf("expected default environment to be develop, got %s", cfg.AppEnv)
	}
}

func TestConfigLoad_CustomEnv(t *testing.T) {
	os.Setenv("APP_PORT", "9090")
	os.Setenv("DB_HOST", "db.example.com")
	defer func() {
		os.Unsetenv("APP_PORT")
		os.Unsetenv("DB_HOST")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config with custom env: %v", err)
	}

	if cfg.AppPort != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.AppPort)
	}

	if cfg.Database.Host != "db.example.com" {
		t.Errorf("expected db host db.example.com, got %s", cfg.Database.Host)
	}
}

func TestConfigLoad_EnvironmentVariants(t *testing.T) {
	tests := []struct {
		rawEnv       string
		expectedEnv  config.Environment
		isDevelop    bool
		isUAT        bool
		isStaging    bool
		isProd       bool
		expectedLog  string
		expectedSSL  string
	}{
		{"develop", config.EnvDevelop, true, false, false, false, "debug", "disable"},
		{"development", config.EnvDevelop, true, false, false, false, "debug", "disable"},
		{"dev", config.EnvDevelop, true, false, false, false, "debug", "disable"},
		{"uat", config.EnvUAT, false, true, false, false, "info", "disable"},
		{"staging", config.EnvStaging, false, false, true, false, "info", "require"},
		{"stage", config.EnvStaging, false, false, true, false, "info", "require"},
	}

	for _, tt := range tests {
		t.Run(tt.rawEnv, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.rawEnv)
			defer os.Unsetenv("APP_ENV")

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("unexpected error loading config for %s: %v", tt.rawEnv, err)
			}

			if cfg.Environment() != tt.expectedEnv {
				t.Errorf("expected canonical env %s, got %s", tt.expectedEnv, cfg.Environment())
			}
			if cfg.IsDevelop() != tt.isDevelop {
				t.Errorf("expected IsDevelop == %v", tt.isDevelop)
			}
			if cfg.IsUAT() != tt.isUAT {
				t.Errorf("expected IsUAT == %v", tt.isUAT)
			}
			if cfg.IsStaging() != tt.isStaging {
				t.Errorf("expected IsStaging == %v", tt.isStaging)
			}
			if cfg.IsProd() != tt.isProd {
				t.Errorf("expected IsProd == %v", tt.isProd)
			}
			if cfg.LogLevel != tt.expectedLog {
				t.Errorf("expected LogLevel %s, got %s", tt.expectedLog, cfg.LogLevel)
			}
			if cfg.Database.SSLMode != tt.expectedSSL {
				t.Errorf("expected SSLMode %s, got %s", tt.expectedSSL, cfg.Database.SSLMode)
			}
		})
	}
}

func TestConfigLoad_ProductionValidation(t *testing.T) {
	os.Setenv("APP_ENV", "prod")
	defer os.Unsetenv("APP_ENV")

	// 1. Missing secrets in prod should fail
	os.Unsetenv("JWT_ACCESS_SECRET")
	os.Unsetenv("JWT_REFRESH_SECRET")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when loading prod config without JWT secrets, got nil")
	}

	// 2. Weak secrets in prod should fail
	os.Setenv("JWT_ACCESS_SECRET", "short")
	os.Setenv("JWT_REFRESH_SECRET", "short")
	defer func() {
		os.Unsetenv("JWT_ACCESS_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
	}()

	_, err = config.Load()
	if err == nil {
		t.Fatal("expected error when loading prod config with short JWT secret, got nil")
	}

	// 3. Valid secrets in prod should succeed
	os.Setenv("JWT_ACCESS_SECRET", "very-strong-production-access-secret-32-chars-ok")
	os.Setenv("JWT_REFRESH_SECRET", "very-strong-production-refresh-secret-32-chars-ok")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected valid prod config to load, got error: %v", err)
	}

	if !cfg.IsProd() {
		t.Errorf("expected IsProd == true, got false")
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("expected prod default SSL mode require, got %s", cfg.Database.SSLMode)
	}
	if cfg.Database.MaxOpenConns != 100 {
		t.Errorf("expected prod max open conns 100, got %d", cfg.Database.MaxOpenConns)
	}
}

func TestConfigLoad_CORSAllowedOrigins(t *testing.T) {
	os.Setenv("CORS_ALLOWED_ORIGINS", "https://bengkol.com, https://admin.bengkol.com")
	defer os.Unsetenv("CORS_ALLOWED_ORIGINS")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed origins, got %d", len(cfg.CORS.AllowedOrigins))
	}
	if cfg.CORS.AllowedOrigins[0] != "https://bengkol.com" {
		t.Errorf("expected https://bengkol.com, got %s", cfg.CORS.AllowedOrigins[0])
	}
	if cfg.CORS.AllowedOrigins[1] != "https://admin.bengkol.com" {
		t.Errorf("expected https://admin.bengkol.com, got %s", cfg.CORS.AllowedOrigins[1])
	}
}
