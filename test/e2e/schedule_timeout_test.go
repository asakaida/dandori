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

func TestE2E_ScheduleToCloseTimeout(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SchedCloseWorkflow",
		TaskQueue:    "sched-close-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with ScheduleToCloseTimeout=2s
	wt1 := pollWorkflowTaskUntil(t, ctx, "sched-close-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt1.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmdWithScheduleTimeouts(1, "SlowActivity", json.RawMessage(`{}`), 0, 2*time.Second, 0, nil),
		},
	})
	require.NoError(t, err)

	// Poll AT to start it, but do NOT complete
	at := pollActivityTaskUntil(t, ctx, "sched-close-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())

	// Wait for timeout to fire
	wt2 := pollWorkflowTaskUntil(t, ctx, "sched-close-queue", "worker-1", 10*time.Second)
	require.NotZero(t, wt2.GetTaskId())
	assert.NotNil(t, findEvent(wt2.GetEvents(), "ActivityTaskTimedOut"))

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("schedule-to-close timeout")},
	})
	require.NoError(t, err)

	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())
}

func TestE2E_ScheduleToStartTimeout(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SchedStartWorkflow",
		TaskQueue:    "sched-start-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with ScheduleToStartTimeout=1s
	wt1 := pollWorkflowTaskUntil(t, ctx, "sched-start-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt1.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmdWithScheduleTimeouts(1, "NeverPolled", json.RawMessage(`{}`), 0, 0, 1*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Do NOT poll AT — let schedule-to-start timeout fire
	wt2 := pollWorkflowTaskUntil(t, ctx, "sched-start-queue", "worker-1", 10*time.Second)
	require.NotZero(t, wt2.GetTaskId())
	assert.NotNil(t, findEvent(wt2.GetEvents(), "ActivityTaskTimedOut"))

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("schedule-to-start timeout")},
	})
	require.NoError(t, err)

	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())
}

func TestE2E_ScheduleToStartTimeout_ClearedOnPoll(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SchedStartClearWorkflow",
		TaskQueue:    "sched-start-clear-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with ScheduleToStartTimeout=1s
	wt1 := pollWorkflowTaskUntil(t, ctx, "sched-start-clear-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt1.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmdWithScheduleTimeouts(1, "QuickActivity", json.RawMessage(`{}`), 0, 0, 1*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Poll AT immediately — this clears schedule_to_start_timeout_at
	at := pollActivityTaskUntil(t, ctx, "sched-start-clear-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())

	// Wait 2s — no timeout should fire
	time.Sleep(2 * time.Second)

	// Complete the activity
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"ok":true}`),
	})
	require.NoError(t, err)

	// Poll the WT triggered by activity completion
	wt2 := pollWorkflowTaskUntil(t, ctx, "sched-start-clear-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Verify no timeout event
	assert.Nil(t, findEvent(wt2.GetEvents(), "ActivityTaskTimedOut"), "should have no timeout event")
	assert.NotNil(t, findEvent(wt2.GetEvents(), "ActivityTaskCompleted"), "should have completed event")

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"done":true}`))},
	})
	require.NoError(t, err)

	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}
