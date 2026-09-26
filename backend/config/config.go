package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the Bengkol application.
type Config struct {
	AppEnv   string
	AppPort  string
	AppName  string
	LogLevel string

	Database DatabaseConfig
	JWT      JWTConfig
	CORS     CORSConfig
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

// Load loads configuration from environment variables, optionally reading from .env file if present.
func Load() (*Config, error) {
	// Attempt to load .env file if it exists; ignore error if not found in prod
	_ = godotenv.Load()

	dbPort, err := strconv.Atoi(getEnv("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	maxOpenConns, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "25"))
	maxIdleConns, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "10"))
	connLifetime, _ := strconv.Atoi(getEnv("DB_CONN_MAX_LIFETIME_MINUTES", "15"))

	accessExpiry, _ := strconv.Atoi(getEnv("JWT_ACCESS_EXPIRY_MINUTES", "15"))
	refreshExpiry, _ := strconv.Atoi(getEnv("JWT_REFRESH_EXPIRY_DAYS", "7"))

	cfg := &Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		AppPort:  getEnv("APP_PORT", "8080"),
		AppName:  getEnv("APP_NAME", "bengkol-api"),
		LogLevel: getEnv("LOG_LEVEL", "debug"),
		Database: DatabaseConfig{
			Host:               getEnv("DB_HOST", "localhost"),
			Port:               dbPort,
			User:               getEnv("DB_USER", "postgres"),
			Password:           getEnv("DB_PASSWORD", "postgres"),
			Name:               getEnv("DB_NAME", "bengkol_db"),
			SSLMode:            getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:       maxOpenConns,
			MaxIdleConns:       maxIdleConns,
			ConnMaxLifetimeMin: connLifetime,
		},
		JWT: JWTConfig{
			AccessSecret:        getEnv("JWT_ACCESS_SECRET", "super-secret-access-token-key-change-in-production"),
			RefreshSecret:       getEnv("JWT_REFRESH_SECRET", "super-secret-refresh-token-key-change-in-production"),
			AccessExpiryMinutes: accessExpiry,
			RefreshExpiryDays:   refreshExpiry,
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"},
		},
	}

	return cfg, nil
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
