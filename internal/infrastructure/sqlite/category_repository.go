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

type categoryRepository struct {
	db *sql.DB
}

// NewCategoryRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCategoryRepository(db *sql.DB) repositories.CategoryRepository {
	return &categoryRepository{db: db}
}

const categoryColumns = `id, user_id, name, avoidability_percent, created_at`

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
		`INSERT INTO categories (`+categoryColumns+`) VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.Name, nullableInt(c.AvoidabilityPercent), formatTime(c.CreatedAt))
	if err != nil {
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

func (r *categoryRepository) Delete(ctx context.Context, userID, categoryID string) error {
	result, err := r.db.ExecContext(ctx,
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
	return nil
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
		c                    dto.CategoryDTO
		avoidabilityPercent  sql.NullInt64
		createdAt            string
	)
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &avoidabilityPercent, &createdAt); err != nil {
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
