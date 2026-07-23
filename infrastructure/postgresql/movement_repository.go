package postgresql

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
		return nil, fmt.Errorf("postgresql: begin batch: %w", err)
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
		return nil, fmt.Errorf("postgresql: commit batch: %w", err)
	}
	return movements, nil
}

func (r *movementRepository) GetByID(ctx context.Context, movementID string) (*entities.Movement, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+movementColumns+` FROM movements WHERE id = $1`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepository) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*entities.Movement, error) {
	query := `SELECT ` + movementColumns + ` FROM movements WHERE user_id = $1`
	args := []any{userID}
	if currency != nil {
		args = append(args, *currency)
		query += fmt.Sprintf(` AND currency = $%d`, len(args))
	}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND timestamp < $%d`, len(args))
	}
	if limit <= 0 {
		limit = -1 // sentinel for "no limit", matching SQLite's convention
	}
	args = append(args, limit)
	// Unlike SQLite, Postgres has no "-1 means unlimited" LIMIT behavior, so
	// the sentinel is converted to NULL (Postgres's actual "no limit" spelling).
	query += fmt.Sprintf(` ORDER BY timestamp DESC, created_at DESC LIMIT NULLIF($%d, -1)`, len(args))
	args = append(args, offset)
	query += fmt.Sprintf(` OFFSET $%d`, len(args))

	return r.queryMovements(ctx, query, args...)
}

func (r *movementRepository) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements WHERE credit_card_purchase_id = $1 ORDER BY installment_number ASC`,
		purchaseID)
}

func (r *movementRepository) ListByTransferID(ctx context.Context, transferID string) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements WHERE transfer_id = $1 ORDER BY amount ASC`,
		transferID)
}

func (r *movementRepository) NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE account_id = $1 AND status = 'active'`
	args := []any{accountID}
	if after != nil {
		args = append(args, *after)
		query += fmt.Sprintf(` AND timestamp > $%d`, len(args))
	}
	if until != nil {
		args = append(args, *until)
		query += fmt.Sprintf(` AND timestamp <= $%d`, len(args))
	}

	var net int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&net); err != nil {
		return 0, fmt.Errorf("postgresql: net by account: %w", err)
	}
	return net, nil
}

func (r *movementRepository) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration) ([]*entities.Movement, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementColumns+` FROM movements
		 WHERE status = 'active' AND sync_status IN ('pending', 'failed')
		   AND timestamp <= $1
		   AND (last_sync_attempt_at IS NULL OR last_sync_attempt_at <= $2)
		 ORDER BY timestamp ASC`,
		now, now.Add(-retryCooldown))
}

func (r *movementRepository) MarkSynced(ctx context.Context, movementID, ledgerTransactionID string, at time.Time) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements
		 SET sync_status = 'synced', ledger_transaction_id = $1, synced_at = $2,
		     last_sync_attempt_at = $3, last_sync_error = NULL, sync_attempts = sync_attempts + 1
		 WHERE id = $4`,
		ledgerTransactionID, at, at, movementID)
}

func (r *movementRepository) MarkSyncFailed(ctx context.Context, movementID, syncErr string, at time.Time) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements
		 SET sync_status = 'failed', last_sync_error = $1, last_sync_attempt_at = $2,
		     sync_attempts = sync_attempts + 1
		 WHERE id = $3`,
		syncErr, at, movementID)
}

func (r *movementRepository) UpdateMetadata(ctx context.Context, movementID, description string, category entities.Category, paymentMethod entities.PaymentMethod, accountID *string) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements SET description = $1, category = $2, payment_method = $3, account_id = $4 WHERE id = $5`,
		nullString(description), string(category), string(paymentMethod), strOrNil(accountID), movementID)
}

func (r *movementRepository) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements SET amount = $1, currency = $2, timestamp = $3 WHERE id = $4`,
		amount, currency, timestamp, movementID)
}

func (r *movementRepository) Void(ctx context.Context, movementID string) error {
	return execOnRow(ctx, r.db, `UPDATE movements SET status = 'voided' WHERE id = $1`, movementID)
}

func (r *movementRepository) CreateReversal(ctx context.Context, reversal *entities.Movement) (*entities.Movement, error) {
	if reversal.CancelsMovementID == nil {
		return nil, fmt.Errorf("postgresql: reversal has no cancels_movement_id")
	}
	if reversal.ID == "" {
		reversal.ID = id.NewUUID()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("postgresql: begin reversal: %w", err)
	}
	defer tx.Rollback()

	if err := createReversalTx(ctx, tx, reversal); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("postgresql: commit reversal: %w", err)
	}
	return reversal, nil
}

