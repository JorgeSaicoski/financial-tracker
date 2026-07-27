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

type recurringRuleRepository struct {
	db *sql.DB
}

// NewRecurringRuleRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewRecurringRuleRepository(db *sql.DB) repositories.RecurringRuleRepository {
	return &recurringRuleRepository{db: db}
}

const recurringRuleColumns = `id, user_id, amount, currency, description, category, payment_method,
	account_id, day_of_month, starts_at, ends_at, active, last_generated_at, created_at`

func (r *recurringRuleRepository) Create(ctx context.Context, rule *dto.RecurringRuleDTO) (*dto.RecurringRuleDTO, error) {
	if rule.ID == "" {
		rule.ID = id.NewUUID()
	}
	if err := insertRecurringRule(ctx, r.db, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *recurringRuleRepository) GetByID(ctx context.Context, ruleID string) (*dto.RecurringRuleDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+recurringRuleColumns+` FROM recurring_rules WHERE id = ?`, ruleID)
	rule, err := scanRecurringRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return rule, err
}

func (r *recurringRuleRepository) ListByUser(ctx context.Context, userID string) ([]*dto.RecurringRuleDTO, error) {
	return r.queryRules(ctx, `SELECT `+recurringRuleColumns+` FROM recurring_rules WHERE user_id = ? ORDER BY created_at ASC`, userID)
}

func (r *recurringRuleRepository) ListActive(ctx context.Context) ([]*dto.RecurringRuleDTO, error) {
	return r.queryRules(ctx, `SELECT `+recurringRuleColumns+` FROM recurring_rules WHERE active = 1 ORDER BY created_at ASC`)
}

func (r *recurringRuleRepository) UpdateMetadata(ctx context.Context, ruleID, description, category, paymentMethod string, accountID *string) error {
	return r.execOnRow(ctx,
		`UPDATE recurring_rules SET description = ?, category = ?, payment_method = ?, account_id = ? WHERE id = ?`,
		nullString(description), category, paymentMethod, accountID, ruleID)
}

func (r *recurringRuleRepository) UpdateFinancial(ctx context.Context, ruleID string, amount int64, currency string) error {
	return r.execOnRow(ctx, `UPDATE recurring_rules SET amount = ?, currency = ? WHERE id = ?`, amount, currency, ruleID)
}

func (r *recurringRuleRepository) UpdateSchedule(ctx context.Context, ruleID, dayOfMonth string, endsAt *time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE recurring_rules SET day_of_month = ?, ends_at = ? WHERE id = ?`,
		dayOfMonth, nullTime(endsAt), ruleID)
}

func (r *recurringRuleRepository) SetActive(ctx context.Context, ruleID string, active bool) error {
	return r.execOnRow(ctx, `UPDATE recurring_rules SET active = ? WHERE id = ?`, boolToInt(active), ruleID)
}

func (r *recurringRuleRepository) GenerateAndAdvance(ctx context.Context, ruleID string, movements []*dto.MovementDTO, newWatermark time.Time) ([]*dto.MovementDTO, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin generate: %w", err)
	}
	defer tx.Rollback()

	for _, m := range movements {
		if m.ID == "" {
			m.ID = id.NewUUID()
		}
		if err := insertMovement(ctx, tx, m); err != nil {
			return nil, err
		}
	}

	res, err := tx.ExecContext(ctx, `UPDATE recurring_rules SET last_generated_at = ? WHERE id = ?`, formatTime(newWatermark), ruleID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: advance watermark: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, apperrors.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit generate: %w", err)
	}
	return movements, nil
}

func (r *recurringRuleRepository) queryRules(ctx context.Context, query string, args ...any) ([]*dto.RecurringRuleDTO, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query recurring rules: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.RecurringRuleDTO, 0)
	for rows.Next() {
		rule, err := scanRecurringRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *recurringRuleRepository) execOnRow(ctx context.Context, query string, args ...any) error {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite: exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func insertRecurringRule(ctx context.Context, ex execer, rule *dto.RecurringRuleDTO) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO recurring_rules (`+recurringRuleColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.UserID, rule.Amount, rule.Currency,
		nullString(rule.Description), rule.Category, rule.PaymentMethod,
		rule.AccountID, rule.DayOfMonth, formatTime(rule.StartsAt), nullTime(rule.EndsAt),
		boolToInt(rule.Active), nullTime(rule.LastGeneratedAt), formatTime(rule.CreatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: insert recurring rule: %w", err)
	}
	return nil
}

// scanRecurringRule adapts one recurring_rules row to the application
// layer's RecurringRuleDTO.
func scanRecurringRule(row scannable) (*dto.RecurringRuleDTO, error) {
	var (
		rule                    dto.RecurringRuleDTO
		description             sql.NullString
		accountID               sql.NullString
		startsAt                string
		endsAt, lastGeneratedAt sql.NullString
		active                  int
		createdAt               string
	)

	err := row.Scan(
		&rule.ID, &rule.UserID, &rule.Amount, &rule.Currency,
		&description, &rule.Category, &rule.PaymentMethod,
		&accountID, &rule.DayOfMonth, &startsAt, &endsAt,
		&active, &lastGeneratedAt, &createdAt)
	if err != nil {
		return nil, err
	}

	rule.Description = description.String
	rule.AccountID = stringPtr(accountID)
	rule.Active = active != 0

	if rule.StartsAt, err = parseTime(startsAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse starts_at: %w", err)
	}
	if rule.EndsAt, err = timePtr(endsAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse ends_at: %w", err)
	}
	if rule.LastGeneratedAt, err = timePtr(lastGeneratedAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse last_generated_at: %w", err)
	}
	if rule.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse created_at: %w", err)
	}
	return &rule, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
