package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migration/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	file    string
}

func RunMigrations(ctx context.Context, db *sql.DB) error {
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return err
	}

	migrations := []migration{
		{version: 1, name: "initial", file: "migration/000001_initial.up.sql"},
		{version: 2, name: "heartbeat", file: "migration/000002_heartbeat.up.sql"},
		{version: 3, name: "activity_timeouts", file: "migration/000003_activity_timeouts.up.sql"},
		{version: 4, name: "child_workflow", file: "migration/000004_child_workflow.up.sql"},
		{version: 5, name: "query", file: "migration/000005_query.up.sql"},
		{version: 6, name: "cron", file: "migration/000006_cron.up.sql"},
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, m := range migrations {
		applied, err := isMigrationApplied(ctx, db, m.version)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}

		upSQL, err := migrationFS.ReadFile(m.file)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", m.version, err)
		}
		if _, err := db.ExecContext(ctx, string(upSQL)); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
		if err := recordMigration(ctx, db, m.version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}

func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Backward compatibility: if workflow_executions exists but version 1 isn't recorded, register it
	var wfExists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='workflow_executions')").Scan(&wfExists); err != nil {
		return err
	}
	if wfExists {
		applied, err := isMigrationApplied(ctx, db, 1)
		if err != nil {
			return err
		}
		if !applied {
			if err := recordMigration(ctx, db, 1); err != nil {
				return err
			}
		}
	}
	return nil
}

func isMigrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
	return exists, err
}

func recordMigration(ctx context.Context, db *sql.DB, version int) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", version)
	return err
}

// MigrationFiles returns all embedded up migration SQL filenames (for testing).
func MigrationFiles() []string {
	entries, _ := migrationFS.ReadDir("migration")
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, "migration/"+e.Name())
		}
	}
	sort.Strings(files)
	return files
}

// ReadMigrationFile reads an embedded migration file (for testing).
func ReadMigrationFile(name string) ([]byte, error) {
	return migrationFS.ReadFile(name)
}
