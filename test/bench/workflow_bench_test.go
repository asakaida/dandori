package bench_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/asakaida/dandori/internal/adapter/postgres"
	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/engine"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	benchDB  *sql.DB
	benchEng *engine.Engine
	store    *postgres.Store
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dandori_bench"),
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

	benchDB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	benchDB.SetMaxOpenConns(25)
	benchDB.SetMaxIdleConns(5)

	if err := postgres.RunMigrations(ctx, benchDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	store = postgres.New(benchDB)
	benchEng = engine.New(
		store.Workflows(),
		store.Events(),
		store.WorkflowTasks(),
		store.ActivityTasks(),
		store.Timers(),
		store.Queries(),
		store.Namespaces(),
		store,
		engine.NewBroadcaster(),
	)

	code := m.Run()

	benchDB.Close()
	if err := pgContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %v", err)
	}
	os.Exit(code)
}

func truncateAll(b *testing.B) {
	b.Helper()
	tables := []string{"workflow_queries", "timers", "activity_tasks", "workflow_tasks", "workflow_events", "workflow_executions"}
	for _, table := range tables {
		if _, err := benchDB.ExecContext(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			b.Fatalf("failed to truncate %s: %v", table, err)
		}
	}
}

// BenchmarkWorkflowCreate measures workflow creation throughput.
func BenchmarkWorkflowCreate(b *testing.B) {
	truncateAll(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchEng.StartWorkflow(ctx, port.StartWorkflowParams{
			ID:           uuid.New(),
			Namespace:    "default",
			WorkflowType: "bench-workflow",
			TaskQueue:    "bench-queue",
			Input:        json.RawMessage(`{"iteration":` + fmt.Sprintf("%d", i) + `}`),
		})
		if err != nil {
			b.Fatalf("StartWorkflow: %v", err)
		}
	}
}

// BenchmarkEventAppend measures event append throughput for a single workflow.
func BenchmarkEventAppend(b *testing.B) {
	truncateAll(b)
	ctx := context.Background()

	wf, err := benchEng.StartWorkflow(ctx, port.StartWorkflowParams{
		ID:           uuid.New(),
		Namespace:    "default",
		WorkflowType: "bench-workflow",
		TaskQueue:    "bench-queue",
		Input:        json.RawMessage(`{}`),
	})
	if err != nil {
		b.Fatalf("StartWorkflow: %v", err)
	}

	events := store.Events()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := events.Append(ctx, []domain.HistoryEvent{
			{
				WorkflowID: wf.ID,
				Type:       domain.EventActivityTaskScheduled,
				Data:       json.RawMessage(`{"seq_id":` + fmt.Sprintf("%d", i) + `}`),
			},
		})
		if err != nil {
			b.Fatalf("Append: %v", err)
		}
	}
}

// BenchmarkEventGetByWorkflowID measures event retrieval throughput.
func BenchmarkEventGetByWorkflowID(b *testing.B) {
	truncateAll(b)
	ctx := context.Background()

	wf, err := benchEng.StartWorkflow(ctx, port.StartWorkflowParams{
		ID:           uuid.New(),
		Namespace:    "default",
		WorkflowType: "bench-workflow",
		TaskQueue:    "bench-queue",
		Input:        json.RawMessage(`{}`),
	})
	if err != nil {
		b.Fatalf("StartWorkflow: %v", err)
	}

	events := store.Events()
	for i := 0; i < 100; i++ {
		if err := events.Append(ctx, []domain.HistoryEvent{
			{
				WorkflowID: wf.ID,
				Type:       domain.EventActivityTaskScheduled,
				Data:       json.RawMessage(`{"seq_id":` + fmt.Sprintf("%d", i) + `}`),
			},
		}); err != nil {
			b.Fatalf("Append: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := events.GetByWorkflowID(ctx, wf.ID)
		if err != nil {
			b.Fatalf("GetByWorkflowID: %v", err)
		}
	}
}

// BenchmarkTaskPollComplete measures the Poll -> Complete cycle for workflow tasks.
func BenchmarkTaskPollComplete(b *testing.B) {
	truncateAll(b)
	ctx := context.Background()
	wfTasks := store.WorkflowTasks()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wfID := uuid.New()
		if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID: wfID, Namespace: "default", WorkflowType: "bench", TaskQueue: "bench-queue", Status: domain.WorkflowStatusRunning,
		}); err != nil {
			b.Fatalf("Create workflow: %v", err)
		}

		if err := wfTasks.Enqueue(ctx, domain.WorkflowTask{
			Namespace: "default", QueueName: "bench-queue", WorkflowID: wfID, ScheduledAt: time.Now(),
		}); err != nil {
			b.Fatalf("Enqueue: %v", err)
		}

		task, err := wfTasks.Poll(ctx, "default", "bench-queue", "bench-worker")
		if err != nil {
			b.Fatalf("Poll: %v", err)
		}

		if err := wfTasks.Complete(ctx, task.ID); err != nil {
			b.Fatalf("Complete: %v", err)
		}
	}
}

// BenchmarkActivityTaskPollComplete measures activity task Poll -> Complete cycle.
func BenchmarkActivityTaskPollComplete(b *testing.B) {
	truncateAll(b)
	ctx := context.Background()
	actTasks := store.ActivityTasks()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wfID := uuid.New()
		if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID: wfID, Namespace: "default", WorkflowType: "bench", TaskQueue: "bench-queue", Status: domain.WorkflowStatusRunning,
		}); err != nil {
			b.Fatalf("Create workflow: %v", err)
		}

		if err := actTasks.Enqueue(ctx, domain.ActivityTask{
			Namespace:    "default",
			QueueName:    "bench-queue",
			WorkflowID:   wfID,
			ActivityType: "bench-activity",
			ActivityInput: json.RawMessage(`{}`),
			ActivitySeqID: int64(i),
			Attempt:      1,
			MaxAttempts:  3,
			ScheduledAt:  time.Now(),
		}); err != nil {
			b.Fatalf("Enqueue: %v", err)
		}

		task, err := actTasks.Poll(ctx, "default", "bench-queue", "bench-worker")
		if err != nil {
			b.Fatalf("Poll: %v", err)
		}

		if err := actTasks.Complete(ctx, task.ID); err != nil {
			b.Fatalf("Complete: %v", err)
		}
	}
}
