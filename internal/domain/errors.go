package domain

import "errors"

var (
	ErrWorkflowNotFound      = errors.New("workflow not found")
	ErrWorkflowAlreadyExists = errors.New("workflow already exists")
	ErrWorkflowNotRunning    = errors.New("workflow is not in running state")
	ErrTaskNotFound          = errors.New("task not found")
	ErrTaskAlreadyCompleted  = errors.New("task already completed")
	ErrNoTaskAvailable       = errors.New("no task available")
	ErrQueryNotFound         = errors.New("query not found")
	ErrQueryTimedOut         = errors.New("query timed out")
)
