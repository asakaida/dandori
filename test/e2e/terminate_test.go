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

func TestE2E_TerminateRunningWorkflow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TermWorkflow",
		TaskQueue:    "term-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll and schedule an activity
	wt := pollWorkflowTaskUntil(t, ctx, "term-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "LongActivity", json.RawMessage(`{}`), 60*time.Second, nil)},
	})
	require.NoError(t, err)

	// Terminate
	_, err = client.TerminateWorkflow(ctx, &apiv1.TerminateWorkflowRequest{
		WorkflowId: wfID,
		Reason:     "operator requested termination",
	})
	require.NoError(t, err)

	// Verify TERMINATED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED, desc.GetWorkflowExecution().GetStatus())

	// Verify history contains termination event
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.NotNil(t, findEvent(hist.GetEvents(), "WorkflowExecutionTerminated"))
}

func TestE2E_TerminatedActivityResultDiscarded(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TermDiscardWorkflow",
		TaskQueue:    "term-discard-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule Activity
	wt := pollWorkflowTaskUntil(t, ctx, "term-discard-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "SlowActivity", json.RawMessage(`{}`), 60*time.Second, nil)},
	})
	require.NoError(t, err)

	// Poll AT (get the task but don't complete yet)
	at := pollActivityTaskUntil(t, ctx, "term-discard-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())

	// Terminate workflow while activity is in progress
	_, err = client.TerminateWorkflow(ctx, &apiv1.TerminateWorkflowRequest{
		WorkflowId: wfID,
		Reason:     "terminate during activity",
	})
	require.NoError(t, err)

	// Complete activity task after termination (should succeed without error but result is discarded)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"late":"result"}`),
	})
	// The completion may succeed or return an error depending on implementation.
	// Either way, the workflow should remain TERMINATED.

	// Verify still TERMINATED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED, desc.GetWorkflowExecution().GetStatus())

	// History should NOT contain ActivityTaskCompleted
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Nil(t, findEvent(hist.GetEvents(), "ActivityTaskCompleted"), "ActivityTaskCompleted should not appear after termination")

	// Poll WT should return empty (no new workflow task for terminated workflow)
	pollResp, err := client.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "term-discard-queue",
		WorkerId:  "worker-1",
	})
	require.NoError(t, err)
	assert.Zero(t, pollResp.GetTaskId(), "expected empty poll response for terminated workflow")
}
