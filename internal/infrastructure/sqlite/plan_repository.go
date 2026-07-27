package sqlite

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
		`INSERT INTO plans (`+planColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.UserID, p.Name, p.Type, nullableInt64(p.TargetAmount), p.Currency,
		p.MonthlyTargetAmount, p.AccountID, formatTime(p.StartDate), nullTime(p.EndDate),
		p.Status, formatTime(p.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("sqlite: insert plan: %w", err)
	}
	return p, nil
}

func (r *planRepository) GetByID(ctx context.Context, userID, id string) (*dto.PlanDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+planColumns+` FROM plans WHERE id = ? AND user_id = ?`, id, userID)
	p, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return p, err
}

func (r *planRepository) ListByUser(ctx context.Context, userID string) ([]*dto.PlanDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+planColumns+` FROM plans WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query plans: %w", err)
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
		`UPDATE plans SET name = ?, target_amount = ?, monthly_target_amount = ?, end_date = ?, status = ?
		 WHERE id = ? AND user_id = ?`,
		name, nullableInt64(targetAmount), monthlyTargetAmount, nullTime(endDate), status, planID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: update plan: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update plan rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// nullableInt64 converts a *int64 to the driver.Value SQLite expects for
// a nullable INTEGER column — nil stores SQL NULL, matching how a
// stress-test plan carries no target_amount.
func nullableInt64(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func scanPlan(row scannable) (*dto.PlanDTO, error) {
	var (
		p                    dto.PlanDTO
		targetAmount         sql.NullInt64
		accountID            sql.NullString
		startDate, createdAt string
		endDate              sql.NullString
	)
	if err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Type, &targetAmount, &p.Currency,
		&p.MonthlyTargetAmount, &accountID, &startDate, &endDate, &p.Status, &createdAt); err != nil {
		return nil, err
	}
	if targetAmount.Valid {
		n := targetAmount.Int64
		p.TargetAmount = &n
	}
	p.AccountID = stringPtr(accountID)

	parsedStart, err := parseTime(startDate)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parse plan start_date: %w", err)
	}
	p.StartDate = parsedStart
	if endDate.Valid {
		parsedEnd, err := parseTime(endDate.String)
		if err != nil {
			return nil, fmt.Errorf("sqlite: parse plan end_date: %w", err)
		}
		p.EndDate = &parsedEnd
	}
	parsedCreated, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parse plan created_at: %w", err)
	}
	p.CreatedAt = parsedCreated
	return &p, nil
}