// createReversalTx holds the logic shared by movementRepository.CreateReversal
// and movementRepositoryTx.CreateReversal: it must run inside a transaction
// so the reversal insert and the original's reversed_by_movement_id update
// commit (or roll back) together.
func createReversalTx(ctx context.Context, tx *sql.Tx, reversal *entities.Movement) error {
	var reversedBy sql.NullString
	var status string
	err := tx.QueryRowContext(ctx, `SELECT reversed_by_movement_id, status FROM movements WHERE id = $1`,
		*reversal.CancelsMovementID).Scan(&reversedBy, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("postgresql: load original: %w", err)
	}
	if reversedBy.Valid || status != string(entities.MovementStatusActive) {
		return apperrors.ErrConflict
	}

	// The reversal must exist before the original can reference it
	// (foreign key on reversed_by_movement_id); the guard on the update
	// keeps concurrent cancels of the same movement safe: Postgres's
	// row-level lock on UPDATE serializes them, and the loser's WHERE
	// clause finds reversed_by_movement_id already set and matches zero
	// rows.
	if err := insertMovement(ctx, tx, reversal); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE movements SET reversed_by_movement_id = $1
		 WHERE id = $2 AND reversed_by_movement_id IS NULL AND status = 'active'`,
		reversal.ID, *reversal.CancelsMovementID)
	if err != nil {
		return fmt.Errorf("postgresql: link reversal: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperrors.ErrConflict
	}
	return nil
}

func (r *movementRepository) Transact(ctx context.Context, fn func(repositories.MovementRepository) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgresql: begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(&movementRepositoryTx{tx: tx}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgresql: commit transaction: %w", err)
	}
	return nil
}

func (r *movementRepository) queryMovements(ctx context.Context, query string, args ...any) ([]*entities.Movement, error) {
	return queryMovements(ctx, r.db, query, args...)
}

// movementRepositoryTx wraps a *sql.Tx and satisfies MovementRepository.
// It is unexported and only created inside movementRepository.Transact.
// Callers must not retain a reference to the value passed to Transact's
// callback beyond the callback's return, as the underlying transaction will
// have been committed or rolled back by then.
type movementRepositoryTx struct {
	tx *sql.Tx
}

func (r *movementRepositoryTx) Create(ctx context.Context, movement *entities.Movement) (*entities.Movement, error) {
	if movement.ID == "" {
		movement.ID = id.NewUUID()
	}
	if err := insertMovement(ctx, r.tx, movement); err != nil {
		return nil, err
	}
	return movement, nil
}

func (r *movementRepositoryTx) CreateBatch(ctx context.Context, movements []*entities.Movement) ([]*entities.Movement, error) {
	for _, m := range movements {
		if m.ID == "" {
			m.ID = id.NewUUID()
		}
		if err := insertMovement(ctx, r.tx, m); err != nil {
			return nil, err
		}
	}
	return movements, nil
}

func (r *movementRepositoryTx) GetByID(ctx context.Context, movementID string) (*entities.Movement, error) {
	row := r.tx.QueryRowContext(ctx, `SELECT `+movementColumns+` FROM movements WHERE id = $1`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepositoryTx) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*entities.Movement, error) {
	query := `SELECT ` + movementColumns + ` FROM movements WHERE user_id = $1`
	args := []any{userID}
	if currency != nil {
		args = append(args, *currency)
		query += fmt.Sprintf(` AND currency = $%d`, len(args))
	}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND timestamp < $%d`, len(args))
	}
	if limit <= 0 {
		limit = -1
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY timestamp DESC, created_at DESC LIMIT NULLIF($%d, -1)`, len(args))
	args = append(args, offset)
	query += fmt.Sprintf(` OFFSET $%d`, len(args))

	return queryMovements(ctx, r.tx, query, args...)
}

func (r *movementRepositoryTx) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*entities.Movement, error) {
	return queryMovements(ctx, r.tx,
		`SELECT `+movementColumns+` FROM movements WHERE credit_card_purchase_id = $1 ORDER BY installment_number ASC`,
		purchaseID)
}

func (r *movementRepositoryTx) ListByTransferID(ctx context.Context, transferID string) ([]*entities.Movement, error) {
	return queryMovements(ctx, r.tx,
		`SELECT `+movementColumns+` FROM movements WHERE transfer_id = $1 ORDER BY amount ASC`,
		transferID)
}

func (r *movementRepositoryTx) NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE account_id = $1 AND status = 'active'`
	args := []any{accountID}
	if after != nil {
		args = append(args, *after)
		query += fmt.Sprintf(` AND timestamp > $%d`, len(args))
	}
	if until != nil {
		args = append(args, *until)
		query += fmt.Sprintf(` AND timestamp <= $%d`, len(args))
	}
	var net int64
	if err := r.tx.QueryRowContext(ctx, query, args...).Scan(&net); err != nil {
		return 0, fmt.Errorf("postgresql: net by account: %w", err)
	}
	return net, nil
}

func (r *movementRepositoryTx) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration) ([]*entities.Movement, error) {
	return queryMovements(ctx, r.tx,
		`SELECT `+movementColumns+` FROM movements
		 WHERE status = 'active' AND sync_status IN ('pending', 'failed')
		   AND timestamp <= $1
		   AND (last_sync_attempt_at IS NULL OR last_sync_attempt_at <= $2)
		 ORDER BY timestamp ASC`,
		now, now.Add(-retryCooldown))
}

