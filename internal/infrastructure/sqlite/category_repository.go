package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// isUniqueViolation reports whether err came from the categories table's
// (user_id, lower(name)) unique index rejecting an insert, as opposed to
// some other failure — the two need different HTTP statuses (409 vs 500).
// modernc.org/sqlite surfaces the extended result code via Error.Code().
func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}

type categoryRepository struct {
	db *sql.DB
}

// NewCategoryRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCategoryRepository(db *sql.DB) repositories.CategoryRepository {
	return &categoryRepository{db: db}
}

const categoryColumns = `id, user_id, name, avoidability_percent, is_default, created_at`

// EnsureByName is a check-then-insert, not an atomic upsert. SQLite's
// single-connection pool (db.SetMaxOpenConns(1), see db.go) already
// serializes every call here, so the (user_id, lower(name)) unique index
// this table carries can't actually be raced in practice — but Create
// re-reads on conflict anyway (mirroring the Postgres implementation,
// which does need it) so this stays correct if that pooling assumption
// ever changes.
func (r *categoryRepository) EnsureByName(ctx context.Context, userID, name string, avoidabilityPercent *int) (*dto.CategoryDTO, error) {
	existing, err := r.getByUserAndName(ctx, userID, name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}
	created, err := r.Create(ctx, &dto.CategoryDTO{
		UserID:              userID,
		Name:                name,
		AvoidabilityPercent: avoidabilityPercent,
		CreatedAt:           time.Now().UTC(),
	})
	if err == nil {
		return created, nil
	}
	if winner, getErr := r.getByUserAndName(ctx, userID, name); getErr == nil {
		return winner, nil
	}
	return nil, err
}

func (r *categoryRepository) getByUserAndName(ctx context.Context, userID, name string) (*dto.CategoryDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE user_id = ? AND lower(name) = lower(?)`,
		userID, name)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return c, err
}

func (r *categoryRepository) GetByID(ctx context.Context, userID, id string) (*dto.CategoryDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE id = ? AND user_id = ?`, id, userID)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return c, err
}

func (r *categoryRepository) ListByUser(ctx context.Context, userID string) ([]*dto.CategoryDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query categories: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.CategoryDTO, 0)
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *categoryRepository) Create(ctx context.Context, c *dto.CategoryDTO) (*dto.CategoryDTO, error) {
	if c.ID == "" {
		c.ID = id.NewUUID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO categories (`+categoryColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.Name, nullableInt(c.AvoidabilityPercent), c.IsDefault, formatTime(c.CreatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: category %q already exists", apperrors.ErrConflict, c.Name)
		}
		return nil, fmt.Errorf("sqlite: insert category: %w", err)
	}
	return c, nil
}

func (r *categoryRepository) Update(ctx context.Context, userID, categoryID, name string, avoidabilityPercent *int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, avoidability_percent = ? WHERE id = ? AND user_id = ?`,
		name, nullableInt(avoidabilityPercent), categoryID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: update category: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update category rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *categoryRepository) HasDefault(ctx context.Context, userID string) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM categories WHERE user_id = ? AND is_default = 1`, userID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("sqlite: check default category: %w", err)
	}
	return n > 0, nil
}

// SetDefault runs both statements in a transaction so a crash between
// them can never leave two categories (or zero) flagged default for the
// same user — the partial unique index on (user_id) WHERE is_default
// backs this at the constraint level too.
func (r *categoryRepository) SetDefault(ctx context.Context, userID, categoryID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin set default: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE categories SET is_default = 0 WHERE user_id = ? AND is_default = 1`, userID,
	); err != nil {
		return fmt.Errorf("sqlite: clear previous default: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE categories SET is_default = 1 WHERE id = ? AND user_id = ?`, categoryID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: set default: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: set default rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return tx.Commit()
}

// DeleteAndReassign moves every movement and credit-card purchase
// pointing at categoryID onto defaultCategoryID before deleting
// categoryID, all inside one transaction — a crash partway through must
// never leave a movement's category_id pointing at a row that no longer
// exists.
func (r *categoryRepository) DeleteAndReassign(ctx context.Context, userID, categoryID, defaultCategoryID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin delete and reassign: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE movements SET category_id = ? WHERE category_id = ? AND user_id = ?`,
		defaultCategoryID, categoryID, userID,
	); err != nil {
		return fmt.Errorf("sqlite: reassign movements: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE credit_card_purchases SET category_id = ? WHERE category_id = ? AND user_id = ?`,
		defaultCategoryID, categoryID, userID,
	); err != nil {
		return fmt.Errorf("sqlite: reassign credit card purchases: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		`DELETE FROM categories WHERE id = ? AND user_id = ?`, categoryID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: delete category: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete category rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return tx.Commit()
}

// nullableInt converts a *int to the driver.Value SQLite expects for a
// nullable INTEGER column — nil stores SQL NULL, matching how the two
// system categories ("transfer", "income") carry no avoidability.
func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func scanCategory(row scannable) (*dto.CategoryDTO, error) {
	var (
		c                   dto.CategoryDTO
		avoidabilityPercent sql.NullInt64
		createdAt           string
	)
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &avoidabilityPercent, &c.IsDefault, &createdAt); err != nil {
		return nil, err
	}
	if avoidabilityPercent.Valid {
		n := int(avoidabilityPercent.Int64)
		c.AvoidabilityPercent = &n
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parse category created_at: %w", err)
	}
	c.CreatedAt = parsed
	return &c, nil
}
