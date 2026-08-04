package usecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type createPlanUseCase struct {
	plans    repositories.PlanRepository
	accounts repositories.AccountRepository
}

// NewCreatePlan returns interface type for dependency injection.
func NewCreatePlan(plans repositories.PlanRepository, accounts repositories.AccountRepository) CreatePlanUseCase {
	return &createPlanUseCase{plans: plans, accounts: accounts}
}

func (uc *createPlanUseCase) Execute(ctx context.Context, input CreatePlanInput) (*dto.PlanDTO, error) {
	name := strings.TrimSpace(input.Name)
	currency := strings.ToLower(strings.TrimSpace(input.Currency))
	if input.UserID == "" || name == "" || currency == "" {
		return nil, fmt.Errorf("%w: user_id, name and currency are required", apperrors.ErrInvalidInput)
	}
	planType := entities.PlanType(input.Type)
	if !planType.IsValid() {
		return nil, fmt.Errorf("%w: plan_type must be %q or %q", apperrors.ErrInvalidInput, entities.PlanTypeStressTest, entities.PlanTypeSavings)
	}
	if input.MonthlyTargetAmount <= 0 {
		return nil, fmt.Errorf("%w: monthly_target_amount must be positive", apperrors.ErrInvalidInput)
	}

	switch planType {
	case entities.PlanTypeStressTest:
		if input.TargetAmount != nil || input.AccountID != nil {
			return nil, fmt.Errorf("%w: a stress-test plan can't have target_amount or account_id", apperrors.ErrInvalidInput)
		}
	case entities.PlanTypeSavings:
		if input.TargetAmount == nil || *input.TargetAmount <= 0 {
			return nil, fmt.Errorf("%w: a savings plan requires a positive target_amount", apperrors.ErrInvalidInput)
		}
		if input.AccountID == nil || *input.AccountID == "" {
			return nil, fmt.Errorf("%w: a savings plan requires account_id", apperrors.ErrInvalidInput)
		}
		account, err := uc.accounts.GetByID(ctx, *input.AccountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.UserID != input.UserID {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if account.Currency != currency {
			return nil, fmt.Errorf("%w: plan currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, currency, account.Currency)
		}
	}

	startDate := input.StartDate
	if startDate.IsZero() {
		startDate = time.Now().UTC()
	}

	return uc.plans.Create(ctx, &dto.PlanDTO{
		UserID:              input.UserID,
		Name:                name,
		Type:                string(planType),
		TargetAmount:        input.TargetAmount,
		Currency:            currency,
		MonthlyTargetAmount: input.MonthlyTargetAmount,
		AccountID:           input.AccountID,
		StartDate:           startDate,
		EndDate:             input.EndDate,
		Status:              string(entities.PlanStatusActive),
	})
}

type listPlansUseCase struct {
	plans     repositories.PlanRepository
	movements repositories.MovementRepository
}

// NewListPlans returns interface type for dependency injection.
func NewListPlans(plans repositories.PlanRepository, movements repositories.MovementRepository) ListPlansUseCase {
	return &listPlansUseCase{plans: plans, movements: movements}
}

func (uc *listPlansUseCase) Execute(ctx context.Context, userID string, at time.Time) ([]PlanSummary, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	rows, err := uc.plans.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]PlanSummary, 0, len(rows))
	for _, p := range rows {
		progress, targetReached, err := planProgress(ctx, uc.movements, p, at)
		if err != nil {
			return nil, err
		}
		out = append(out, PlanSummary{Plan: p, Progress: progress, TargetReached: targetReached})
	}
	return out, nil
}

type getPlanUseCase struct {
	plans     repositories.PlanRepository
	movements repositories.MovementRepository
}

// NewGetPlan returns interface type for dependency injection.
func NewGetPlan(plans repositories.PlanRepository, movements repositories.MovementRepository) GetPlanUseCase {
	return &getPlanUseCase{plans: plans, movements: movements}
}

func (uc *getPlanUseCase) Execute(ctx context.Context, userID, id string, at time.Time) (PlanDetail, error) {
	if userID == "" || id == "" {
		return PlanDetail{}, apperrors.ErrInvalidInput
	}
	p, err := uc.plans.GetByID(ctx, userID, id)
	if err != nil {
		return PlanDetail{}, err
	}

	progress, targetReached, err := planProgress(ctx, uc.movements, p, at)
	if err != nil {
		return PlanDetail{}, err
	}
	onTrack, projectedShortfall, err := planPace(ctx, uc.movements, p, at)
	if err != nil {
		return PlanDetail{}, err
	}

	return PlanDetail{
		PlanSummary:        PlanSummary{Plan: p, Progress: progress, TargetReached: targetReached},
		OnTrack:            onTrack,
		ProjectedShortfall: projectedShortfall,
	}, nil
}

type updatePlanUseCase struct {
	plans repositories.PlanRepository
}

// NewUpdatePlan returns interface type for dependency injection.
func NewUpdatePlan(plans repositories.PlanRepository) UpdatePlanUseCase {
	return &updatePlanUseCase{plans: plans}
}

func (uc *updatePlanUseCase) Execute(ctx context.Context, userID, id string, input UpdatePlanInput) (*dto.PlanDTO, error) {
	if userID == "" || id == "" {
		return nil, apperrors.ErrInvalidInput
	}
	existing, err := uc.plans.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: plan name is required", apperrors.ErrInvalidInput)
		}
	}

	targetAmount := existing.TargetAmount
	if input.TargetAmount != nil {
		if existing.Type != string(entities.PlanTypeSavings) {
			return nil, fmt.Errorf("%w: only a savings plan has a target_amount", apperrors.ErrInvalidInput)
		}
		if *input.TargetAmount <= 0 {
			return nil, fmt.Errorf("%w: target_amount must be positive", apperrors.ErrInvalidInput)
		}
		targetAmount = input.TargetAmount
	}

	monthlyTargetAmount := existing.MonthlyTargetAmount
	if input.MonthlyTargetAmount != nil {
		if *input.MonthlyTargetAmount <= 0 {
			return nil, fmt.Errorf("%w: monthly_target_amount must be positive", apperrors.ErrInvalidInput)
		}
		monthlyTargetAmount = *input.MonthlyTargetAmount
	}

	endDate := existing.EndDate
	if input.EndDate != nil {
		endDate = input.EndDate
	}

	status := existing.Status
	if input.Status != nil {
		s := entities.PlanStatus(*input.Status)
		if !s.IsValid() {
			return nil, fmt.Errorf("%w: status must be %q, %q or %q",
				apperrors.ErrInvalidInput, entities.PlanStatusActive, entities.PlanStatusCompleted, entities.PlanStatusAbandoned)
		}
		status = string(s)
	}

	if err := uc.plans.Update(ctx, userID, id, name, targetAmount, monthlyTargetAmount, endDate, status); err != nil {
		return nil, err
	}
	return uc.plans.GetByID(ctx, userID, id)
}

