package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	grpcadapter "github.com/asakaida/dandori/internal/adapter/grpc"
	"github.com/asakaida/dandori/internal/adapter/postgres"
	"github.com/asakaida/dandori/internal/engine"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
)

const bufSize = 1024 * 1024

var (
	testDB     *sql.DB
	client     apiv1.DandoriServiceClient
	httpServer *httptest.Server
	bgCancel   context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dandori_e2e"),
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

	if err := postgres.RunMigrations(ctx, testDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Wire up components
	store := postgres.New(testDB)
	eng := engine.New(
		store.Workflows(),
		store.Events(),
		store.WorkflowTasks(),
		store.ActivityTasks(),
		store.Timers(),
		store.Queries(),
		store.Namespaces(),
		store,
	)
	bgWorker := engine.NewBackgroundWorker(
		store.Workflows(),
		store.Events(),
		store.WorkflowTasks(),
		store.ActivityTasks(),
		store.Timers(),
		store,
	)
	handler := grpcadapter.NewHandler(eng, eng, eng)

	// bufconn listener
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	apiv1.RegisterDandoriServiceServer(srv, handler)

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("grpc serve error: %v", err)
		}
	}()

	// gRPC client via bufconn
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to create grpc client: %v", err)
	}
	client = apiv1.NewDandoriServiceClient(conn)

	// HTTP server (gRPC-Gateway) for HTTP API tests
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)
	if err := apiv1.RegisterDandoriServiceHandlerServer(ctx, gwMux, handler); err != nil {
		log.Fatalf("failed to register gateway handler: %v", err)
	}
	topMux := http.NewServeMux()
	topMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := testDB.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})
	topMux.Handle("/", gwMux)
	httpServer = httptest.NewServer(topMux)

	// Background workers with fast intervals for testing
	bgCtx, cancel := context.WithCancel(ctx)
	bgCancel = cancel

	go func() {
		if err := bgWorker.RunActivityTimeoutChecker(bgCtx, 500*time.Millisecond); err != nil && bgCtx.Err() == nil {
			log.Printf("activity timeout checker stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunHeartbeatTimeoutChecker(bgCtx, 500*time.Millisecond); err != nil && bgCtx.Err() == nil {
			log.Printf("heartbeat timeout checker stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTimerPoller(bgCtx, 500*time.Millisecond); err != nil && bgCtx.Err() == nil {
			log.Printf("timer poller stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTaskRecovery(bgCtx, 2*time.Second); err != nil && bgCtx.Err() == nil {
			log.Printf("task recovery stopped: %v", err)
		}
	}()

	// Run tests
	code := m.Run()

	// Cleanup
	bgCancel()
	httpServer.Close()
	srv.GracefulStop()
	conn.Close()
	testDB.Close()
	if err := pgContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %v", err)
	}
	os.Exit(code)
}

// truncateAll clears all tables between tests.
func truncateAll(t *testing.T) {
	t.Helper()
	tables := []string{"workflow_queries", "timers", "activity_tasks", "workflow_tasks", "workflow_events", "workflow_executions"}
	for _, table := range tables {
		if _, err := testDB.ExecContext(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			t.Fatalf("failed to truncate %s: %v", table, err)
		}
	}
}

// pollWorkflowTaskUntil polls for a non-empty workflow task until timeout.
func pollWorkflowTaskUntil(t *testing.T, ctx context.Context, queue, workerID string, timeout time.Duration) *apiv1.PollWorkflowTaskResponse {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for workflow task on queue %s", queue)
		default:
		}
		resp, err := client.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
			QueueName: queue,
			WorkerId:  workerID,
		})
		if err != nil {
			t.Fatalf("PollWorkflowTask: %v", err)
		}
		if resp.GetTaskId() != 0 {
			return resp
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// pollActivityTaskUntil polls for a non-empty activity task until timeout.
func pollActivityTaskUntil(t *testing.T, ctx context.Context, queue, workerID string, timeout time.Duration) *apiv1.PollActivityTaskResponse {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for activity task on queue %s", queue)
		default:
		}
		resp, err := client.PollActivityTask(ctx, &apiv1.PollActivityTaskRequest{
			QueueName: queue,
			WorkerId:  workerID,
		})
		if err != nil {
			t.Fatalf("PollActivityTask: %v", err)
		}
		if resp.GetTaskId() != 0 {
			return resp
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// scheduleActivityCmd creates a ScheduleActivityTask command.
func scheduleActivityCmd(seqID int64, actType string, input json.RawMessage, timeout time.Duration, retryPolicy map[string]any) *apiv1.Command {
	attrs := map[string]any{
		"seq_id":                  seqID,
		"activity_type":          actType,
		"input":                  input,
		"start_to_close_timeout": int64(timeout),
	}
	if retryPolicy != nil {
		attrs["retry_policy"] = retryPolicy
	}
	data, _ := json.Marshal(attrs)
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK,
		Attributes: data,
	}
}

// completeWorkflowCmd creates a CompleteWorkflow command.
func completeWorkflowCmd(result json.RawMessage) *apiv1.Command {
	attrs, _ := json.Marshal(map[string]json.RawMessage{"result": result})
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_COMPLETE_WORKFLOW,
		Attributes: attrs,
	}
}

