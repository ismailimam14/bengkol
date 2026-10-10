package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Environment represents supported runtime environment variants.
type Environment string

const (
	EnvDevelop Environment = "develop"
	EnvUAT     Environment = "uat"
	EnvStaging Environment = "staging"
	EnvProd    Environment = "prod"
)

// NormalizeEnv normalizes environment strings and aliases into canonical Environment values.
func NormalizeEnv(raw string) Environment {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "prod", "production":
		return EnvProd
	case "staging", "stage":
		return EnvStaging
	case "uat":
		return EnvUAT
	case "develop", "development", "dev":
		fallthrough
	default:
		return EnvDevelop
	}
}

// Config holds all configuration values for the Bengkol application.
type Config struct {
	AppEnv    string
	AppPort   string
	AppName   string
	LogLevel  string
	UploadDir string

	Database DatabaseConfig
	JWT      JWTConfig
	CORS     CORSConfig
}

// Environment returns the canonical Environment representation of AppEnv.
func (c *Config) Environment() Environment {
	return NormalizeEnv(c.AppEnv)
}

// IsDevelop returns true if the active environment is develop/development.
func (c *Config) IsDevelop() bool {
	return c.Environment() == EnvDevelop
}

// IsUAT returns true if the active environment is UAT.
func (c *Config) IsUAT() bool {
	return c.Environment() == EnvUAT
}

// IsStaging returns true if the active environment is staging.
func (c *Config) IsStaging() bool {
	return c.Environment() == EnvStaging
}

// IsProd returns true if the active environment is production.
func (c *Config) IsProd() bool {
	return c.Environment() == EnvProd
}

