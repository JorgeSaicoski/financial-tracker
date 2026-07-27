package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// pgUniqueViolation is Postgres's error code for a unique-index conflict
// (class 23, "integrity constraint violation" — 23505 specifically).
const pgUniqueViolation = "23505"

// isUniqueViolation reports whether err came from the categories table's
// (user_id, lower(name)) unique index rejecting an insert, as opposed to
// some other failure — the two need different HTTP statuses (409 vs 500).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

type categoryRepository struct {
	db *sql.DB
}

// NewCategoryRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCategoryRepository(db *sql.DB) repositories.CategoryRepository {
	return &categoryRepository{db: db}
}

const categoryColumns = `id, user_id, name, avoidability_percent, created_at`

// EnsureByName is a check-then-insert, not an atomic upsert, but stays
// idempotent under concurrent callers despite the (user_id, lower(name))
// unique index this table carries: if Create loses the race, that unique
// violation is exactly the signal that a concurrent EnsureByName call
// just won the insert, so re-reading returns its row instead of
// propagating the DB error.
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
		`SELECT `+categoryColumns+` FROM categories WHERE user_id = $1 AND lower(name) = lower($2)`,
		userID, name)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return c, err
}

func (r *categoryRepository) GetByID(ctx context.Context, userID, id string) (*dto.CategoryDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE id = $1 AND user_id = $2`, id, userID)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return c, err
}

func (r *categoryRepository) ListByUser(ctx context.Context, userID string) ([]*dto.CategoryDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query categories: %w", err)
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
		`INSERT INTO categories (`+categoryColumns+`) VALUES ($1, $2, $3, $4, $5)`,
		c.ID, c.UserID, c.Name, nullableInt(c.AvoidabilityPercent), c.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: category %q already exists", apperrors.ErrConflict, c.Name)
		}
		return nil, fmt.Errorf("postgresql: insert category: %w", err)
	}
	return c, nil
}

func (r *categoryRepository) Update(ctx context.Context, userID, categoryID, name string, avoidabilityPercent *int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = $1, avoidability_percent = $2 WHERE id = $3 AND user_id = $4`,
		name, nullableInt(avoidabilityPercent), categoryID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: update category: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: update category rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *categoryRepository) Delete(ctx context.Context, userID, categoryID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM categories WHERE id = $1 AND user_id = $2`, categoryID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: delete category: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: delete category rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// nullableInt converts a *int to the driver.Value Postgres expects for a
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
	)
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &avoidabilityPercent, &c.CreatedAt); err != nil {
		return nil, err
	}
	if avoidabilityPercent.Valid {
		n := int(avoidabilityPercent.Int64)
		c.AvoidabilityPercent = &n
	}
	return &c, nil
}
