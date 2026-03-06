package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	apiv1.UnimplementedDandoriServiceServer
	client  port.ClientService
	wfTask  port.WorkflowTaskService
	actTask port.ActivityTaskService
}

func NewHandler(client port.ClientService, wfTask port.WorkflowTaskService, actTask port.ActivityTaskService) *Handler {
	return &Handler{
		client:  client,
		wfTask:  wfTask,
		actTask: actTask,
	}
}

// --- Client API ---

func (h *Handler) StartWorkflow(ctx context.Context, req *apiv1.StartWorkflowRequest) (*apiv1.StartWorkflowResponse, error) {
	var wfID uuid.UUID
	if req.GetWorkflowId() != "" {
		var err error
		wfID, err = uuid.Parse(req.GetWorkflowId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
		}
	}

	wf, err := h.client.StartWorkflow(ctx, port.StartWorkflowParams{
		ID:           wfID,
		WorkflowType: req.GetWorkflowType(),
		TaskQueue:    req.GetTaskQueue(),
		Input:        json.RawMessage(req.GetInput()),
	})
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.StartWorkflowResponse{WorkflowId: wf.ID.String()}, nil
}

func (h *Handler) DescribeWorkflow(ctx context.Context, req *apiv1.DescribeWorkflowRequest) (*apiv1.DescribeWorkflowResponse, error) {
	id, err := uuid.Parse(req.GetWorkflowId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
	}

	wf, err := h.client.DescribeWorkflow(ctx, id)
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.DescribeWorkflowResponse{WorkflowExecution: domainWorkflowToProto(wf)}, nil
}

func (h *Handler) GetWorkflowHistory(ctx context.Context, req *apiv1.GetWorkflowHistoryRequest) (*apiv1.GetWorkflowHistoryResponse, error) {
	id, err := uuid.Parse(req.GetWorkflowId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
	}

	events, err := h.client.GetWorkflowHistory(ctx, id)
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.GetWorkflowHistoryResponse{Events: domainEventsToProto(events)}, nil
}

func (h *Handler) TerminateWorkflow(ctx context.Context, req *apiv1.TerminateWorkflowRequest) (*apiv1.TerminateWorkflowResponse, error) {
	id, err := uuid.Parse(req.GetWorkflowId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
	}

	if err := h.client.TerminateWorkflow(ctx, id, req.GetReason()); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.TerminateWorkflowResponse{}, nil
}

func (h *Handler) SignalWorkflow(ctx context.Context, req *apiv1.SignalWorkflowRequest) (*apiv1.SignalWorkflowResponse, error) {
	id, err := uuid.Parse(req.GetWorkflowId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
	}

	if err := h.client.SignalWorkflow(ctx, id, req.GetSignalName(), json.RawMessage(req.GetInput())); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.SignalWorkflowResponse{}, nil
}

func (h *Handler) ListWorkflows(ctx context.Context, req *apiv1.ListWorkflowsRequest) (*apiv1.ListWorkflowsResponse, error) {
	params := port.ListWorkflowsParams{
		PageSize:     int(req.GetPageSize()),
		StatusFilter: req.GetStatusFilter(),
		TypeFilter:   req.GetTypeFilter(),
		QueueFilter:  req.GetQueueFilter(),
	}

	if token := req.GetNextPageToken(); token != "" {
		cursor, err := decodeCursor(token)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid next_page_token: %v", err)
		}
		params.Cursor = cursor
	}

	result, err := h.client.ListWorkflows(ctx, params)
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}

	pbWorkflows := make([]*apiv1.WorkflowExecution, len(result.Workflows))
	for i := range result.Workflows {
		pbWorkflows[i] = domainWorkflowToProto(&result.Workflows[i])
	}

	resp := &apiv1.ListWorkflowsResponse{Workflows: pbWorkflows}
	if result.NextCursor != nil {
		resp.NextPageToken = encodeCursor(result.NextCursor)
	}
	return resp, nil
}

func (h *Handler) CancelWorkflow(ctx context.Context, req *apiv1.CancelWorkflowRequest) (*apiv1.CancelWorkflowResponse, error) {
	id, err := uuid.Parse(req.GetWorkflowId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
	}

	if err := h.client.CancelWorkflow(ctx, id); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.CancelWorkflowResponse{}, nil
}

// --- Workflow Task API ---

func (h *Handler) PollWorkflowTask(ctx context.Context, req *apiv1.PollWorkflowTaskRequest) (*apiv1.PollWorkflowTaskResponse, error) {
	result, err := h.wfTask.PollWorkflowTask(ctx, req.GetQueueName(), req.GetWorkerId())
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}
	if result == nil {
		return &apiv1.PollWorkflowTaskResponse{}, nil
	}
	return &apiv1.PollWorkflowTaskResponse{
		TaskId:       result.Task.ID,
		WorkflowId:   result.Task.WorkflowID.String(),
		WorkflowType: result.WorkflowType,
		Events:       domainEventsToProto(result.Events),
	}, nil
}

func (h *Handler) CompleteWorkflowTask(ctx context.Context, req *apiv1.CompleteWorkflowTaskRequest) (*apiv1.CompleteWorkflowTaskResponse, error) {
	commands, err := protoCommandsToDomain(req.GetCommands())
	if err != nil {
		return nil, err
	}
	if md := req.GetMetadata(); len(md) > 0 {
		for i := range commands {
			commands[i].Metadata = md
		}
	}
	if err := h.wfTask.CompleteWorkflowTask(ctx, req.GetTaskId(), commands); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.CompleteWorkflowTaskResponse{}, nil
}

func (h *Handler) FailWorkflowTask(ctx context.Context, req *apiv1.FailWorkflowTaskRequest) (*apiv1.FailWorkflowTaskResponse, error) {
	if err := h.wfTask.FailWorkflowTask(ctx, req.GetTaskId(), req.GetCause(), req.GetMessage()); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.FailWorkflowTaskResponse{}, nil
}

