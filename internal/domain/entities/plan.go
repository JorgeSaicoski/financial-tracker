package entities

import "time"

// PlanType distinguishes a plan that never posts real movements
// (stress-test: "what if I had this extra cost?") from one that's funded
// by real movements toward a real target (savings).
type PlanType string

const (
	PlanTypeStressTest PlanType = "stress_test"
	PlanTypeSavings    PlanType = "savings"
)

func (t PlanType) IsValid() bool {
	return t == PlanTypeStressTest || t == PlanTypeSavings
}

// PlanStatus never changes as a side effect of a read (GET) — only an
// explicit PATCH /plans/{id} moves a plan out of "active".
type PlanStatus string

const (
	PlanStatusActive    PlanStatus = "active"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusAbandoned PlanStatus = "abandoned"
)

func (s PlanStatus) IsValid() bool {
	return s == PlanStatusActive || s == PlanStatusCompleted || s == PlanStatusAbandoned
}

// Plan is a user's monthly-figure goal with a pace checker computed on
// read (BACK-10). TargetAmount and AccountID are set only for Savings
// plans (nil for StressTest — a hypothetical cost has no real target or
// funding account).
type Plan struct {
	ID                  string
	Name                string
	Type                PlanType
	Currency            string
	MonthlyTargetAmount []MonthlyTargetAmount
	StartDate           time.Time
	EndDate             *time.Time
	Status              PlanStatus
	CreatedAt           time.Time
}

type MonthlyTargetAmount struct {
	CostTarget   int64
	IncomeTarget int64
	CostActual   int64
	IncomeActual int64
	Date         time.Time
}
