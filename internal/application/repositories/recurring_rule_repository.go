package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// RecurringRuleRepository persists recurring movement rules (BACK-07) and
// tracks each rule's generation watermark, expressed in application/dto
// types — infrastructure adapts its rows to these at the boundary.
type RecurringRuleRepository interface {
	// Create inserts a rule, generating its ID, and returns the stored row.
	Create(ctx context.Context, rule *dto.RecurringRuleDTO) (*dto.RecurringRuleDTO, error)
	GetByID(ctx context.Context, id string) (*dto.RecurringRuleDTO, error)
	ListByUser(ctx context.Context, userID string) ([]*dto.RecurringRuleDTO, error)
	// ListActive returns every active rule regardless of user — the
	// background generator runs one pass across every user's rules.
	ListActive(ctx context.Context) ([]*dto.RecurringRuleDTO, error)

	// UpdateMetadata overwrites the local, non-financial fields —
	// description, category, payment method, account — mirroring
	// MovementRepository's own metadata/financial split.
	UpdateMetadata(ctx context.Context, id, description, category, paymentMethod string, accountID *string) error
	// UpdateFinancial overwrites amount/currency in place. Only ever
	// affects future generations — movements already generated are
	// ordinary movements and are never retroactively edited by this.
	UpdateFinancial(ctx context.Context, id string, amount int64, currency string) error
	// UpdateSchedule overwrites day_of_month/ends_at.
	UpdateSchedule(ctx context.Context, id, dayOfMonth string, endsAt *time.Time) error
	// SetActive flips active; false stops future generation without
	// touching anything already generated.
	SetActive(ctx context.Context, id string, active bool) error

	// GenerateAndAdvance atomically inserts the given movements (each
	// already carrying RecurringRuleID) and advances the rule's
	// last_generated_at to newWatermark, in one transaction — so a crash
	// between the two can never double-generate on the next pass.
	GenerateAndAdvance(ctx context.Context, ruleID string, movements []*dto.MovementDTO, newWatermark time.Time) ([]*dto.MovementDTO, error)
}
