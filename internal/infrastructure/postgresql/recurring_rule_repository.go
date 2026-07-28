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

type recurringRuleRepository struct {
	db *sql.DB
}

// NewRecurringRuleRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewRecurringRuleRepository(db *sql.DB) repositories.RecurringRuleRepository {
	return &recurringRuleRepository{db: db}
}

// recurringRuleInsertColumns is the column list an INSERT into
// recurring_rules targets — category_id (BACK-14 follow-up) comes
// straight from the DTO, no name resolution happens here.
const recurringRuleInsertColumns = `id, user_id, amount, currency, description, category_id, payment_method,
	account_id, day_of_month, starts_at, ends_at, active, last_generated_at, created_at`

// recurringRuleSelectColumns/recurringRuleFromClause mirror
// movementSelectColumns/movementFromClause: a LEFT JOIN against
// categories resolves category_id back to a name, so
// dto.RecurringRuleDTO.Category keeps behaving exactly as it did when
// category was a plain string column.
const recurringRuleSelectColumns = `recurring_rules.id, recurring_rules.user_id, recurring_rules.amount, recurring_rules.currency,
	recurring_rules.description, COALESCE(categories.name, '') AS category, recurring_rules.category_id, recurring_rules.payment_method,
	recurring_rules.account_id, recurring_rules.day_of_month, recurring_rules.starts_at, recurring_rules.ends_at,
	recurring_rules.active, recurring_rules.last_generated_at, recurring_rules.created_at`

const recurringRuleFromClause = `recurring_rules LEFT JOIN categories ON recurring_rules.category_id = categories.id`

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
	row := r.db.QueryRowContext(ctx,
		`SELECT `+recurringRuleSelectColumns+` FROM `+recurringRuleFromClause+` WHERE recurring_rules.id = $1`, ruleID)
	rule, err := scanRecurringRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return rule, err
}

func (r *recurringRuleRepository) ListByUser(ctx context.Context, userID string) ([]*dto.RecurringRuleDTO, error) {
	return r.queryRules(ctx,
		`SELECT `+recurringRuleSelectColumns+` FROM `+recurringRuleFromClause+` WHERE recurring_rules.user_id = $1 ORDER BY recurring_rules.created_at ASC`, userID)
}

func (r *recurringRuleRepository) ListActive(ctx context.Context) ([]*dto.RecurringRuleDTO, error) {
	return r.queryRules(ctx,
		`SELECT `+recurringRuleSelectColumns+` FROM `+recurringRuleFromClause+` WHERE recurring_rules.active = TRUE ORDER BY recurring_rules.created_at ASC`)
}

func (r *recurringRuleRepository) UpdateMetadata(ctx context.Context, ruleID, description string, categoryID *string, paymentMethod string, accountID *string) error {
	return execOnRow(ctx, r.db,
		`UPDATE recurring_rules SET description = $1, category_id = $2, payment_method = $3, account_id = $4 WHERE id = $5`,
		nullString(description), strOrNil(categoryID), paymentMethod, strOrNil(accountID), ruleID)
}

func (r *recurringRuleRepository) UpdateFinancial(ctx context.Context, ruleID string, amount int64, currency string) error {
	return execOnRow(ctx, r.db, `UPDATE recurring_rules SET amount = $1, currency = $2 WHERE id = $3`, amount, currency, ruleID)
}

func (r *recurringRuleRepository) UpdateSchedule(ctx context.Context, ruleID, dayOfMonth string, endsAt *time.Time) error {
	return execOnRow(ctx, r.db,
		`UPDATE recurring_rules SET day_of_month = $1, ends_at = $2 WHERE id = $3`,
		dayOfMonth, timeOrNil(endsAt), ruleID)
}

func (r *recurringRuleRepository) SetActive(ctx context.Context, ruleID string, active bool) error {
	return execOnRow(ctx, r.db, `UPDATE recurring_rules SET active = $1 WHERE id = $2`, active, ruleID)
}

func (r *recurringRuleRepository) GenerateAndAdvance(ctx context.Context, ruleID string, movements []*dto.MovementDTO, newWatermark time.Time) ([]*dto.MovementDTO, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("postgresql: begin generate: %w", err)
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

	res, err := tx.ExecContext(ctx, `UPDATE recurring_rules SET last_generated_at = $1 WHERE id = $2`, newWatermark, ruleID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: advance watermark: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, apperrors.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("postgresql: commit generate: %w", err)
	}
	return movements, nil
}

func (r *recurringRuleRepository) queryRules(ctx context.Context, query string, args ...any) ([]*dto.RecurringRuleDTO, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query recurring rules: %w", err)
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

func insertRecurringRule(ctx context.Context, ex execer, rule *dto.RecurringRuleDTO) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO recurring_rules (`+recurringRuleInsertColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		rule.ID, rule.UserID, rule.Amount, rule.Currency,
		nullString(rule.Description), strOrNil(rule.CategoryID), rule.PaymentMethod,
		strOrNil(rule.AccountID), rule.DayOfMonth, rule.StartsAt, timeOrNil(rule.EndsAt),
		rule.Active, timeOrNil(rule.LastGeneratedAt), rule.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgresql: insert recurring rule: %w", err)
	}
	return nil
}

// scanRecurringRule adapts one recurring_rules row to the application
// layer's RecurringRuleDTO.
func scanRecurringRule(row scannable) (*dto.RecurringRuleDTO, error) {
	var (
		rule                    dto.RecurringRuleDTO
		description             sql.NullString
		categoryID              sql.NullString
		accountID               sql.NullString
		endsAt, lastGeneratedAt sql.NullTime
	)

	err := row.Scan(
		&rule.ID, &rule.UserID, &rule.Amount, &rule.Currency,
		&description, &rule.Category, &categoryID, &rule.PaymentMethod,
		&accountID, &rule.DayOfMonth, &rule.StartsAt, &endsAt,
		&rule.Active, &lastGeneratedAt, &rule.CreatedAt)
	if err != nil {
		return nil, err
	}

	rule.Description = description.String
	rule.CategoryID = stringPtr(categoryID)
	rule.AccountID = stringPtr(accountID)
	if endsAt.Valid {
		t := endsAt.Time
		rule.EndsAt = &t
	}
	if lastGeneratedAt.Valid {
		t := lastGeneratedAt.Time
		rule.LastGeneratedAt = &t
	}
	return &rule, nil
}
