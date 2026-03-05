package engine

import (
	"testing"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestComputeNextRetryTime_NilPolicy(t *testing.T) {
	task := &domain.ActivityTask{Attempt: 1}
	before := time.Now()
	result := computeNextRetryTime(task)
	assert.WithinDuration(t, before, result, time.Second)
}

func TestComputeNextRetryTime_FirstAttempt(t *testing.T) {
	task := &domain.ActivityTask{
		Attempt: 1,
		RetryPolicy: &domain.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaxInterval:        time.Minute,
		},
	}
	before := time.Now()
	result := computeNextRetryTime(task)
	expected := before.Add(time.Second)
	assert.WithinDuration(t, expected, result, 100*time.Millisecond)
}

func TestComputeNextRetryTime_ExponentialBackoff(t *testing.T) {
	task := &domain.ActivityTask{
		Attempt: 3,
		RetryPolicy: &domain.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaxInterval:        time.Minute,
		},
	}
	before := time.Now()
	result := computeNextRetryTime(task)
	// 1s * 2^(3-1) = 4s
	expected := before.Add(4 * time.Second)
	assert.WithinDuration(t, expected, result, 100*time.Millisecond)
}

func TestComputeNextRetryTime_CappedAtMaxInterval(t *testing.T) {
	task := &domain.ActivityTask{
		Attempt: 10,
		RetryPolicy: &domain.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaxInterval:        5 * time.Second,
		},
	}
	before := time.Now()
	result := computeNextRetryTime(task)
	expected := before.Add(5 * time.Second)
	assert.WithinDuration(t, expected, result, 100*time.Millisecond)
}

func TestComputeNextRetryTime_ZeroMaxInterval(t *testing.T) {
	task := &domain.ActivityTask{
		Attempt: 3,
		RetryPolicy: &domain.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaxInterval:        0,
		},
	}
	before := time.Now()
	result := computeNextRetryTime(task)
	// No cap: 1s * 2^2 = 4s
	expected := before.Add(4 * time.Second)
	assert.WithinDuration(t, expected, result, 100*time.Millisecond)
}
