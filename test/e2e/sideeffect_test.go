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

func TestE2E_SideEffect(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SideEffectWorkflow",
		TaskQueue:    "sideeffect-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT
	wt := pollWorkflowTaskUntil(t, ctx, "sideeffect-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt.GetTaskId())

	// Complete with RecordSideEffect + CompleteWorkflow
	sideEffectAttrs, _ := json.Marshal(map[string]any{
		"seq_id": 1,
		"value":  "random-42",
	})
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			{
				Type:       apiv1.CommandType_COMMAND_TYPE_RECORD_SIDE_EFFECT,
				Attributes: sideEffectAttrs,
			},
			completeWorkflowCmd(json.RawMessage(`{"done":true}`)),
		},
	})
	require.NoError(t, err)

	// Verify COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())

	// Verify history contains SideEffectRecorded
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)

	seEvent := findEvent(hist.GetEvents(), "SideEffectRecorded")
	require.NotNil(t, seEvent, "expected SideEffectRecorded event in history")

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(seEvent.GetEventData(), &data))
	assert.JSONEq(t, `"random-42"`, string(data["value"]))
}
