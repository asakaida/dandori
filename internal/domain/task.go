package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "PENDING"
	TaskStatusRunning   TaskStatus = "RUNNING"
	TaskStatusCompleted TaskStatus = "COMPLETED"
)

type WorkflowTask struct {
	ID          int64
	QueueName   string
	WorkflowID  uuid.UUID
	Status      TaskStatus
	ScheduledAt time.Time
}

type ActivityTask struct {
	ID                  int64
	QueueName           string
	WorkflowID          uuid.UUID
	ActivityType        string
	ActivityInput       json.RawMessage
	ActivitySeqID       int64
	StartToCloseTimeout time.Duration
	Attempt             int
	MaxAttempts         int
	RetryPolicy         *RetryPolicy
	Status              TaskStatus
	ScheduledAt         time.Time
	TimeoutAt           *time.Time
	HeartbeatAt              *time.Time
	HeartbeatTimeout         time.Duration
	ScheduleToCloseTimeout   time.Duration
	ScheduleToCloseTimeoutAt *time.Time
	ScheduleToStartTimeout   time.Duration
	ScheduleToStartTimeoutAt *time.Time
}

type ActivityFailure struct {
	Message      string
	Type         string
	NonRetryable bool
}
