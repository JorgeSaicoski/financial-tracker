package dto

import "time"

// PlanDTO is the application layer's representation of one user's plan
// row (BACK-10). TargetAmount and AccountID are nil for stress_test plans
// (a hypothetical cost has no real target or funding account).
type PlanDTO struct {
	ID                  string
	UserID              string
	Name                string
	Type                string
	TargetAmount        *int64
	Currency            string
	MonthlyTargetAmount int64
	AccountID           *string
	StartDate           time.Time
	EndDate             *time.Time
	Status              string
	CreatedAt           time.Time
}
