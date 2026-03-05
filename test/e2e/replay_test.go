package e2e_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_WorkerRestartReplay(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "ReplayWorkflow",
		TaskQueue:    "replay-queue",
		Input:        []byte(`{"data":"test"}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// First WT: schedule activity
	wt1 := pollWorkflowTaskUntil(t, ctx, "replay-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "ReplayStep", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Complete activity
	at := pollActivityTaskUntil(t, ctx, "replay-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"step":"done"}`),
	})
	require.NoError(t, err)

	// Second WT arrives (triggered by activity completion) — poll it but DON'T complete (simulate worker crash)
	wt2 := pollWorkflowTaskUntil(t, ctx, "replay-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())
	// Worker "crashes" here — task remains locked

	// Expire the lock manually to simulate lock timeout
	_, err = testDB.ExecContext(ctx, `UPDATE workflow_tasks SET locked_until = NOW() - INTERVAL '1 second' WHERE status = 'RUNNING'`)
	require.NoError(t, err)

	// Wait for RunTaskRecovery (2s interval) to recover the stale task
	// The recovered task will be re-enqueued as PENDING
	time.Sleep(4 * time.Second)

	// A different worker picks up the recovered task (replay)
	wt3 := pollWorkflowTaskUntil(t, ctx, "replay-queue", "worker-2", 10*time.Second)
	require.NotZero(t, wt3.GetTaskId())

	// Verify the replay contains full history: Started, ActivityScheduled, ActivityCompleted
	events := wt3.GetEvents()
	assert.NotNil(t, findEvent(events, "WorkflowExecutionStarted"), "replay should include WorkflowExecutionStarted")
	assert.NotNil(t, findEvent(events, "ActivityTaskScheduled"), "replay should include ActivityTaskScheduled")
	assert.NotNil(t, findEvent(events, "ActivityTaskCompleted"), "replay should include ActivityTaskCompleted")

	// Complete the workflow (worker-2 succeeds where worker-1 crashed)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt3.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"replay":"success"}`))},
	})
	require.NoError(t, err)

	// Verify COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}