// DatabaseConfig holds PostgreSQL connection configuration.
type DatabaseConfig struct {
	Host               string
	Port               int
	User               string
	Password           string
	Name               string
	SSLMode            string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetimeMin int
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	AccessSecret        string
	RefreshSecret       string
	AccessExpiryMinutes int
	RefreshExpiryDays   int
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

// Load loads configuration from environment variables, cascading through environment files.
func Load() (*Config, error) {
	// 1. Detect target environment from OS env
	rawEnv := os.Getenv("APP_ENV")
	if rawEnv == "" {
		rawEnv = os.Getenv("ENV")
	}
	if rawEnv == "" {
		rawEnv = os.Getenv("GO_ENV")
	}

	// If not in OS environment, check if base .env exists and defines APP_ENV
	if rawEnv == "" {
		for _, baseFile := range []string{".env", "backend/.env"} {
			if envMap, err := godotenv.Read(baseFile); err == nil {
				if val, ok := envMap["APP_ENV"]; ok && val != "" {
					rawEnv = val
					break
				}
			}
		}
	}

	env := NormalizeEnv(rawEnv)

	// 2. Cascade load .env files in priority order
	loadEnvCascade(env)

	// Refresh rawEnv after loading env files if it was empty initially
	if rawEnv == "" {
		rawEnv = os.Getenv("APP_ENV")
		if rawEnv == "" {
			rawEnv = os.Getenv("ENV")
		}
		env = NormalizeEnv(rawEnv)
	}

	// 3. Environment-tailored default values
	defaultLogLevel := "debug"
	defaultSSLMode := "disable"
	defaultMaxOpenConns := "25"
	defaultMaxIdleConns := "10"
	defaultOrigins := "*"

	switch env {
	case EnvProd:
		defaultLogLevel = "info"
		defaultSSLMode = "require"
		defaultMaxOpenConns = "100"
		defaultMaxIdleConns = "25"
		defaultOrigins = "https://bengkol.com,https://admin.bengkol.com"
	case EnvStaging:
		defaultLogLevel = "info"
		defaultSSLMode = "require"
		defaultMaxOpenConns = "50"
		defaultMaxIdleConns = "15"
		defaultOrigins = "https://staging.bengkol.com,https://staging-admin.bengkol.com"
	case EnvUAT:
		defaultLogLevel = "info"
		defaultSSLMode = "disable"
		defaultMaxOpenConns = "30"
		defaultMaxIdleConns = "10"
		defaultOrigins = "https://uat.bengkol.com,https://uat-admin.bengkol.com"
	case EnvDevelop:
		defaultLogLevel = "debug"
		defaultSSLMode = "disable"
		defaultMaxOpenConns = "25"
		defaultMaxIdleConns = "10"
		defaultOrigins = "*"
	}

	dbPort, err := strconv.Atoi(getEnv("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	maxOpenConns, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", defaultMaxOpenConns))
	maxIdleConns, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", defaultMaxIdleConns))
	connLifetime, _ := strconv.Atoi(getEnv("DB_CONN_MAX_LIFETIME_MINUTES", "15"))

	accessExpiry, _ := strconv.Atoi(getEnv("JWT_ACCESS_EXPIRY_MINUTES", "15"))
	refreshExpiry, _ := strconv.Atoi(getEnv("JWT_REFRESH_EXPIRY_DAYS", "7"))

	accessSecret := getEnv("JWT_ACCESS_SECRET", "")
	refreshSecret := getEnv("JWT_REFRESH_SECRET", "")

	if env == EnvProd {
		if accessSecret == "" || accessSecret == "super-secret-access-token-key-change-in-production" || len(accessSecret) < 32 {
			return nil, errors.New("JWT_ACCESS_SECRET must be set to a secure unique secret with at least 32 characters in production")
		}
		if refreshSecret == "" || refreshSecret == "super-secret-refresh-token-key-change-in-production" || len(refreshSecret) < 32 {
			return nil, errors.New("JWT_REFRESH_SECRET must be set to a secure unique secret with at least 32 characters in production")
		}
	} else {
		if accessSecret == "" {
			accessSecret = "dev-jwt-access-secret-32-chars-long!"
		}
		if refreshSecret == "" {
			refreshSecret = "dev-jwt-refresh-secret-32-chars-long!"
		}
	}

	rawOrigins := getEnv("CORS_ALLOWED_ORIGINS", defaultOrigins)
	origins := parseCommaSeparated(rawOrigins)

	cfg := &Config{
		AppEnv:    string(env),
		AppPort:   getEnv("APP_PORT", "8080"),
		AppName:   getEnv("APP_NAME", "bengkol-api"),
		LogLevel:  getEnv("LOG_LEVEL", defaultLogLevel),
		UploadDir: getEnv("UPLOAD_DIR", "uploads"),
		Database: DatabaseConfig{
			Host:               getEnv("DB_HOST", "localhost"),
			Port:               dbPort,
			User:               getEnv("DB_USER", "postgres"),
			Password:           getEnv("DB_PASSWORD", "postgres"),
			Name:               getEnv("DB_NAME", "bengkol_db"),
			SSLMode:            getEnv("DB_SSL_MODE", defaultSSLMode),
			MaxOpenConns:       maxOpenConns,
			MaxIdleConns:       maxIdleConns,
			ConnMaxLifetimeMin: connLifetime,
		},
		JWT: JWTConfig{
			AccessSecret:        accessSecret,
			RefreshSecret:       refreshSecret,
			AccessExpiryMinutes: accessExpiry,
			RefreshExpiryDays:   refreshExpiry,
		},
		CORS: CORSConfig{
			AllowedOrigins: origins,
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID", "appName", "appDevice", "appVersion", "X-App-Name", "X-App-Device", "X-App-Version"},
		},
	}

	return cfg, nil
}

func loadEnvCascade(env Environment) {
	envStr := string(env)
	candidates := []string{
		fmt.Sprintf(".env.%s.local", envStr),
		fmt.Sprintf("backend/.env.%s.local", envStr),
		fmt.Sprintf(".env.%s", envStr),
		fmt.Sprintf("backend/.env.%s", envStr),
		".env.local",
		"backend/.env.local",
		".env",
		"backend/.env",
	}

	var files []string
	for _, f := range candidates {
		if info, err := os.Stat(f); err == nil && !info.IsDir() {
			files = append(files, f)
		}
	}

	if len(files) > 0 {
		_ = godotenv.Load(files...)
	}
}

func parseCommaSeparated(raw string) []string {
	if raw == "" || raw == "*" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{"*"}
	}
	return result
}

// ConnMaxLifetime returns the database connection lifetime as a time.Duration.
func (d DatabaseConfig) ConnMaxLifetime() time.Duration {
	return time.Duration(d.ConnMaxLifetimeMin) * time.Minute
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
