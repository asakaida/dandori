package domain

import "time"

type RetryPolicy struct {
	MaxAttempts        int           `json:"max_attempts"`
	InitialInterval    time.Duration `json:"initial_interval"`
	BackoffCoefficient float64       `json:"backoff_coefficient"`
	MaxInterval        time.Duration `json:"max_interval"`
}
