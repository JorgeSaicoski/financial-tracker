package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

type planRepository struct {
	db *sql.DB
}

// NewPlanRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewPlanRepository(db *sql.DB) repositories.PlanRepository {
	return &planRepository{db: db}
}

const planColumns = `id, user_id, name, plan_type, target_amount, currency,
	monthly_target_amount, account_id, start_date, end_date, status, created_at`

func (r *planRepository) Create(ctx context.Context, p *dto.PlanDTO) (*dto.PlanDTO, error) {
	if p.ID == "" {
		p.ID = id.NewUUID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO plans (`+planColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		p.ID, p.UserID, p.Name, p.Type, int64OrNil(p.TargetAmount), p.Currency,
		p.MonthlyTargetAmount, strOrNil(p.AccountID), p.StartDate, timeOrNil(p.EndDate),
		p.Status, p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert plan: %w", err)
	}
	return p, nil
}

func (r *planRepository) GetByID(ctx context.Context, userID, id string) (*dto.PlanDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+planColumns+` FROM plans WHERE id = $1 AND user_id = $2`, id, userID)
	p, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return p, err
}

func (r *planRepository) ListByUser(ctx context.Context, userID string) ([]*dto.PlanDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+planColumns+` FROM plans WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query plans: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.PlanDTO, 0)
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *planRepository) Update(ctx context.Context, userID, planID string, name string, targetAmount *int64, monthlyTargetAmount int64, endDate *time.Time, status string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE plans SET name = $1, target_amount = $2, monthly_target_amount = $3, end_date = $4, status = $5
		 WHERE id = $6 AND user_id = $7`,
		name, int64OrNil(targetAmount), monthlyTargetAmount, timeOrNil(endDate), status, planID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: update plan: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: update plan rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// int64OrNil converts a *int64 to the driver.Value Postgres expects for
// a nullable BIGINT column — nil stores SQL NULL, matching how a
// stress-test plan carries no target_amount.
func int64OrNil(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func scanPlan(row scannable) (*dto.PlanDTO, error) {
	var (
		p            dto.PlanDTO
		targetAmount sql.NullInt64
		accountID    sql.NullString
		endDate      sql.NullTime
	)
	if err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Type, &targetAmount, &p.Currency,
		&p.MonthlyTargetAmount, &accountID, &p.StartDate, &endDate, &p.Status, &p.CreatedAt); err != nil {
		return nil, err
	}
	if targetAmount.Valid {
		n := targetAmount.Int64
		p.TargetAmount = &n
	}
	p.AccountID = stringPtr(accountID)
	if endDate.Valid {
		t := endDate.Time
		p.EndDate = &t
	}
	return &p, nil
}
