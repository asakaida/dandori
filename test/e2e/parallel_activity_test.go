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

func TestE2E_ParallelActivity(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "ParallelWorkflow",
		TaskQueue:    "parallel-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT and schedule 3 activities in a single CompleteWorkflowTask
	wt := pollWorkflowTaskUntil(t, ctx, "parallel-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmd(1, "task-a", json.RawMessage(`{"name":"A"}`), 30*time.Second, nil),
			scheduleActivityCmd(2, "task-b", json.RawMessage(`{"name":"B"}`), 30*time.Second, nil),
			scheduleActivityCmd(3, "task-c", json.RawMessage(`{"name":"C"}`), 30*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Poll and complete all 3 activity tasks independently
	completedActivities := make(map[string]bool)
	for i := 0; i < 3; i++ {
		at := pollActivityTaskUntil(t, ctx, "parallel-queue", "worker-1", 5*time.Second)
		require.NotZero(t, at.GetTaskId())
		completedActivities[at.GetActivityType()] = true

		_, err := client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
			TaskId: at.GetTaskId(),
			Result: []byte(`{"ok":true}`),
		})
		require.NoError(t, err)
	}

	assert.True(t, completedActivities["task-a"], "task-a should be completed")
	assert.True(t, completedActivities["task-b"], "task-b should be completed")
	assert.True(t, completedActivities["task-c"], "task-c should be completed")

	// Drain all 3 workflow tasks (one per activity completion) and complete the last one
	for i := 0; i < 3; i++ {
		wt := pollWorkflowTaskUntil(t, ctx, "parallel-queue", "worker-1", 5*time.Second)
		if i < 2 {
			// Not all activities done yet, just complete WT with no commands
			_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
				TaskId: wt.GetTaskId(),
			})
			require.NoError(t, err)
		} else {
			// All 3 activities completed, complete workflow
			_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
				TaskId:   wt.GetTaskId(),
				Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"all_done":true}`))},
			})
			require.NoError(t, err)
		}
	}

	// Verify COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())

	// Verify history has 3 ActivityTaskScheduled and 3 ActivityTaskCompleted
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, 3, countEvents(hist.GetEvents(), "ActivityTaskScheduled"))
	assert.Equal(t, 3, countEvents(hist.GetEvents(), "ActivityTaskCompleted"))
}
