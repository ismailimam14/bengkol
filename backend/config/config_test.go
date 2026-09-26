package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
)

func TestConfigLoad_Defaults(t *testing.T) {
	// Clear relevant env vars
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
