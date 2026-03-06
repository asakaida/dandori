package domain

import "time"

type Namespace struct {
	Name        string
	Description string
	CreatedAt   time.Time
}
