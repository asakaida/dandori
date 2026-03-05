package engine

import (
	"math"
	"time"

	"github.com/asakaida/dandori/internal/domain"
)

func computeNextRetryTime(task *domain.ActivityTask) time.Time {
	if task.RetryPolicy == nil {
		return time.Now()
	}

	p := task.RetryPolicy
	delay := time.Duration(float64(p.InitialInterval) * math.Pow(p.BackoffCoefficient, float64(task.Attempt-1)))
	if p.MaxInterval > 0 && delay > p.MaxInterval {
		delay = p.MaxInterval
	}
	return time.Now().Add(delay)
}