// failWorkflowCmd creates a FailWorkflow command.
func failWorkflowCmd(errorMessage string) *apiv1.Command {
	attrs, _ := json.Marshal(map[string]string{"error_message": errorMessage})
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_FAIL_WORKFLOW,
		Attributes: attrs,
	}
}

// findEvent searches events for a given event type.
func findEvent(events []*apiv1.HistoryEvent, eventType string) *apiv1.HistoryEvent {
	for _, e := range events {
		if e.GetEventType() == eventType {
			return e
		}
	}
	return nil
}

// countEvents counts events of a given type.
func countEvents(events []*apiv1.HistoryEvent, eventType string) int {
	n := 0
	for _, e := range events {
		if e.GetEventType() == eventType {
			n++
		}
	}
	return n
}

// startTimerCmd creates a StartTimer command.
func startTimerCmd(seqID int64, duration time.Duration) *apiv1.Command {
	attrs, _ := json.Marshal(map[string]any{
		"seq_id":   seqID,
		"duration": int64(duration),
	})
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_START_TIMER,
		Attributes: attrs,
	}
}

// scheduleActivityCmdWithHeartbeat creates a ScheduleActivityTask command with heartbeat_timeout.
func scheduleActivityCmdWithHeartbeat(seqID int64, actType string, input json.RawMessage, stcTimeout time.Duration, hbTimeout time.Duration, retryPolicy map[string]any) *apiv1.Command {
	attrs := map[string]any{
		"seq_id":            seqID,
		"activity_type":    actType,
		"input":            input,
		"heartbeat_timeout": int64(hbTimeout),
	}
	if stcTimeout > 0 {
		attrs["start_to_close_timeout"] = int64(stcTimeout)
	}
	if retryPolicy != nil {
		attrs["retry_policy"] = retryPolicy
	}
	data, _ := json.Marshal(attrs)
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK,
		Attributes: data,
	}
}

// scheduleActivityCmdWithScheduleTimeouts creates a ScheduleActivityTask command with schedule timeouts.
func scheduleActivityCmdWithScheduleTimeouts(seqID int64, actType string, input json.RawMessage, stcTimeout time.Duration, schedCloseTimeout time.Duration, schedStartTimeout time.Duration, retryPolicy map[string]any) *apiv1.Command {
	attrs := map[string]any{
		"seq_id":        seqID,
		"activity_type": actType,
		"input":         input,
	}
	if stcTimeout > 0 {
		attrs["start_to_close_timeout"] = int64(stcTimeout)
	}
	if schedCloseTimeout > 0 {
		attrs["schedule_to_close_timeout"] = int64(schedCloseTimeout)
	}
	if schedStartTimeout > 0 {
		attrs["schedule_to_start_timeout"] = int64(schedStartTimeout)
	}
	if retryPolicy != nil {
		attrs["retry_policy"] = retryPolicy
	}
	data, _ := json.Marshal(attrs)
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK,
		Attributes: data,
	}
}

// startChildWorkflowCmd creates a StartChildWorkflow command.
func startChildWorkflowCmd(seqID int64, workflowType string, taskQueue string, input json.RawMessage) *apiv1.Command {
	attrs := map[string]any{
		"seq_id":        seqID,
		"workflow_type": workflowType,
		"input":         input,
	}
	if taskQueue != "" {
		attrs["task_queue"] = taskQueue
	}
	data, _ := json.Marshal(attrs)
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_START_CHILD_WORKFLOW,
		Attributes: data,
	}
}

// continueAsNewCmd creates a ContinueAsNew command.
func continueAsNewCmd(input json.RawMessage, workflowType string, taskQueue string) *apiv1.Command {
	attrs := map[string]any{
		"input": input,
	}
	if workflowType != "" {
		attrs["workflow_type"] = workflowType
	}
	if taskQueue != "" {
		attrs["task_queue"] = taskQueue
	}
	data, _ := json.Marshal(attrs)
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_CONTINUE_AS_NEW,
		Attributes: data,
	}
}

// cancelTimerCmd creates a CancelTimer command.
func cancelTimerCmd(seqID int64) *apiv1.Command {
	attrs, _ := json.Marshal(map[string]any{
		"seq_id": seqID,
	})
	return &apiv1.Command{
		Type:       apiv1.CommandType_COMMAND_TYPE_CANCEL_TIMER,
		Attributes: attrs,
	}
}
