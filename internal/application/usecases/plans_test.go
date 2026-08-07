package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func int64Ptr2(n int64) *int64 { return &n }

func TestCreatePlanRejectsStressTestWithTargetOrAccountID(t *testing.T) {
	uc := NewCreatePlan(newFakePlanRepo(), newFakeAccountRepo())

	if _, err := uc.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "car payment", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 1000, TargetAmount: int64Ptr2(5000),
	}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for stress-test with target_amount, got %v", err)
	}

	accountID := "acc-1"
	if _, err := uc.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "car payment", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 1000, AccountID: &accountID,
	}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for stress-test with account_id, got %v", err)
	}
}

func TestCreatePlanRejectsSavingsWithoutTargetOrAccountID(t *testing.T) {
	uc := NewCreatePlan(newFakePlanRepo(), newFakeAccountRepo())

	if _, err := uc.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 1000,
	}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for savings without target_amount/account_id, got %v", err)
	}
}

func TestCreatePlanRejectsSavingsAccountCurrencyMismatch(t *testing.T) {
	accounts := newFakeAccountRepo()
	account, err := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Currency: "brl"})
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreatePlan(newFakePlanRepo(), accounts)

	_, err = uc.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 1000, TargetAmount: int64Ptr2(50000), AccountID: &account.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for plan/account currency mismatch, got %v", err)
	}
}

func TestCreatePlanSavingsSucceeds(t *testing.T) {
	accounts := newFakeAccountRepo()
	account, err := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreatePlan(newFakePlanRepo(), accounts)

	plan, err := uc.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 1000, TargetAmount: int64Ptr2(50000), AccountID: &account.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != string(entities.PlanStatusActive) {
		t.Errorf("status = %q, want active", plan.Status)
	}
	if plan.StartDate.IsZero() {
		t.Error("start_date should default to now")
	}
}

// TestStressTestPlanNeverWritesMovements is the ticket's own acceptance
// criterion: creating/checking a stress-test plan writes zero rows to
// movements. CreatePlanUseCase structurally can't write movements (it has
// no MovementRepository dependency at all); this asserts that ListPlans/
// GetPlan reading a stress-test plan don't create any either.
func TestStressTestPlanNeverWritesMovements(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()
	createUC := NewCreatePlan(plans, newFakeAccountRepo())

	plan, err := createUC.Execute(context.Background(), CreatePlanInput{
		UserID: "u1", Name: "car payment", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 29600,
	})
	if err != nil {
		t.Fatal(err)
	}

	getUC := NewGetPlan(plans, movements)
	if _, err := getUC.Execute(context.Background(), "u1", plan.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	all, err := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("stress-test plan created/checked %d movement rows, want 0", len(all))
	}
}

