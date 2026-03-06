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
	WorkflowStatusTerminated WorkflowStatus = "TERMINATED"
)

func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusCompleted || s == WorkflowStatusFailed || s == WorkflowStatusTerminated
}

type WorkflowExecution struct {
	ID               uuid.UUID
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
}
