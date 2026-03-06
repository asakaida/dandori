package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCronSchedule_Valid(t *testing.T) {
	cases := []string{
		"* * * * *",
		"0 12 * * *",
		"*/5 * * * *",
		"0 0 1 1 *",
		"0 9 * * 1-5",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			err := ValidateCronSchedule(expr)
			require.NoError(t, err)
		})
	}
}

func TestValidateCronSchedule_Invalid(t *testing.T) {
	cases := []string{
		"",
		"not-a-cron",
		"* * *",
		"60 * * * *",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			err := ValidateCronSchedule(expr)
			assert.Error(t, err)
		})
	}
}