// --- Activity Task API ---

func (h *Handler) PollActivityTask(ctx context.Context, req *apiv1.PollActivityTaskRequest) (*apiv1.PollActivityTaskResponse, error) {
	task, err := h.actTask.PollActivityTask(ctx, req.GetQueueName(), req.GetWorkerId())
	if err != nil {
		return nil, domainErrorToGRPC(err)
	}
	if task == nil {
		return &apiv1.PollActivityTaskResponse{}, nil
	}
	return &apiv1.PollActivityTaskResponse{
		TaskId:        task.ID,
		WorkflowId:    task.WorkflowID.String(),
		ActivityType:  task.ActivityType,
		ActivityInput: task.ActivityInput,
		Attempt:       int32(task.Attempt),
		ScheduledAt:   timestamppb.New(task.ScheduledAt),
	}, nil
}

func (h *Handler) CompleteActivityTask(ctx context.Context, req *apiv1.CompleteActivityTaskRequest) (*apiv1.CompleteActivityTaskResponse, error) {
	if err := h.actTask.CompleteActivityTask(ctx, req.GetTaskId(), json.RawMessage(req.GetResult())); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.CompleteActivityTaskResponse{}, nil
}

func (h *Handler) FailActivityTask(ctx context.Context, req *apiv1.FailActivityTaskRequest) (*apiv1.FailActivityTaskResponse, error) {
	var failure domain.ActivityFailure
	if f := req.GetFailure(); f != nil {
		failure = domain.ActivityFailure{
			Message:      f.GetMessage(),
			Type:         f.GetType(),
			NonRetryable: f.GetNonRetryable(),
		}
	}
	if err := h.actTask.FailActivityTask(ctx, req.GetTaskId(), failure); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.FailActivityTaskResponse{}, nil
}

func (h *Handler) RecordActivityHeartbeat(ctx context.Context, req *apiv1.RecordActivityHeartbeatRequest) (*apiv1.RecordActivityHeartbeatResponse, error) {
	if err := h.actTask.RecordActivityHeartbeat(ctx, req.GetTaskId(), json.RawMessage(req.GetDetails())); err != nil {
		return nil, domainErrorToGRPC(err)
	}
	return &apiv1.RecordActivityHeartbeatResponse{}, nil
}

// --- Error conversion ---

func domainErrorToGRPC(err error) error {
	switch {
	case errors.Is(err, domain.ErrWorkflowNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrWorkflowAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrWorkflowNotRunning):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrTaskAlreadyCompleted):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// --- Type conversion helpers ---

func workflowStatusToProto(s domain.WorkflowStatus) apiv1.WorkflowExecutionStatus {
	switch s {
	case domain.WorkflowStatusRunning:
		return apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING
	case domain.WorkflowStatusCompleted:
		return apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED
	case domain.WorkflowStatusFailed:
		return apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED
	case domain.WorkflowStatusTerminated:
		return apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED
	default:
		return apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_UNSPECIFIED
	}
}

func domainWorkflowToProto(wf *domain.WorkflowExecution) *apiv1.WorkflowExecution {
	pb := &apiv1.WorkflowExecution{
		Id:           wf.ID.String(),
		WorkflowType: wf.WorkflowType,
		TaskQueue:    wf.TaskQueue,
		Status:       workflowStatusToProto(wf.Status),
		Input:        wf.Input,
		Result:       wf.Result,
		ErrorMessage: wf.Error,
		CreatedAt:    timestamppb.New(wf.CreatedAt),
	}
	if wf.ClosedAt != nil {
		pb.ClosedAt = timestamppb.New(*wf.ClosedAt)
	}
	return pb
}

func domainEventsToProto(events []domain.HistoryEvent) []*apiv1.HistoryEvent {
	pb := make([]*apiv1.HistoryEvent, len(events))
	for i, e := range events {
		pb[i] = &apiv1.HistoryEvent{
			Id:          e.ID,
			WorkflowId:  e.WorkflowID.String(),
			SequenceNum: int32(e.SequenceNum),
			EventType:   string(e.Type),
			EventData:   e.Data,
			Timestamp:   timestamppb.New(e.Timestamp),
		}
	}
	return pb
}

func protoCommandsToDomain(cmds []*apiv1.Command) ([]domain.Command, error) {
	result := make([]domain.Command, len(cmds))
	for i, c := range cmds {
		ct, err := commandTypeFromProto(c.GetType())
		if err != nil {
			return nil, err
		}
		result[i] = domain.Command{
			Type:       ct,
			Attributes: json.RawMessage(c.GetAttributes()),
		}
	}
	return result, nil
}

func encodeCursor(c *port.ListWorkflowsCursor) string {
	data, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(data)
}

func decodeCursor(token string) (*port.ListWorkflowsCursor, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var c port.ListWorkflowsCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func commandTypeFromProto(ct apiv1.CommandType) (domain.CommandType, error) {
	switch ct {
	case apiv1.CommandType_COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK:
		return domain.CommandScheduleActivityTask, nil
	case apiv1.CommandType_COMMAND_TYPE_COMPLETE_WORKFLOW:
		return domain.CommandCompleteWorkflow, nil
	case apiv1.CommandType_COMMAND_TYPE_FAIL_WORKFLOW:
		return domain.CommandFailWorkflow, nil
	case apiv1.CommandType_COMMAND_TYPE_START_TIMER:
		return domain.CommandStartTimer, nil
	case apiv1.CommandType_COMMAND_TYPE_CANCEL_TIMER:
		return domain.CommandCancelTimer, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "unknown command type: %v", ct)
	}
}
