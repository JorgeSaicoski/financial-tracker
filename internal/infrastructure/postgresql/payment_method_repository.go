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

type paymentMethodRepository struct {
	db *sql.DB
}

// NewPaymentMethodRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewPaymentMethodRepository(db *sql.DB) repositories.PaymentMethodRepository {
	return &paymentMethodRepository{db: db}
}

const paymentMethodColumns = `id, user_id, name, created_at`

// EnsureByName is a check-then-insert, not an atomic upsert, but stays
// idempotent under concurrent callers despite the (user_id, lower(name))
// unique index this table carries: if Create loses the race, that unique
// violation is exactly the signal that a concurrent EnsureByName call
// just won the insert, so re-reading returns its row instead of
// propagating the DB error.
func (r *paymentMethodRepository) EnsureByName(ctx context.Context, userID, name string) (*dto.PaymentMethodDTO, error) {
	existing, err := r.getByUserAndName(ctx, userID, name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}
	created, err := r.Create(ctx, &dto.PaymentMethodDTO{
		UserID:    userID,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		return created, nil
	}
	if winner, getErr := r.getByUserAndName(ctx, userID, name); getErr == nil {
		return winner, nil
	}
	return nil, err
}

func (r *paymentMethodRepository) getByUserAndName(ctx context.Context, userID, name string) (*dto.PaymentMethodDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE user_id = $1 AND lower(name) = lower($2)`,
		userID, name)
	m, err := scanPaymentMethod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *paymentMethodRepository) GetByID(ctx context.Context, userID, id string) (*dto.PaymentMethodDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE id = $1 AND user_id = $2`, id, userID)
	m, err := scanPaymentMethod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *paymentMethodRepository) ListByUser(ctx context.Context, userID string) ([]*dto.PaymentMethodDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query payment methods: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.PaymentMethodDTO, 0)
	for rows.Next() {
		m, err := scanPaymentMethod(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *paymentMethodRepository) Create(ctx context.Context, m *dto.PaymentMethodDTO) (*dto.PaymentMethodDTO, error) {
	if m.ID == "" {
		m.ID = id.NewUUID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO payment_methods (`+paymentMethodColumns+`) VALUES ($1, $2, $3, $4)`,
		m.ID, m.UserID, m.Name, m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert payment method: %w", err)
	}
	return m, nil
}

func (r *paymentMethodRepository) Update(ctx context.Context, userID, methodID, name string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE payment_methods SET name = $1 WHERE id = $2 AND user_id = $3`,
		name, methodID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: update payment method: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: update payment method rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *paymentMethodRepository) Delete(ctx context.Context, userID, methodID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM payment_methods WHERE id = $1 AND user_id = $2`, methodID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: delete payment method: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: delete payment method rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func scanPaymentMethod(row scannable) (*dto.PaymentMethodDTO, error) {
	var m dto.PaymentMethodDTO
	if err := row.Scan(&m.ID, &m.UserID, &m.Name, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}
