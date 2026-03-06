package domain

import (
	"time"

	"github.com/google/uuid"
)

type Timer struct {
	ID         int64
	Namespace  string
	WorkflowID uuid.UUID
	SeqID      int64
	FireAt     time.Time
	Status     TaskStatus
	CreatedAt  time.Time
}
