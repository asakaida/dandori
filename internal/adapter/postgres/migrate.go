package postgres

import (
	"context"
	"database/sql"
	"embed"
)

//go:embed migration/*.sql
var migrationFS embed.FS

func RunMigrations(ctx context.Context, db *sql.DB) error {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='workflow_executions')").Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	upSQL, err := migrationFS.ReadFile("migration/000001_initial.up.sql")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(upSQL))
	return err
}
