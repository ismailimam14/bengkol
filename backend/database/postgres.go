package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/pkg/logger"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps standard sql.DB with helper methods.
type DB struct {
	*sql.DB
	logger *logger.Logger
}

// NewPostgres creates and verifies a connection pool to PostgreSQL.
func NewPostgres(cfg config.DatabaseConfig, log *logger.Logger) (*DB, error) {
	dsn := cfg.DSN()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database driver: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime())

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("successfully connected to postgresql database", "host", cfg.Host, "database", cfg.Name)

	return &DB{
		DB:     db,
		logger: log,
	}, nil
}

// PingCheck verifies database responsiveness for health checks.
func (db *DB) PingCheck(ctx context.Context) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctxTimeout)
}
