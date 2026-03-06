package engine

import (
	"fmt"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func ValidateCronSchedule(cronExpr string) error {
	_, err := cronParser.Parse(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", cronExpr, err)
	}
	return nil
}
