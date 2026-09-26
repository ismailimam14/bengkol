package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bengkol/backend/pkg/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrator handles database schema migrations.
type Migrator struct {
	db     *sql.DB
	logger *logger.Logger
}

// NewMigrator creates a new Migrator instance.
func NewMigrator(db *sql.DB, log *logger.Logger) *Migrator {
	return &Migrator{
		db:     db,
		logger: log,
	}
}

type migrationFile struct {
	version int
	name    string
	path    string
	isUp    bool
}

// EnsureSchemaTable creates the schema_migrations tracking table if not exists.
func (m *Migrator) EnsureSchemaTable(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := m.db.ExecContext(ctx, query)
	return err
}

// Up applies all pending up migrations.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.EnsureSchemaTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	appliedVersions, err := m.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migration versions: %w", err)
	}

	files, err := m.loadMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to load migration files: %w", err)
	}

	for _, f := range files {
		if !f.isUp {
			continue
		}
		if appliedVersions[f.version] {
			continue
		}

		content, err := migrationsFS.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", f.path, err)
		}

		m.logger.Info("applying migration", "version", f.version, "file", f.name)

		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin tx for migration %d: %w", f.version, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", f.name, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", f.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration version %d: %w", f.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", f.version, err)
		}

		m.logger.Info("successfully applied migration", "version", f.version, "file", f.name)
	}

	return nil
}

func (m *Migrator) getAppliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (m *Migrator) loadMigrationFiles() ([]migrationFile, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		isUp := strings.HasSuffix(entry.Name(), ".up.sql")
		files = append(files, migrationFile{
			version: version,
			name:    entry.Name(),
			path:    filepath.Join("migrations", entry.Name()),
			isUp:    isUp,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].version != files[j].version {
			return files[i].version < files[j].version
		}
		return files[i].name < files[j].name
	})

	return files, nil
}