// TestStressTestOnTrackFlipsWithHypotheticalCost is the ticket's own
// acceptance criterion: given fixed real income/expenses for the month,
// adding a hypothetical cost flips on_track from true to false at the
// right threshold.
func TestStressTestOnTrackFlipsWithHypotheticalCost(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	// Mid-month fixture: day 15 of a 30-day month, so the linear
	// projection multiplies expense-to-date by 2 (30/15).
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	income := activeMovement("income", 500000, entities.SyncStatusSynced)
	income.Category = string(entities.CategoryIncome)
	income.Timestamp = monthStart.AddDate(0, 0, 1)
	movements.add(income)

	expense := activeMovement("rent", -100000, entities.SyncStatusSynced)
	expense.Timestamp = monthStart.AddDate(0, 0, 2)
	movements.add(expense)
	// income=500000, expense=100000 to date. Projected expense = 100000*2 = 200000.

	lowCost, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "small hypothetical", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 100000, Status: string(entities.PlanStatusActive), StartDate: monthStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	// projected surplus = 500000 - 200000 - 100000 = 200000 >= 0 -> on_track true.

	highCost, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "big hypothetical", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 400000, Status: string(entities.PlanStatusActive), StartDate: monthStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	// projected surplus = 500000 - 200000 - 400000 = -100000 < 0 -> on_track false.

	getUC := NewGetPlan(plans, movements)

	lowDetail, err := getUC.Execute(context.Background(), "u1", lowCost.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if !lowDetail.OnTrack {
		t.Errorf("low hypothetical cost: on_track = false, want true (detail: %+v)", lowDetail)
	}

	highDetail, err := getUC.Execute(context.Background(), "u1", highCost.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if highDetail.OnTrack {
		t.Errorf("high hypothetical cost: on_track = true, want false (detail: %+v)", highDetail)
	}
}

// TestSavingsPlanProgressSumsTaggedMovements and the rest of this file
// cover the ticket's savings-plan acceptance criteria.
func TestSavingsPlanProgressSumsTaggedMovements(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	plan, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 10000, TargetAmount: int64Ptr2(30000),
		Status: string(entities.PlanStatusActive), StartDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	m1 := activeMovement("c1", 10000, entities.SyncStatusSynced)
	m1.PlanID = &plan.ID
	movements.add(m1)
	m2 := activeMovement("c2", 15000, entities.SyncStatusSynced)
	m2.PlanID = &plan.ID
	movements.add(m2)
	// An untagged movement must not count.
	movements.add(activeMovement("untagged", 99999, entities.SyncStatusSynced))

	getUC := NewGetPlan(plans, movements)
	detail, err := getUC.Execute(context.Background(), "u1", plan.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if detail.Progress != 25000 {
		t.Errorf("progress = %d, want 25000", detail.Progress)
	}
	if detail.TargetReached {
		t.Error("target_reached should be false at 25000/30000")
	}
}

func TestSavingsPlanTargetReached(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	plan, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 10000, TargetAmount: int64Ptr2(30000),
		Status: string(entities.PlanStatusActive), StartDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	m := activeMovement("c1", 30000, entities.SyncStatusSynced)
	m.PlanID = &plan.ID
	movements.add(m)

	getUC := NewGetPlan(plans, movements)
	detail, err := getUC.Execute(context.Background(), "u1", plan.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !detail.TargetReached {
		t.Error("target_reached should be true at exactly the target")
	}

	// A read never changes status — still active until an explicit PATCH.
	stored, err := plans.GetByID(context.Background(), "u1", plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(entities.PlanStatusActive) {
		t.Errorf("GET must not change status as a side effect, got %q", stored.Status)
	}
}

// TestSavingsPlanCancelledMovementReducesProgress: cancelling a
// plan-linked, already-synced movement produces a reversal that also
// carries the plan link, so it nets the contribution back out.
func TestSavingsPlanCancelledMovementReducesProgress(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	plan, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 10000, TargetAmount: int64Ptr2(30000),
		Status: string(entities.PlanStatusActive), StartDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	m := activeMovement("c1", 15000, entities.SyncStatusSynced)
	m.PlanID = &plan.ID
	movements.add(m)

	cancelUC := NewCancelMovement(movements, &fakeSyncTrigger{})
	if _, err := cancelUC.Execute(context.Background(), "u1", "c1"); err != nil {
		t.Fatal(err)
	}

	getUC := NewGetPlan(plans, movements)
	detail, err := getUC.Execute(context.Background(), "u1", plan.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if detail.Progress != 0 {
		t.Errorf("progress after cancelling the only contribution = %d, want 0", detail.Progress)
	}
}

// TestTwoPlansOfDifferentTypesDontInterfere covers the ticket's own
// acceptance criterion.
func TestTwoPlansOfDifferentTypesDontInterfere(t *testing.T) {
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	savings, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 10000, TargetAmount: int64Ptr2(30000),
		Status: string(entities.PlanStatusActive), StartDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The recommended funding shape (ticket's own words): a transfer leg,
	// category "transfer" — that's what keeps it out of the general
	// income/expense cashflow an unrelated stress-test plan reads from.
	m := activeMovement("c1", 15000, entities.SyncStatusSynced)
	m.Category = string(entities.CategoryTransfer)
	m.PlanID = &savings.ID
	movements.add(m)

	stressTest, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "car payment", Type: string(entities.PlanTypeStressTest),
		Currency: "usd", MonthlyTargetAmount: 5000,
		Status: string(entities.PlanStatusActive), StartDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Abandoning the savings plan doesn't touch the stress-test plan's
	// own numbers.
	updateUC := NewUpdatePlan(plans)
	abandoned := string(entities.PlanStatusAbandoned)
	if _, err := updateUC.Execute(context.Background(), "u1", savings.ID, UpdatePlanInput{Status: &abandoned}); err != nil {
		t.Fatal(err)
	}

	getUC := NewGetPlan(plans, movements)
	stressDetail, err := getUC.Execute(context.Background(), "u1", stressTest.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	// No real income/expense movements exist for this user this month, so
	// the stress-test's own surplus/deficit is just 0 - 0 - its
	// $5000 hypothetical monthly cost: a 5000 deficit. It must come out
	// exactly this value, unaffected by the savings plan's own transfer-
	// funded contribution (which stays out of cashflow) or by that plan
	// later being abandoned.
	const wantDeficit = -5000
	if stressDetail.Progress != wantDeficit {
		t.Errorf("stress-test plan's progress should be unaffected by the savings plan, got %d, want %d", stressDetail.Progress, wantDeficit)
	}

	savingsAfter, err := plans.GetByID(context.Background(), "u1", savings.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savingsAfter.Status != string(entities.PlanStatusAbandoned) {
		t.Errorf("savings plan status = %q, want abandoned", savingsAfter.Status)
	}
}

// --- Pace checker: early-month, mid-month ahead/behind, end-of-month exact-target ---

func TestPaceCheckerEarlyMonthSavingsCannotBeBehindYet(t *testing.T) {
	// Day 1 of a 31-day month: expected pace is monthly_target/31, a tiny
	// number, so even a small contribution already clears it.
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	plan, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 31000, TargetAmount: int64Ptr2(300000),
		Status: string(entities.PlanStatusActive), StartDate: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := activeMovement("c1", 2000, entities.SyncStatusSynced) // ahead of day-1's ~1000 pace
	m.PlanID = &plan.ID
	m.Timestamp = at
	movements.add(m)

	detail, err := NewGetPlan(plans, movements).Execute(context.Background(), "u1", plan.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.OnTrack {
		t.Errorf("day 1 with a contribution already ahead of the tiny day-1 pace should be on_track, got %+v", detail)
	}
}

func TestPaceCheckerMidMonthSavingsAheadAndBehind(t *testing.T) {
	// Day 20 of a 30-day month (2026-04): expected pace = target*20/30.
	at := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	const monthlyTarget = 30000 // expected pace at day 20 = 20000

	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	ahead, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "ahead", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: monthlyTarget, TargetAmount: int64Ptr2(300000),
		Status: string(entities.PlanStatusActive), StartDate: monthStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	mAhead := activeMovement("ahead-1", 25000, entities.SyncStatusSynced) // > 20000 pace
	mAhead.PlanID = &ahead.ID
	mAhead.Timestamp = monthStart.AddDate(0, 0, 5)
	movements.add(mAhead)

	behind, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "behind", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: monthlyTarget, TargetAmount: int64Ptr2(300000),
		Status: string(entities.PlanStatusActive), StartDate: monthStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	mBehind := activeMovement("behind-1", 10000, entities.SyncStatusSynced) // < 20000 pace
	mBehind.PlanID = &behind.ID
	mBehind.Timestamp = monthStart.AddDate(0, 0, 5)
	movements.add(mBehind)

	getUC := NewGetPlan(plans, movements)

	aheadDetail, err := getUC.Execute(context.Background(), "u1", ahead.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if !aheadDetail.OnTrack {
		t.Errorf("25000 contributed by day 20 (pace 20000) should be on_track, got %+v", aheadDetail)
	}

	behindDetail, err := getUC.Execute(context.Background(), "u1", behind.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if behindDetail.OnTrack {
		t.Errorf("10000 contributed by day 20 (pace 20000) should not be on_track, got %+v", behindDetail)
	}
	if behindDetail.ProjectedShortfall == nil {
		t.Fatal("savings plan detail must include projected_shortfall")
	}
	// projected at current rate = 10000 * 30/20 = 15000; shortfall = 30000-15000 = 15000.
	if *behindDetail.ProjectedShortfall != 15000 {
		t.Errorf("projected_shortfall = %d, want 15000", *behindDetail.ProjectedShortfall)
	}
}

func TestPaceCheckerEndOfMonthExactTarget(t *testing.T) {
	// Last day of a 30-day month: elapsed == total, so pace == full target.
	at := time.Date(2026, 4, 30, 23, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	plans := newFakePlanRepo()
	movements := newFakeMovementRepo()

	plan, err := plans.Create(context.Background(), &dto.PlanDTO{
		UserID: "u1", Name: "vacation", Type: string(entities.PlanTypeSavings),
		Currency: "usd", MonthlyTargetAmount: 30000, TargetAmount: int64Ptr2(300000),
		Status: string(entities.PlanStatusActive), StartDate: monthStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := activeMovement("c1", 30000, entities.SyncStatusSynced) // exactly hits the monthly target
	m.PlanID = &plan.ID
	m.Timestamp = monthStart.AddDate(0, 0, 10)
	movements.add(m)

	detail, err := NewGetPlan(plans, movements).Execute(context.Background(), "u1", plan.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.OnTrack {
		t.Errorf("hitting exactly the monthly target by month-end should be on_track, got %+v", detail)
	}
	if detail.ProjectedShortfall == nil || *detail.ProjectedShortfall != 0 {
		t.Errorf("projected_shortfall at exact target should be 0, got %v", detail.ProjectedShortfall)
	}
}
