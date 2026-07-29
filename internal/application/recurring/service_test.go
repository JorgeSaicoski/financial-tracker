package recurring

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// fakeRecurringRuleRepo is an in-memory RecurringRuleRepository mirroring
// the semantics the real SQLite/Postgres implementations guarantee:
// GenerateAndAdvance inserts movements and advances the watermark
// together, and ListActive only returns rules with Active=true.
type fakeRecurringRuleRepo struct {
	rules     map[string]*dto.RecurringRuleDTO
	movements []*dto.MovementDTO
	nextID    int
}

func newFakeRecurringRuleRepo() *fakeRecurringRuleRepo {
	return &fakeRecurringRuleRepo{rules: map[string]*dto.RecurringRuleDTO{}}
}

func strPtr(s string) *string { return &s }

func (f *fakeRecurringRuleRepo) add(r *dto.RecurringRuleDTO) *dto.RecurringRuleDTO {
	if r.ID == "" {
		f.nextID++
		r.ID = time.Now().Format("150405") + string(rune('a'+f.nextID))
	}
	cp := *r
	f.rules[r.ID] = &cp
	return r
}

func (f *fakeRecurringRuleRepo) Create(_ context.Context, r *dto.RecurringRuleDTO) (*dto.RecurringRuleDTO, error) {
	return f.add(r), nil
}

func (f *fakeRecurringRuleRepo) GetByID(_ context.Context, id string) (*dto.RecurringRuleDTO, error) {
	r, ok := f.rules[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRecurringRuleRepo) ListByUser(_ context.Context, userID string) ([]*dto.RecurringRuleDTO, error) {
	var out []*dto.RecurringRuleDTO
	for _, r := range f.rules {
		if r.UserID == userID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeRecurringRuleRepo) ListActive(_ context.Context) ([]*dto.RecurringRuleDTO, error) {
	var out []*dto.RecurringRuleDTO
	for _, r := range f.rules {
		if r.Active {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeRecurringRuleRepo) UpdateMetadata(_ context.Context, id, description string, categoryID *string, paymentMethod string, accountID *string) error {
	r, ok := f.rules[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	r.Description, r.CategoryID, r.PaymentMethod, r.AccountID = description, categoryID, paymentMethod, accountID
	return nil
}

func (f *fakeRecurringRuleRepo) UpdateFinancial(_ context.Context, id string, amount int64, currency string) error {
	r, ok := f.rules[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	r.Amount, r.Currency = amount, currency
	return nil
}

func (f *fakeRecurringRuleRepo) UpdateSchedule(_ context.Context, id, dayOfMonth string, endsAt *time.Time) error {
	r, ok := f.rules[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	r.DayOfMonth, r.EndsAt = dayOfMonth, endsAt
	return nil
}

func (f *fakeRecurringRuleRepo) SetActive(_ context.Context, id string, active bool) error {
	r, ok := f.rules[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	r.Active = active
	return nil
}

func (f *fakeRecurringRuleRepo) GenerateAndAdvance(_ context.Context, ruleID string, movements []*dto.MovementDTO, newWatermark time.Time) ([]*dto.MovementDTO, error) {
	r, ok := f.rules[ruleID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	for i, m := range movements {
		if m.ID == "" {
			m.ID = ruleID + "-m" + time.Now().Format("150405.000000") + string(rune('a'+i))
		}
		f.movements = append(f.movements, m)
	}
	r.LastGeneratedAt = &newWatermark
	return movements, nil
}

type discardLogger struct{}

func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

var _ logger.Logger = discardLogger{}

func TestServiceGeneratesDueMovementsAndAdvancesWatermark(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	repo.add(&dto.RecurringRuleDTO{
		UserID: "user-1", Amount: -5000, Currency: "usd", CategoryID: strPtr("housing"), PaymentMethod: "bank_transfer",
		DayOfMonth: "1", StartsAt: mustDate("2026-01-01"), Active: true,
	})
	svc := NewService(repo, discardLogger{})
	svc.(*service).now = func() time.Time { return mustDate("2026-01-01") }

	sum := svc.RunPass(context.Background())
	if sum.RulesProcessed != 1 || sum.MovementsGenerated != 1 || sum.Errors != 0 {
		t.Fatalf("summary = %+v, want 1 rule processed, 1 movement generated", sum)
	}
	if len(repo.movements) != 1 {
		t.Fatalf("got %d movements, want 1", len(repo.movements))
	}
	m := repo.movements[0]
	if m.RecurringRuleID == nil {
		t.Fatal("generated movement missing RecurringRuleID link")
	}
	if m.Amount != -5000 || m.UserID != "user-1" || m.Status != "active" || m.SyncStatus != "pending" {
		t.Errorf("generated movement = %+v, want an ordinary pending movement", m)
	}

	// Running again the same day must not double-generate: the watermark
	// already advanced past today's occurrence.
	sum = svc.RunPass(context.Background())
	if sum.MovementsGenerated != 0 {
		t.Errorf("second pass generated %d movements, want 0 (already generated)", sum.MovementsGenerated)
	}
	if len(repo.movements) != 1 {
		t.Errorf("total movements after second pass = %d, want still 1", len(repo.movements))
	}
}

func TestServiceSkipsDeactivatedRules(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	repo.add(&dto.RecurringRuleDTO{
		UserID: "user-1", Amount: -1000, Currency: "usd", CategoryID: strPtr("other"), PaymentMethod: "other",
		DayOfMonth: "1", StartsAt: mustDate("2026-01-01"), Active: false,
	})
	svc := NewService(repo, discardLogger{})
	svc.(*service).now = func() time.Time { return mustDate("2026-06-01") }

	sum := svc.RunPass(context.Background())
	if sum.RulesProcessed != 0 || sum.MovementsGenerated != 0 {
		t.Errorf("summary = %+v, want a deactivated rule to be skipped entirely", sum)
	}
	if len(repo.movements) != 0 {
		t.Errorf("got %d movements, want 0 for a deactivated rule", len(repo.movements))
	}
}

func TestServiceCatchesUpMultipleMissedPeriodsInOnePass(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	repo.add(&dto.RecurringRuleDTO{
		UserID: "user-1", Amount: 300000, Currency: "usd", CategoryID: strPtr("income"), PaymentMethod: "bank_transfer",
		DayOfMonth: "1", StartsAt: mustDate("2026-01-01"), Active: true,
	})
	svc := NewService(repo, discardLogger{})
	svc.(*service).now = func() time.Time { return mustDate("2026-04-01") }

	sum := svc.RunPass(context.Background())
	if sum.MovementsGenerated != 4 {
		t.Fatalf("generated %d movements, want 4 (Jan-Apr, one missed nightly run caught up)", sum.MovementsGenerated)
	}
}