func (r *movementRepositoryTx) MarkSynced(ctx context.Context, movementID, ledgerTransactionID string, at time.Time) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements
		 SET sync_status = 'synced', ledger_transaction_id = $1, synced_at = $2,
		     last_sync_attempt_at = $3, last_sync_error = NULL, sync_attempts = sync_attempts + 1
		 WHERE id = $4`,
		ledgerTransactionID, at, at, movementID)
}

func (r *movementRepositoryTx) MarkSyncFailed(ctx context.Context, movementID, syncErr string, at time.Time) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements
		 SET sync_status = 'failed', last_sync_error = $1, last_sync_attempt_at = $2,
		     sync_attempts = sync_attempts + 1
		 WHERE id = $3`,
		syncErr, at, movementID)
}

func (r *movementRepositoryTx) UpdateMetadata(ctx context.Context, movementID, description string, category entities.Category, paymentMethod entities.PaymentMethod, accountID *string) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements SET description = $1, category = $2, payment_method = $3, account_id = $4 WHERE id = $5`,
		nullString(description), string(category), string(paymentMethod), strOrNil(accountID), movementID)
}

func (r *movementRepositoryTx) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements SET amount = $1, currency = $2, timestamp = $3 WHERE id = $4`,
		amount, currency, timestamp, movementID)
}

func (r *movementRepositoryTx) Void(ctx context.Context, movementID string) error {
	return execOnRow(ctx, r.tx, `UPDATE movements SET status = 'voided' WHERE id = $1`, movementID)
}

func (r *movementRepositoryTx) CreateReversal(ctx context.Context, reversal *entities.Movement) (*entities.Movement, error) {
	if reversal.CancelsMovementID == nil {
		return nil, fmt.Errorf("postgresql: reversal has no cancels_movement_id")
	}
	if reversal.ID == "" {
		reversal.ID = id.NewUUID()
	}
	if err := createReversalTx(ctx, r.tx, reversal); err != nil {
		return nil, err
	}
	return reversal, nil
}

// Transact re-uses the current transaction — nested Transact calls join the
// outer transaction instead of creating a new one.
func (r *movementRepositoryTx) Transact(_ context.Context, fn func(repositories.MovementRepository) error) error {
	return fn(r)
}

// execer lets insertMovement and execOnRow run inside or outside a
// transaction: both *sql.DB and *sql.Tx satisfy it.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// queryer lets queryMovements run inside or outside a transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func execOnRow(ctx context.Context, ex execer, query string, args ...any) error {
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("postgresql: exec: %w", err)
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

func queryMovements(ctx context.Context, q queryer, query string, args ...any) ([]*entities.Movement, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query movements: %w", err)
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

func insertMovement(ctx context.Context, ex execer, m *entities.Movement) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO movements (`+movementColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
		m.ID, m.UserID, m.Amount, m.Currency,
		nullString(m.Description), string(m.Category), string(m.PaymentMethod),
		strOrNil(m.CreditCardPurchaseID), intOrNil(m.InstallmentNumber),
		string(m.Status), strOrNil(m.CancelsMovementID), strOrNil(m.ReversedByMovementID),
		m.Timestamp, string(m.SyncStatus), strOrNil(m.LedgerTransactionID),
		m.SyncAttempts, strOrNil(m.LastSyncError), timeOrNil(m.LastSyncAttemptAt),
		timeOrNil(m.SyncedAt), m.CreatedAt, strOrNil(m.AccountID), strOrNil(m.TransferID))
	if err != nil {
		return fmt.Errorf("postgresql: insert movement: %w", err)
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
		syncAttempts                            int64
		lastAttemptAt, syncedAt                 sql.NullTime
	)

	err := row.Scan(
		&m.ID, &m.UserID, &m.Amount, &m.Currency,
		&description, &category, &paymentMethod,
		&purchaseID, &installmentNumber,
		&status, &cancelsID, &reversedByID,
		&m.Timestamp, &syncSt, &ledgerTxID,
		&syncAttempts, &lastSyncError, &lastAttemptAt,
		&syncedAt, &m.CreatedAt, &accountID, &transferID)
	if err != nil {
		return nil, err
	}

	m.Description = description.String
	m.Category = entities.Category(category)
	m.PaymentMethod = entities.PaymentMethod(paymentMethod)
	m.Status = entities.MovementStatus(status)
	m.SyncStatus = entities.SyncStatus(syncSt)
	m.SyncAttempts = int(syncAttempts)
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
	if lastAttemptAt.Valid {
		t := lastAttemptAt.Time
		m.LastSyncAttemptAt = &t
	}
	if syncedAt.Valid {
		t := syncedAt.Time
		m.SyncedAt = &t
	}
	return &m, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func strOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func intOrNil(n *int) any {
	if n == nil {
		return nil
	}
	return int64(*n)
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func stringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}
