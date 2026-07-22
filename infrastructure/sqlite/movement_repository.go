package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/id"
)

type movementRepository struct {
	db *sql.DB
}

// NewMovementRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewMovementRepository(db *sql.DB) repositories.MovementRepository {
	return &movementRepository{db: db}
}

const movementColumns = `id, user_id, amount, currency, description, category, payment_method,
	credit_card_purchase_id, installment_number, status, cancels_movement_id, reversed_by_movement_id,
	timestamp, sync_status, ledger_transaction_id, sync_attempts, last_sync_error, last_sync_attempt_at,
	synced_at, created_at, account_id, transfer_id`

func (r *movementRepository) Create(ctx context.Context, movement *entities.Movement) (*entities.Movement, error) {
	if movement.ID == "" {
		movement.ID = id.NewUUID()
	}
	if err := insertMovement(ctx, r.db, movement); err != nil {
		return nil, err
	}
	return movement, nil
}

func (r *movementRepository) CreateBatch(ctx context.Context, movements []*entities.Movement) ([]*entities.Movement, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin batch: %w", err)
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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit batch: %w", err)
	}
	return movements, nil
}

func (r *movementRepository) GetByID(ctx context.Context, movementID string) (*entities.Movement, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+movementColumns+` FROM movements WHERE id = ?`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepository) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*entities.Movement, error) {
	query := `SELECT ` + movementColumns + ` FROM movements WHERE user_id = ?`
	args := []any{userID}
	if currency != nil {
		query += ` AND currency = ?`
		args = append(args, *currency)
	}
	if from != nil {
		query += ` AND timestamp >= ?`
		args = append(args, formatTime(*from))
	}
	if to != nil {
		query += ` AND timestamp < ?`
		args = append(args, formatTime(*to))
	}
	query += ` ORDER BY timestamp DESC, created_at DESC LIMIT ? OFFSET ?`
	if limit <= 0 {
		limit = -1 // SQLite: no limit
	}
	args = append(args, limit, offset)

	return r.queryMovements(ctx, query, args...)
}

func (r *movementRepository) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements WHERE credit_card_purchase_id = ? ORDER BY installment_number ASC`,
		purchaseID)
}

func (r *movementRepository) ListByTransferID(ctx context.Context, transferID string) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements WHERE transfer_id = ? ORDER BY amount ASC`,
		transferID)
}

func (r *movementRepository) NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE account_id = ? AND status = 'active'`
	args := []any{accountID}
	if after != nil {
		query += ` AND timestamp > ?`
		args = append(args, formatTime(*after))
	}
	if until != nil {
		query += ` AND timestamp <= ?`
		args = append(args, formatTime(*until))
	}

	var net int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&net); err != nil {
		return 0, fmt.Errorf("sqlite: net by account: %w", err)
	}
	return net, nil
}

func (r *movementRepository) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements
		 WHERE status = 'active' AND sync_status IN ('pending', 'failed')
		   AND timestamp <= ?
		   AND (last_sync_attempt_at IS NULL OR last_sync_attempt_at <= ?)
		 ORDER BY timestamp ASC`,
		formatTime(now), formatTime(now.Add(-retryCooldown)))
}

func (r *movementRepository) MarkSynced(ctx context.Context, movementID, ledgerTransactionID string, at time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements
		 SET sync_status = 'synced', ledger_transaction_id = ?, synced_at = ?,
		     last_sync_attempt_at = ?, last_sync_error = NULL, sync_attempts = sync_attempts + 1
		 WHERE id = ?`,
		ledgerTransactionID, formatTime(at), formatTime(at), movementID)
}

func (r *movementRepository) MarkSyncFailed(ctx context.Context, movementID, syncErr string, at time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements
		 SET sync_status = 'failed', last_sync_error = ?, last_sync_attempt_at = ?,
		     sync_attempts = sync_attempts + 1
		 WHERE id = ?`,
		syncErr, formatTime(at), movementID)
}

func (r *movementRepository) UpdateMetadata(ctx context.Context, movementID, description string, category entities.Category, paymentMethod entities.PaymentMethod, accountID *string) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET description = ?, category = ?, payment_method = ?, account_id = ? WHERE id = ?`,
		nullString(description), string(category), string(paymentMethod), accountID, movementID)
}

func (r *movementRepository) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET amount = ?, currency = ?, timestamp = ? WHERE id = ?`,
		amount, currency, formatTime(timestamp), movementID)
}

func (r *movementRepository) Void(ctx context.Context, movementID string) error {
	return r.execOnRow(ctx, `UPDATE movements SET status = 'voided' WHERE id = ?`, movementID)
}

