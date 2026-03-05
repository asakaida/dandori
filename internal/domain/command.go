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
)

type Command struct {
	Type       CommandType
	Attributes json.RawMessage
}

type ScheduleActivityTaskAttributes struct {
	SeqID               int64           `json:"seq_id"`
	ActivityType        string          `json:"activity_type"`
	Input               json.RawMessage `json:"input"`
	TaskQueue           string          `json:"task_queue,omitempty"`
	StartToCloseTimeout time.Duration   `json:"start_to_close_timeout"`
	RetryPolicy         *RetryPolicy    `json:"retry_policy,omitempty"`
}

type CompleteWorkflowAttributes struct {
	Result json.RawMessage `json:"result"`
}

type FailWorkflowAttributes struct {
	ErrorMessage string `json:"error_message"`
}
