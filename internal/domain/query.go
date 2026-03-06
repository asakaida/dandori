package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type QueryStatus string

const (
	QueryStatusPending  QueryStatus = "PENDING"
	QueryStatusAnswered QueryStatus = "ANSWERED"
)

type WorkflowQuery struct {
	ID           int64
	WorkflowID   uuid.UUID
	QueryType    string
	Input        json.RawMessage
	Result       json.RawMessage
	ErrorMessage string
	Status       QueryStatus
	CreatedAt    time.Time
	AnsweredAt   *time.Time
}
