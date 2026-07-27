package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

const categoryColumns = `id, name, avoidability_percent, created_at`

// GetByID returns any category by id — categories are globally visible,
// there's no ownership check here (see CategoryRepository doc comment).
func (r *categoryRepository) GetByID(ctx context.Context, id string) (*dto.CategoryDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+categoryColumns+` FROM categories WHERE id = ?`, id)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	contributors, err := r.loadContributors(ctx, []string{c.ID})
	if err != nil {
		return nil, err
	}
	c.ContributorIDs = contributors[c.ID]
	return c, nil
}

func (r *categoryRepository) ListAll(ctx context.Context) ([]*dto.CategoryDTO, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+categoryColumns+` FROM categories ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query all categories: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.CategoryDTO, 0)
	ids := make([]string, 0)
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	contributors, err := r.loadContributors(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range out {
		c.ContributorIDs = contributors[c.ID]
	}
	return out, nil
}

// loadContributors batches category_maintainers lookups for one or many
// category ids into a single query, so ListAll doesn't do an N+1 round
// trip per row. Missing keys in the returned map mean zero contributors
// (a system category), not an error.
func (r *categoryRepository) loadContributors(ctx context.Context, categoryIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(categoryIDs))
	if len(categoryIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(categoryIDs))
	args := make([]any, len(categoryIDs))
	for i, cid := range categoryIDs {
		placeholders[i] = "?"
		args[i] = cid
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT category_id, user_id FROM category_maintainers WHERE category_id IN (`+strings.Join(placeholders, ", ")+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query category maintainers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var categoryID, userID string
		if err := rows.Scan(&categoryID, &userID); err != nil {
			return nil, err
		}
		out[categoryID] = append(out[categoryID], userID)
	}
	return out, rows.Err()
}

// Create inserts the category and, in the same transaction, adds every
// id in c.ContributorIDs to category_maintainers.
func (r *categoryRepository) Create(ctx context.Context, c *dto.CategoryDTO) (*dto.CategoryDTO, error) {
	if c.ID == "" {
		c.ID = id.NewUUID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin create category: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO categories (`+categoryColumns+`) VALUES (?, ?, ?, ?)`,
		c.ID, c.Name, nullableInt(c.AvoidabilityPercent), formatTime(c.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("sqlite: insert category: %w", err)
	}
	for _, contributorID := range c.ContributorIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO category_maintainers (category_id, user_id) VALUES (?, ?)`,
			c.ID, contributorID); err != nil {
			return nil, fmt.Errorf("sqlite: add contributor: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit create category: %w", err)
	}
	return c, nil
}

func (r *categoryRepository) Update(ctx context.Context, categoryID, name string, avoidabilityPercent *int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, avoidability_percent = ? WHERE id = ?`,
		name, nullableInt(avoidabilityPercent), categoryID)
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

func (r *categoryRepository) IsContributor(ctx context.Context, userID, categoryID string) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM category_maintainers WHERE category_id = ? AND user_id = ?`,
		categoryID, userID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("sqlite: check contributor: %w", err)
	}
	return n > 0, nil
}

func (r *categoryRepository) CountByContributor(ctx context.Context, userID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM category_maintainers WHERE user_id = ?`, userID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count categories by contributor: %w", err)
	}
	return n, nil
}

func (r *categoryRepository) Hide(ctx context.Context, userID, categoryID string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_hidden_categories (user_id, category_id) VALUES (?, ?)`,
		userID, categoryID,
	); err != nil {
		return fmt.Errorf("sqlite: hide category: %w", err)
	}
	return nil
}

func (r *categoryRepository) HideAndReassign(ctx context.Context, userID, categoryID, defaultCategoryID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin hide and reassign: %w", err)
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
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_hidden_categories (user_id, category_id) VALUES (?, ?)`,
		userID, categoryID,
	); err != nil {
		return fmt.Errorf("sqlite: hide category: %w", err)
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
	if err := row.Scan(&c.ID, &c.Name, &avoidabilityPercent, &createdAt); err != nil {
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
