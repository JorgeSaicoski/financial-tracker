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

type paymentMethodRepository struct {
	db *sql.DB
}

// NewPaymentMethodRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewPaymentMethodRepository(db *sql.DB) repositories.PaymentMethodRepository {
	return &paymentMethodRepository{db: db}
}

const paymentMethodColumns = `id, user_id, name, created_at`

// EnsureByName is a check-then-insert, not an atomic upsert. SQLite's
// single-connection pool (db.SetMaxOpenConns(1), see db.go) already
// serializes every call here, so the (user_id, lower(name)) unique index
// this table carries can't actually be raced in practice — but Create
// re-reads on conflict anyway (mirroring the Postgres implementation,
// which does need it) so this stays correct if that pooling assumption
// ever changes.
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
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE user_id = ? AND lower(name) = lower(?)`,
		userID, name)
	m, err := scanPaymentMethod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *paymentMethodRepository) GetByID(ctx context.Context, userID, id string) (*dto.PaymentMethodDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE id = ? AND user_id = ?`, id, userID)
	m, err := scanPaymentMethod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *paymentMethodRepository) ListByUser(ctx context.Context, userID string) ([]*dto.PaymentMethodDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+paymentMethodColumns+` FROM payment_methods WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query payment methods: %w", err)
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
		`INSERT INTO payment_methods (`+paymentMethodColumns+`) VALUES (?, ?, ?, ?)`,
		m.ID, m.UserID, m.Name, formatTime(m.CreatedAt))
	if err != nil {
		// The DB's unique (user_id, lower(name)) index rejects a
		// concurrent duplicate-name insert; detecting that portably
		// (without depending on driver-specific error types) means
		// re-reading by (user_id, name) rather than inspecting err.
		if _, getErr := r.getByUserAndName(ctx, m.UserID, m.Name); getErr == nil {
			return nil, apperrors.ErrConflict
		}
		return nil, fmt.Errorf("sqlite: insert payment method: %w", err)
	}
	return m, nil
}

func (r *paymentMethodRepository) Update(ctx context.Context, userID, methodID, name string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE payment_methods SET name = ? WHERE id = ? AND user_id = ?`,
		name, methodID, userID)
	if err != nil {
		// Same unique (user_id, lower(name)) index as Create: renaming
		// to a name another of the user's payment methods already has
		// hits it too.
		if existing, getErr := r.getByUserAndName(ctx, userID, name); getErr == nil && existing.ID != methodID {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("sqlite: update payment method: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update payment method rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *paymentMethodRepository) Delete(ctx context.Context, userID, methodID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM payment_methods WHERE id = ? AND user_id = ?`, methodID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: delete payment method: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete payment method rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func scanPaymentMethod(row scannable) (*dto.PaymentMethodDTO, error) {
	var (
		m         dto.PaymentMethodDTO
		createdAt string
	)
	if err := row.Scan(&m.ID, &m.UserID, &m.Name, &createdAt); err != nil {
		return nil, err
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parse payment method created_at: %w", err)
	}
	m.CreatedAt = parsed
	return &m, nil
}
