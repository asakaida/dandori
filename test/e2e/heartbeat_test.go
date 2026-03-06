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

func TestE2E_HeartbeatTimeout(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HeartbeatWorkflow",
		TaskQueue:    "heartbeat-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT and schedule activity with short heartbeat_timeout
	wt := pollWorkflowTaskUntil(t, ctx, "heartbeat-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmdWithHeartbeat(1, "long-task", json.RawMessage(`{}`), 0, 1*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Poll activity task but do NOT send heartbeats
	at := pollActivityTaskUntil(t, ctx, "heartbeat-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())

	// Wait for heartbeat timeout to be detected
	wt2 := pollWorkflowTaskUntil(t, ctx, "heartbeat-queue", "worker-1", 10*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Verify ActivityTaskTimedOut event
	var hasTimedOut bool
	for _, e := range wt2.GetEvents() {
		if e.GetEventType() == "ActivityTaskTimedOut" {
			hasTimedOut = true
		}
	}
	assert.True(t, hasTimedOut, "expected ActivityTaskTimedOut event")

	// Complete workflow
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"timeout":true}`))},
	})
	require.NoError(t, err)

	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}

func TestE2E_HeartbeatKeepalive(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HeartbeatKeepaliveWorkflow",
		TaskQueue:    "hb-keepalive-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT and schedule activity with heartbeat_timeout
	wt := pollWorkflowTaskUntil(t, ctx, "hb-keepalive-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmdWithHeartbeat(1, "keepalive-task", json.RawMessage(`{}`), 0, 2*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Poll activity task and send heartbeats
	at := pollActivityTaskUntil(t, ctx, "hb-keepalive-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())

	// Send heartbeats for 3 seconds (longer than 2s timeout) to prove keepalive works
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
heartbeatLoop:
	for {
		select {
		case <-deadline:
			break heartbeatLoop
		case <-ticker.C:
			_, err := client.RecordActivityHeartbeat(ctx, &apiv1.RecordActivityHeartbeatRequest{
				TaskId:  at.GetTaskId(),
				Details: []byte(`{"progress":"working"}`),
			})
			require.NoError(t, err)
		}
	}

	// Complete the activity
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"done":true}`),
	})
	require.NoError(t, err)

	// Poll WT and complete workflow
	wt2 := pollWorkflowTaskUntil(t, ctx, "hb-keepalive-queue", "worker-1", 5*time.Second)

	// Verify NO timeout event (activity completed normally thanks to heartbeats)
	var hasTimedOut bool
	for _, e := range wt2.GetEvents() {
		if e.GetEventType() == "ActivityTaskTimedOut" {
			hasTimedOut = true
		}
	}
	assert.False(t, hasTimedOut, "should NOT have ActivityTaskTimedOut event when heartbeats are sent")

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"success":true}`))},
	})
	require.NoError(t, err)

	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}
