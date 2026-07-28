package dto

import "time"

// CreatePlanRequest is the API request body for POST /plans. TargetAmount
// and AccountID are required for a savings plan and rejected for a
// stress-test plan; StartDate defaults to now when omitted.
type CreatePlanRequest struct {
	Name                string     `json:"name"`
	Type                string     `json:"plan_type"`
	TargetAmount        *int64     `json:"target_amount,omitempty"`
	Currency            string     `json:"currency"`
	MonthlyTargetAmount int64      `json:"monthly_target_amount"`
	AccountID           *string    `json:"account_id,omitempty"`
	StartDate           *time.Time `json:"start_date,omitempty"`
	EndDate             *time.Time `json:"end_date,omitempty"`
}

// UpdatePlanRequest is the API request body for PATCH /plans/{id} — a
// field absent from the JSON body leaves that value unchanged, same
// convention as UpdateMovementRequest. Type, Currency and AccountID
// aren't editable after creation.
type UpdatePlanRequest struct {
	Name                *string    `json:"name,omitempty"`
	TargetAmount        *int64     `json:"target_amount,omitempty"`
	MonthlyTargetAmount *int64     `json:"monthly_target_amount,omitempty"`
	EndDate             *time.Time `json:"end_date,omitempty"`
	Status              *string    `json:"status,omitempty"`
}

// PlanResponse is a plan's own fields, shared by every plan endpoint.
type PlanResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"plan_type"`
	TargetAmount        *int64     `json:"target_amount,omitempty"`
	Currency            string     `json:"currency"`
	MonthlyTargetAmount int64      `json:"monthly_target_amount"`
	AccountID           *string    `json:"account_id,omitempty"`
	StartDate           time.Time  `json:"start_date"`
	EndDate             *time.Time `json:"end_date,omitempty"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
}

// PlanSummaryResponse is GET /plans' lightweight per-plan row: Progress is
// either a savings plan's all-time contribution total, or a stress-test
// plan's current (not projected) month-to-date surplus.
type PlanSummaryResponse struct {
	Plan          PlanResponse `json:"plan"`
	Progress      int64        `json:"progress"`
	TargetReached bool         `json:"target_reached"`
}

// PlanDetailResponse is GET /plans/{id}'s full response: PlanSummaryResponse
// plus the pace checker. ProjectedShortfall is savings-only — omitted for
// a stress-test plan.
type PlanDetailResponse struct {
	PlanSummaryResponse
	OnTrack            bool   `json:"on_track"`
	ProjectedShortfall *int64 `json:"projected_shortfall,omitempty"`
}

type ListPlansResponse struct {
	Plans []PlanSummaryResponse `json:"plans"`
}