// planProgress returns a plan's headline number and whether it's reached
// its target. For a savings plan that's the all-time SUM(amount) of
// tagged movements against TargetAmount; a stress-test plan never has a
// TargetAmount to reach (TargetReached is always false), and its headline
// number is the current (not projected) month-to-date surplus.
func planProgress(ctx context.Context, movements repositories.MovementRepository, p *dto.PlanDTO, at time.Time) (progress int64, targetReached bool, err error) {
	switch p.Type {
	case string(entities.PlanTypeSavings):
		total, err := movements.SumByPlan(ctx, p.ID, nil, nil)
		if err != nil {
			return 0, false, err
		}
		return total, p.TargetAmount != nil && total >= *p.TargetAmount, nil
	default: // stress_test
		income, expense, err := monthToDateIncomeExpense(ctx, movements, p.UserID, p.Currency, at)
		if err != nil {
			return 0, false, err
		}
		return income - expense - p.MonthlyTargetAmount, false, nil
	}
}

// planPace runs the monthly pace checker on read (never a side effect on
// the plan itself). Projected numbers extrapolate linearly from
// month-to-date figures to month-end — income is never extrapolated for
// a stress-test plan (it's lumpy: salary lands on one day), so an early-
// month check can flag "behind" before payday; that's accepted v1
// behavior, not a bug.
func planPace(ctx context.Context, movements repositories.MovementRepository, p *dto.PlanDTO, at time.Time) (onTrack bool, projectedShortfall *int64, err error) {
	elapsed, total := monthProgress(at)

	switch p.Type {
	case string(entities.PlanTypeSavings):
		monthStart := startOfMonth(at)
		thisMonth, err := movements.SumByPlan(ctx, p.ID, &monthStart, &at)
		if err != nil {
			return false, nil, err
		}
		expectedPace := p.MonthlyTargetAmount * int64(elapsed) / int64(total)
		onTrack = thisMonth >= expectedPace
		projectedAtRate := thisMonth * int64(total) / int64(elapsed)
		shortfall := p.MonthlyTargetAmount - projectedAtRate
		return onTrack, &shortfall, nil
	default: // stress_test
		income, expense, err := monthToDateIncomeExpense(ctx, movements, p.UserID, p.Currency, at)
		if err != nil {
			return false, nil, err
		}
		projectedExpense := expense * int64(total) / int64(elapsed)
		projectedSurplus := income - projectedExpense - p.MonthlyTargetAmount
		return projectedSurplus >= 0, nil, nil
	}
}

