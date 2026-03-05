package e2e_test

import (
	"context"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_TimerFire(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	queue := "timer-fire-q"

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TimerWF",
		TaskQueue:    queue,
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial workflow task and start a 1s timer
	wt := pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{startTimerCmd(1, 1*time.Second)},
	})
	require.NoError(t, err)

	// Wait for timer poller to fire the timer and enqueue a new workflow task
	wt = pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)

	// Complete the workflow
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd([]byte(`"done"`))},
	})
	require.NoError(t, err)

	// Verify history
	histResp, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)

	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerStarted"))
	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerFired"))
}

func TestE2E_TimerCancel(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	queue := "timer-cancel-q"

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TimerCancelWF",
		TaskQueue:    queue,
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial workflow task: start a long timer + schedule an activity
	wt := pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			startTimerCmd(1, 10*time.Second),
			scheduleActivityCmd(2, "fast-activity", []byte(`{}`), 5*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Complete the activity
	at := pollActivityTaskUntil(t, ctx, queue, "w1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`"ok"`),
	})
	require.NoError(t, err)

	// Poll workflow task triggered by activity completion, then cancel timer + complete
	wt = pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			cancelTimerCmd(1),
			completeWorkflowCmd([]byte(`"done"`)),
		},
	})
	require.NoError(t, err)

	// Verify history
	histResp, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)

	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerStarted"))
	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerCanceled"))
	assert.Equal(t, 0, countEvents(histResp.GetEvents(), "TimerFired"))
}

func TestE2E_TimerCancel_AlreadyFired(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	queue := "timer-cancel-fired-q"

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TimerCancelFiredWF",
		TaskQueue:    queue,
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial workflow task and start a 1s timer
	wt := pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{startTimerCmd(1, 1*time.Second)},
	})
	require.NoError(t, err)

	// Wait for timer to fire
	wt = pollWorkflowTaskUntil(t, ctx, queue, "w1", 5*time.Second)

	// Try to cancel the already-fired timer + complete workflow
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			cancelTimerCmd(1),
			completeWorkflowCmd([]byte(`"done"`)),
		},
	})
	require.NoError(t, err)

	// Verify history
	histResp, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)

	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerStarted"))
	assert.Equal(t, 1, countEvents(histResp.GetEvents(), "TimerFired"))
	assert.Equal(t, 0, countEvents(histResp.GetEvents(), "TimerCanceled"))
}
