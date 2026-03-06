package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WorkflowStatus string

const (
	WorkflowStatusRunning    WorkflowStatus = "RUNNING"
	WorkflowStatusCompleted  WorkflowStatus = "COMPLETED"
	WorkflowStatusFailed     WorkflowStatus = "FAILED"
	WorkflowStatusTerminated      WorkflowStatus = "TERMINATED"
	WorkflowStatusContinuedAsNew  WorkflowStatus = "CONTINUED_AS_NEW"
)

func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusCompleted || s == WorkflowStatusFailed || s == WorkflowStatusTerminated || s == WorkflowStatusContinuedAsNew
}

type WorkflowExecution struct {
	ID               uuid.UUID
	Namespace        string
	WorkflowType     string
	TaskQueue        string
	Status           WorkflowStatus
	Input            json.RawMessage
	Result           json.RawMessage
	Error            string
	CreatedAt        time.Time
	ClosedAt         *time.Time
	ParentWorkflowID *uuid.UUID
	ParentSeqID      int64
	CronSchedule     string
	ContinuedAsNewID *uuid.UUID
}
