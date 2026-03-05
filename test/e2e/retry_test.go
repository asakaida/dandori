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

func TestE2E_ActivityRetry(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "RetryWorkflow",
		TaskQueue:    "retry-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with retry policy
	wt1 := pollWorkflowTaskUntil(t, ctx, "retry-queue", "worker-1", 5*time.Second)
	retryPolicy := map[string]any{
		"max_attempts":        3,
		"initial_interval":    int64(100 * time.Millisecond),
		"backoff_coefficient": 2.0,
	}
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "FlakyActivity", json.RawMessage(`{}`), 30*time.Second, retryPolicy)},
	})
	require.NoError(t, err)

	// Attempt 1: PollAT -> Fail (retryable)
	at1 := pollActivityTaskUntil(t, ctx, "retry-queue", "worker-1", 5*time.Second)
	assert.Equal(t, int32(1), at1.GetAttempt())
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at1.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "transient error",
			Type:         "TransientError",
			NonRetryable: false,
		},
	})
	require.NoError(t, err)

	// Attempt 2: PollAT -> Fail (retryable)
	at2 := pollActivityTaskUntil(t, ctx, "retry-queue", "worker-1", 10*time.Second)
	assert.Equal(t, int32(2), at2.GetAttempt())
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at2.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "transient error again",
			Type:         "TransientError",
			NonRetryable: false,
		},
	})
	require.NoError(t, err)

	// Attempt 3: PollAT -> Complete (success on last attempt)
	at3 := pollActivityTaskUntil(t, ctx, "retry-queue", "worker-1", 10*time.Second)
	assert.Equal(t, int32(3), at3.GetAttempt())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at3.GetTaskId(),
		Result: []byte(`{"recovered":true}`),
	})
	require.NoError(t, err)

	// Complete workflow
	wt2 := pollWorkflowTaskUntil(t, ctx, "retry-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"status":"ok"}`))},
	})
	require.NoError(t, err)

	// Verify COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}

func TestE2E_NonRetryableFailure(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "NonRetryWorkflow",
		TaskQueue:    "nonretry-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with retry policy (max_attempts=3)
	wt1 := pollWorkflowTaskUntil(t, ctx, "nonretry-queue", "worker-1", 5*time.Second)
	retryPolicy := map[string]any{
		"max_attempts":        3,
		"initial_interval":    int64(100 * time.Millisecond),
		"backoff_coefficient": 2.0,
	}
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "CriticalActivity", json.RawMessage(`{}`), 30*time.Second, retryPolicy)},
	})
	require.NoError(t, err)

	// Attempt 1: Fail with non_retryable=true (should NOT retry)
	at1 := pollActivityTaskUntil(t, ctx, "nonretry-queue", "worker-1", 5*time.Second)
	assert.Equal(t, int32(1), at1.GetAttempt())
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at1.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "permanent failure",
			Type:         "PermanentError",
			NonRetryable: true,
		},
	})
	require.NoError(t, err)

	// Should get a new WT with ActivityTaskFailed event (no retry)
	wt2 := pollWorkflowTaskUntil(t, ctx, "nonretry-queue", "worker-1", 10*time.Second)
	assert.NotNil(t, findEvent(wt2.GetEvents(), "ActivityTaskFailed"))

	// Fail the workflow
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("activity failed permanently")},
	})
	require.NoError(t, err)

	// Verify FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())
}
