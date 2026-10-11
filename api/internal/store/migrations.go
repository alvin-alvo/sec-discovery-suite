package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sds/api/migrations"
)

func applyMigrations(ctx context.Context, db *sql.DB) error {
	createMigTable := `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INT PRIMARY KEY,
  applied_at TIMESTAMPTZ
);`
	if _, err := db.ExecContext(ctx, createMigTable); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration 1 status: %w", err)
	}

	if count == 0 {
		if _, err := db.ExecContext(ctx, migrations.InitSQL); err != nil {
			return fmt.Errorf("failed to apply migration 001_init.sql: %w", err)
		}
		insertMig := "INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)"
		if _, err := db.ExecContext(ctx, insertMig, time.Now().UTC()); err != nil {
			return fmt.Errorf("failed to record migration 1: %w", err)
		}
	}

	return nil
}
