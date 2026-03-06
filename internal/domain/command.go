package domain

import (
	"encoding/json"
	"time"
)

type CommandType string

const (
	CommandScheduleActivityTask CommandType = "ScheduleActivityTask"
	CommandCompleteWorkflow     CommandType = "CompleteWorkflow"
	CommandFailWorkflow         CommandType = "FailWorkflow"
	CommandStartTimer           CommandType = "StartTimer"
	CommandCancelTimer          CommandType = "CancelTimer"
	CommandStartChildWorkflow   CommandType = "StartChildWorkflow"
	CommandRecordSideEffect    CommandType = "RecordSideEffect"
)

type Command struct {
	Type       CommandType
	Attributes json.RawMessage
	Metadata   map[string]string
}

type ScheduleActivityTaskAttributes struct {
	SeqID               int64           `json:"seq_id"`
	ActivityType        string          `json:"activity_type"`
	Input               json.RawMessage `json:"input"`
	TaskQueue           string          `json:"task_queue,omitempty"`
	StartToCloseTimeout time.Duration   `json:"start_to_close_timeout"`
	RetryPolicy         *RetryPolicy    `json:"retry_policy,omitempty"`
	HeartbeatTimeout       time.Duration `json:"heartbeat_timeout,omitempty"`
	ScheduleToCloseTimeout time.Duration `json:"schedule_to_close_timeout,omitempty"`
	ScheduleToStartTimeout time.Duration `json:"schedule_to_start_timeout,omitempty"`
}

type CompleteWorkflowAttributes struct {
	Result json.RawMessage `json:"result"`
}

type FailWorkflowAttributes struct {
	ErrorMessage string `json:"error_message"`
}

type StartTimerAttributes struct {
	SeqID    int64         `json:"seq_id"`
	Duration time.Duration `json:"duration"`
}

type CancelTimerAttributes struct {
	SeqID int64 `json:"seq_id"`
}

type StartChildWorkflowAttributes struct {
	SeqID        int64           `json:"seq_id"`
	WorkflowID   string          `json:"workflow_id,omitempty"`
	WorkflowType string          `json:"workflow_type"`
	TaskQueue    string          `json:"task_queue,omitempty"`
	Input        json.RawMessage `json:"input"`
}

type RecordSideEffectAttributes struct {
	SeqID int64           `json:"seq_id"`
	Value json.RawMessage `json:"value"`
}