// monthToDateIncomeExpense sums real income/expense for userID in
// currency over [start of at's month, at) — reusing the exact exclusion
// rules GetCashflowUseCase already applies (voided movements and the
// transfer category are never income or expense).
func monthToDateIncomeExpense(ctx context.Context, movements repositories.MovementRepository, userID, currency string, at time.Time) (income, expense int64, err error) {
	monthStart := startOfMonth(at)
	rows, err := movements.ListByUser(ctx, userID, &currency, &monthStart, &at, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range rows {
		if m.Status == string(entities.MovementStatusVoided) {
			continue
		}
		if m.Category == string(entities.CategoryTransfer) {
			continue
		}
		if m.Amount > 0 {
			income += m.Amount
		} else {
			expense += -m.Amount
		}
	}
	return income, expense, nil
}

// startOfMonth truncates at to midnight UTC on the 1st of its month.
func startOfMonth(at time.Time) time.Time {
	y, m, _ := at.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// monthProgress returns the day-of-month (always >= 1, so safe as a
// divisor) and the total number of days in at's month.
func monthProgress(at time.Time) (elapsed, total int) {
	y, m, d := at.Date()
	firstOfNextMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
	total = firstOfNextMonth.AddDate(0, 0, -1).Day()
	return d, total
}

// resolvePlanForMovement validates a caller-supplied plan_id for
// CreateMovement/UpdateMovement/TransferBetweenAccounts (BACK-10): the
// plan must exist, belong to userID, be active, be a savings plan (a
// stress-test plan never has real movements), and its currency must match
// the movement's. planID == "" is not a caller error — it just means "no
// plan link" and returns (nil, nil).
func resolvePlanForMovement(ctx context.Context, plans repositories.PlanRepository, userID, planID, currency string) (*string, error) {
	if planID == "" {
		return nil, nil
	}
	p, err := plans.GetByID(ctx, userID, planID)
	if apperrors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("%w: plan not found", apperrors.ErrInvalidInput)
	}
	if err != nil {
		return nil, err
	}
	if p.Status != string(entities.PlanStatusActive) {
		return nil, fmt.Errorf("%w: plan is not active", apperrors.ErrInvalidInput)
	}
	if p.Type != string(entities.PlanTypeSavings) {
		return nil, fmt.Errorf("%w: only a savings plan can be funded by movements", apperrors.ErrInvalidInput)
	}
	if p.Currency != currency {
		return nil, fmt.Errorf("%w: movement currency %q does not match plan currency %q",
			apperrors.ErrInvalidInput, currency, p.Currency)
	}
	return &planID, nil
}
