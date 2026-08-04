package postgresql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

const planTestUserID = "00000000-0000-0000-0000-000000000001"

func testStressTestPlan() *dto.PlanDTO {
	return &dto.PlanDTO{
		UserID:              planTestUserID,
		Name:                "car payment",
		Type:                "stress_test",
		Currency:            "usd",
		MonthlyTargetAmount: 50000,
		StartDate:           nowTruncated(),
		Status:              "active",
	}
}

func TestPlanCreateGetRoundtrip(t *testing.T) {
	repo := NewPlanRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, testStressTestPlan())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("no id generated")
	}

	got, err := repo.GetByID(ctx, planTestUserID, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "car payment" || got.Type != "stress_test" || got.Currency != "usd" ||
		got.MonthlyTargetAmount != 50000 || got.Status != "active" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.TargetAmount != nil || got.AccountID != nil {
		t.Errorf("stress-test plan must carry no target_amount/account_id, got %+v", got)
	}

	if _, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000002", created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("wrong user: want ErrNotFound, got %v", err)
	}
	if _, err := repo.GetByID(ctx, planTestUserID, "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestPlanCreateSavingsWithAccountAndTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	accounts := NewAccountRepository(db)
	account, err := accounts.Create(ctx, &dto.AccountDTO{
		UserID: planTestUserID, Name: "vacation fund", Type: "bank", Currency: "usd", CreatedAt: nowTruncated(),
	})
	if err != nil {
		t.Fatal(err)
	}

	target := int64(300000)
	plan := &dto.PlanDTO{
		UserID: planTestUserID, Name: "vacation", Type: "savings",
		TargetAmount: &target, Currency: "usd", MonthlyTargetAmount: 30000,
		AccountID: &account.ID, StartDate: nowTruncated(), Status: "active",
	}

	repo := NewPlanRepository(db)
	created, err := repo.Create(ctx, plan)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, planTestUserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetAmount == nil || *got.TargetAmount != target {
		t.Errorf("target_amount = %v, want %d", got.TargetAmount, target)
	}
	if got.AccountID == nil || *got.AccountID != account.ID {
		t.Errorf("account_id = %v, want %s", got.AccountID, account.ID)
	}
}

func TestPlanListByUserNewestFirst(t *testing.T) {
	repo := NewPlanRepository(openTestDB(t))
	ctx := context.Background()

	first := testStressTestPlan()
	first.Name = "first"
	first.CreatedAt = nowTruncated().Add(-time.Hour)
	if _, err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := testStressTestPlan()
	second.Name = "second"
	second.CreatedAt = nowTruncated()
	if _, err := repo.Create(ctx, second); err != nil {
		t.Fatal(err)
	}

	other := testStressTestPlan()
	other.UserID = "00000000-0000-0000-0000-000000000002"
	if _, err := repo.Create(ctx, other); err != nil {
		t.Fatal(err)
	}

	all, err := repo.ListByUser(ctx, planTestUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("listed %d plans, want 2", len(all))
	}
	if all[0].Name != "second" || all[1].Name != "first" {
		t.Errorf("wrong order: got %s, %s", all[0].Name, all[1].Name)
	}
}

func TestPlanUpdateOverwritesFieldsAndRejectsWrongOwner(t *testing.T) {
	repo := NewPlanRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, testStressTestPlan())
	if err != nil {
		t.Fatal(err)
	}

	newEnd := nowTruncated().AddDate(0, 6, 0)
	if err := repo.Update(ctx, planTestUserID, created.ID, "renamed", nil, 75000, &newEnd, "abandoned"); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.GetByID(ctx, planTestUserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" || got.MonthlyTargetAmount != 75000 || got.Status != "abandoned" {
		t.Errorf("update didn't apply: %+v", got)
	}
	if got.EndDate == nil || !got.EndDate.Equal(newEnd) {
		t.Errorf("end_date = %v, want %v", got.EndDate, newEnd)
	}

	if err := repo.Update(ctx, "00000000-0000-0000-0000-000000000002", created.ID, "x", nil, 1, nil, "active"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("wrong owner: want ErrNotFound, got %v", err)
	}
	if err := repo.Update(ctx, planTestUserID, "missing", "x", nil, 1, nil, "active"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestSumByPlanInclusiveUpperBoundAndActiveOnly(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	planRepo := NewPlanRepository(db)
	plan, err := planRepo.Create(ctx, testStressTestPlan())
	if err != nil {
		t.Fatal(err)
	}

	movements := NewMovementRepository(db)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	inRange := testMovement(2000)
	inRange.PlanID = &plan.ID
	inRange.Timestamp = at
	if _, err := movements.Create(ctx, inRange); err != nil {
		t.Fatal(err)
	}

	voided := testMovement(5000)
	voided.PlanID = &plan.ID
	voided.Timestamp = at
	created, err := movements.Create(ctx, voided)
	if err != nil {
		t.Fatal(err)
	}
	if err := movements.Void(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	// A contribution stamped exactly at the query's upper bound must
	// still count — the pace checker calls this with to=now, and a
	// contribution recorded at that exact instant is real progress, not
	// a boundary technicality (BACK-10).
	sum, err := movements.SumByPlan(ctx, plan.ID, nil, &at)
	if err != nil {
		t.Fatal(err)
	}
	if sum != 2000 {
		t.Errorf("SumByPlan = %d, want 2000 (voided row excluded, in-range row included at the exact bound)", sum)
	}
}
