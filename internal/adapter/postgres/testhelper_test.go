package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/asakaida/dandori/internal/adapter/postgres"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dandori_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %v", err)
	}

	testDB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}

	// Run migrations
	migrationSQL, err := os.ReadFile("migration/000001_initial.up.sql")
	if err != nil {
		log.Fatalf("failed to read migration: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, string(migrationSQL)); err != nil {
		log.Fatalf("failed to run migration: %v", err)
	}

	code := m.Run()

	testDB.Close()
	if err := pgContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %v", err)
	}
	os.Exit(code)
}

func newStore(t *testing.T) *postgres.Store {
	t.Helper()
	truncateAll(t)
	return postgres.New(testDB)
}

func truncateAll(t *testing.T) {
	t.Helper()
	tables := []string{"timers", "activity_tasks", "workflow_tasks", "workflow_events", "workflow_executions"}
	for _, table := range tables {
		if _, err := testDB.ExecContext(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			t.Fatalf("failed to truncate %s: %v", table, err)
		}
	}
}