func (r *movementRepository) CreateReversal(ctx context.Context, reversal *entities.Movement) (*entities.Movement, error) {
	if reversal.CancelsMovementID == nil {
		return nil, fmt.Errorf("sqlite: reversal has no cancels_movement_id")
	}
	if reversal.ID == "" {
		reversal.ID = id.NewUUID()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin reversal: %w", err)
	}
	defer tx.Rollback()

	var reversedBy sql.NullString
	var status string
	err = tx.QueryRowContext(ctx, `SELECT reversed_by_movement_id, status FROM movements WHERE id = ?`,
		*reversal.CancelsMovementID).Scan(&reversedBy, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: load original: %w", err)
	}
	if reversedBy.Valid || status != string(entities.MovementStatusActive) {
		return nil, apperrors.ErrConflict
	}

	// The reversal must exist before the original can reference it
	// (foreign key on reversed_by_movement_id); the guard on the update
	// keeps concurrent cancels of the same movement safe: exactly one
	// commits, the loser's insert rolls back with the transaction.
	if err := insertMovement(ctx, tx, reversal); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE movements SET reversed_by_movement_id = ?
		 WHERE id = ? AND reversed_by_movement_id IS NULL AND status = 'active'`,
		reversal.ID, *reversal.CancelsMovementID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: link reversal: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, apperrors.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit reversal: %w", err)
	}
	return reversal, nil
}

func (r *movementRepository) queryMovements(ctx context.Context, query string, args ...any) ([]*entities.Movement, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query movements: %w", err)
	}
	defer rows.Close()

	out := make([]*entities.Movement, 0)
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *movementRepository) execOnRow(ctx context.Context, query string, args ...any) error {
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

// execer lets insertMovement run inside or outside a transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertMovement(ctx context.Context, ex execer, m *entities.Movement) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO movements (`+movementColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.UserID, m.Amount, m.Currency,
		nullString(m.Description), string(m.Category), string(m.PaymentMethod),
		m.CreditCardPurchaseID, m.InstallmentNumber,
		string(m.Status), m.CancelsMovementID, m.ReversedByMovementID,
		formatTime(m.Timestamp), string(m.SyncStatus), m.LedgerTransactionID,
		m.SyncAttempts, m.LastSyncError, nullTime(m.LastSyncAttemptAt),
		nullTime(m.SyncedAt), formatTime(m.CreatedAt), m.AccountID, m.TransferID)
	if err != nil {
		return fmt.Errorf("sqlite: insert movement: %w", err)
	}
	return nil
}

// scannable covers both *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanMovement(row scannable) (*entities.Movement, error) {
	var (
		m                                       entities.Movement
		description, lastSyncError              sql.NullString
		category, paymentMethod, status, syncSt string
		purchaseID, cancelsID, reversedByID     sql.NullString
		ledgerTxID, accountID, transferID       sql.NullString
		installmentNumber                       sql.NullInt64
		timestamp, createdAt                    string
		lastAttemptAt, syncedAt                 sql.NullString
	)

	err := row.Scan(
		&m.ID, &m.UserID, &m.Amount, &m.Currency,
		&description, &category, &paymentMethod,
		&purchaseID, &installmentNumber,
		&status, &cancelsID, &reversedByID,
		&timestamp, &syncSt, &ledgerTxID,
		&m.SyncAttempts, &lastSyncError, &lastAttemptAt,
		&syncedAt, &createdAt, &accountID, &transferID)
	if err != nil {
		return nil, err
	}

	m.Description = description.String
	m.Category = entities.Category(category)
	m.PaymentMethod = entities.PaymentMethod(paymentMethod)
	m.Status = entities.MovementStatus(status)
	m.SyncStatus = entities.SyncStatus(syncSt)
	m.AccountID = stringPtr(accountID)
	m.TransferID = stringPtr(transferID)
	m.CreditCardPurchaseID = stringPtr(purchaseID)
	m.CancelsMovementID = stringPtr(cancelsID)
	m.ReversedByMovementID = stringPtr(reversedByID)
	m.LedgerTransactionID = stringPtr(ledgerTxID)
	m.LastSyncError = stringPtr(lastSyncError)
	if installmentNumber.Valid {
		n := int(installmentNumber.Int64)
		m.InstallmentNumber = &n
	}

	if m.Timestamp, err = parseTime(timestamp); err != nil {
		return nil, fmt.Errorf("sqlite: parse timestamp: %w", err)
	}
	if m.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse created_at: %w", err)
	}
	if m.LastSyncAttemptAt, err = timePtr(lastAttemptAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse last_sync_attempt_at: %w", err)
	}
	if m.SyncedAt, err = timePtr(syncedAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse synced_at: %w", err)
	}
	return &m, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func stringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func timePtr(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
