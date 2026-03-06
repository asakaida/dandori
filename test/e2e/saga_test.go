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

func TestE2E_SagaCompensation(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SagaWorkflow",
		TaskQueue:    "saga-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Step 1: book-flight (success)
	wt := pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "book-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at := pollActivityTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "book-flight", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"confirmation":"FL-001"}`),
	})
	require.NoError(t, err)

	// Step 2: book-hotel (success)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(2, "book-hotel", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "book-hotel", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"confirmation":"HT-001"}`),
	})
	require.NoError(t, err)

	// Step 3: book-car (failure -> triggers compensation)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(3, "book-car", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "book-car", at.GetActivityType())
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "no cars available",
			Type:         "BookingError",
			NonRetryable: true,
		},
	})
	require.NoError(t, err)

	// Compensation 1: cancel-hotel (reverse order)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(4, "cancel-hotel", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "cancel-hotel", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"cancelled":true}`),
	})
	require.NoError(t, err)

	// Compensation 2: cancel-flight (reverse order)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(5, "cancel-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "cancel-flight", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"cancelled":true}`),
	})
	require.NoError(t, err)

	// Final: FailWorkflow
	wt = pollWorkflowTaskUntil(t, ctx, "saga-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("saga compensation completed")},
	})
	require.NoError(t, err)

	// Verify: workflow is FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())

	// Verify: event history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	events := hist.GetEvents()

	// Started + 5*Scheduled + 4*Completed + 1*Failed(activity) + 1*Failed(workflow) = 12
	assert.Equal(t, 12, len(events), "expected 12 history events, got %d", len(events))
	assert.Equal(t, 5, countEvents(events, "ActivityTaskScheduled"))
	assert.Equal(t, 4, countEvents(events, "ActivityTaskCompleted"))
	assert.Equal(t, 1, countEvents(events, "ActivityTaskFailed"))
	assert.NotNil(t, findEvent(events, "WorkflowExecutionFailed"))

	// Verify activity execution order via scheduled events
	scheduledEvents := filterEvents(events, "ActivityTaskScheduled")
	require.Len(t, scheduledEvents, 5)
	expectedOrder := []string{"book-flight", "book-hotel", "book-car", "cancel-hotel", "cancel-flight"}
	for i, e := range scheduledEvents {
		var data map[string]any
		require.NoError(t, json.Unmarshal(e.GetEventData(), &data))
		assert.Equal(t, expectedOrder[i], data["activity_type"], "activity at position %d", i)
	}
}

func TestE2E_SagaContinueWithError(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SagaContinueWorkflow",
		TaskQueue:    "saga-err-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Step 1: book-flight (success)
	wt := pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "book-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at := pollActivityTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"confirmation":"FL-001"}`),
	})
	require.NoError(t, err)

	// Step 2: book-hotel (success)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(2, "book-hotel", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"confirmation":"HT-001"}`),
	})
	require.NoError(t, err)

	// Step 3: book-car (failure -> triggers compensation)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(3, "book-car", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "no cars available",
			Type:         "BookingError",
			NonRetryable: true,
		},
	})
	require.NoError(t, err)

	// Compensation 1: cancel-hotel (FAILS - but compensation continues)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(4, "cancel-hotel", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "cancel-hotel", at.GetActivityType())
	_, err = client.FailActivityTask(ctx, &apiv1.FailActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Failure: &apiv1.ActivityFailure{
			Message:      "cancel service unavailable",
			Type:         "ServiceError",
			NonRetryable: true,
		},
	})
	require.NoError(t, err)

	// Compensation 2: cancel-flight (success - continues despite previous compensation failure)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(5, "cancel-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "cancel-flight", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"cancelled":true}`),
	})
	require.NoError(t, err)

	// Final: FailWorkflow
	wt = pollWorkflowTaskUntil(t, ctx, "saga-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("saga compensation completed with errors")},
	})
	require.NoError(t, err)

	// Verify: workflow is FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())

	// Verify: event history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	events := hist.GetEvents()

	// Started + 5*Scheduled + 3*Completed + 2*Failed(activity: book-car + cancel-hotel) + 1*Failed(workflow) = 12
	assert.Equal(t, 12, len(events), "expected 12 history events, got %d", len(events))
	assert.Equal(t, 5, countEvents(events, "ActivityTaskScheduled"))
	assert.Equal(t, 3, countEvents(events, "ActivityTaskCompleted"))
	assert.Equal(t, 2, countEvents(events, "ActivityTaskFailed"))
	assert.NotNil(t, findEvent(events, "WorkflowExecutionFailed"))
}

func TestE2E_SagaWithMetadata(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SagaMetadataWorkflow",
		TaskQueue:    "saga-meta-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Step 1: book-flight (success, no metadata)
	wt := pollWorkflowTaskUntil(t, ctx, "saga-meta-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "book-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	at := pollActivityTaskUntil(t, ctx, "saga-meta-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"confirmation":"FL-001"}`),
	})
	require.NoError(t, err)

	// Compensation: cancel-flight with metadata (saga_compensating=true)
	wt = pollWorkflowTaskUntil(t, ctx, "saga-meta-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(2, "cancel-flight", json.RawMessage(`{}`), 30*time.Second, nil)},
		Metadata: map[string]string{"saga_compensating": "true"},
	})
	require.NoError(t, err)

	at = pollActivityTaskUntil(t, ctx, "saga-meta-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "cancel-flight", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"cancelled":true}`),
	})
	require.NoError(t, err)

	// Final: FailWorkflow with metadata
	wt = pollWorkflowTaskUntil(t, ctx, "saga-meta-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("saga compensation completed")},
		Metadata: map[string]string{"saga_compensating": "true"},
	})
	require.NoError(t, err)

	// Verify: workflow is FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())

	// Verify: metadata is stored in event_data
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	events := hist.GetEvents()

	// Check that cancel-flight's ActivityTaskScheduled event has metadata
	scheduledEvents := filterEvents(events, "ActivityTaskScheduled")
	require.Len(t, scheduledEvents, 2) // book-flight + cancel-flight

	// book-flight (no metadata)
	var bookData map[string]any
	require.NoError(t, json.Unmarshal(scheduledEvents[0].GetEventData(), &bookData))
	assert.Nil(t, bookData["metadata"], "book-flight should have no metadata")

	// cancel-flight (with metadata)
	var cancelData map[string]any
	require.NoError(t, json.Unmarshal(scheduledEvents[1].GetEventData(), &cancelData))
	require.NotNil(t, cancelData["metadata"], "cancel-flight should have metadata")
	md, ok := cancelData["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "true", md["saga_compensating"])

	// Check that WorkflowExecutionFailed event also has metadata
	failedEvent := findEvent(events, "WorkflowExecutionFailed")
	require.NotNil(t, failedEvent)
	var failedData map[string]any
	require.NoError(t, json.Unmarshal(failedEvent.GetEventData(), &failedData))
	require.NotNil(t, failedData["metadata"], "WorkflowExecutionFailed should have metadata")
	failedMd, ok := failedData["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "true", failedMd["saga_compensating"])
}

// filterEvents returns all events matching the given event type, preserving order.
func filterEvents(events []*apiv1.HistoryEvent, eventType string) []*apiv1.HistoryEvent {
	var result []*apiv1.HistoryEvent
	for _, e := range events {
		if e.GetEventType() == eventType {
			result = append(result, e)
		}
	}
	return result
}
