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
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// excludedUserIDsClause builds a "AND user_id NOT IN (?, ?, ...)" SQL
// fragment for ListPendingSync (BACK-13's per-user ledger sync toggle) —
// empty when there's nothing to exclude, so callers don't need an
// empty-IN-list special case.
func excludedUserIDsClause(excludedUserIDs []string) (string, []any) {
	if len(excludedUserIDs) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(excludedUserIDs))
	args := make([]any, len(excludedUserIDs))
	for i, uid := range excludedUserIDs {
		placeholders[i] = "?"
		args[i] = uid
	}
	return " AND movements.user_id NOT IN (" + strings.Join(placeholders, ", ") + ")", args
}

type movementRepository struct {
	db *sql.DB
}

// NewMovementRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewMovementRepository(db *sql.DB) repositories.MovementRepository {
	return &movementRepository{db: db}
}

// movementInsertColumns is the column list an INSERT into movements
// targets — category_id (BACK-14 follow-up), not category: the DTO's
// Category name is resolved to an id at write time (see
// resolveCategoryID) rather than stored directly.
const movementInsertColumns = `id, user_id, amount, currency, description, category_id, payment_method,
	credit_card_purchase_id, installment_number, status, cancels_movement_id, reversed_by_movement_id,
	timestamp, sync_status, ledger_transaction_id, sync_attempts, last_sync_error, last_sync_attempt_at,
	synced_at, created_at, account_id, transfer_id, avoidability_override_percent, recurring_rule_id,
	card_id, card_payment_for_card_id, plan_id`

// movementSelectColumns is what every read query selects — a LEFT JOIN
// against categories resolves category_id back to a name (COALESCE to
// "" when NULL, matching the old column's always-a-string contract), so
// dto.MovementDTO.Category keeps behaving exactly as it did when
// category was a plain string column. Every column is qualified with
// "movements." since categories also has id/user_id/created_at and an
// unqualified reference would be ambiguous once joined.
const movementSelectColumns = `movements.id, movements.user_id, movements.amount, movements.currency, movements.description,
	COALESCE(categories.name, '') AS category, movements.category_id, movements.payment_method,
	movements.credit_card_purchase_id, movements.installment_number, movements.status,
	movements.cancels_movement_id, movements.reversed_by_movement_id, movements.timestamp,
	movements.sync_status, movements.ledger_transaction_id, movements.sync_attempts,
	movements.last_sync_error, movements.last_sync_attempt_at, movements.synced_at,
	movements.created_at, movements.account_id, movements.transfer_id,
	movements.avoidability_override_percent, movements.recurring_rule_id,
	movements.card_id, movements.card_payment_for_card_id, movements.plan_id`

const movementFromClause = `movements LEFT JOIN categories ON movements.category_id = categories.id`

func (r *movementRepository) Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error) {
	if movement.ID == "" {
		movement.ID = id.NewUUID()
	}
	if err := insertMovement(ctx, r.db, movement); err != nil {
		return nil, err
	}
	return movement, nil
}

func (r *movementRepository) CreateBatch(ctx context.Context, movements []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
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

func (r *movementRepository) GetByID(ctx context.Context, movementID string) (*dto.MovementDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.id = ?`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepository) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error) {
	query := `SELECT ` + movementSelectColumns + ` FROM ` + movementFromClause + ` WHERE movements.user_id = ?`
	args := []any{userID}
	if currency != nil {
		query += ` AND movements.currency = ?`
		args = append(args, *currency)
	}
	if from != nil {
		query += ` AND movements.timestamp >= ?`
		args = append(args, formatTime(*from))
	}
	if to != nil {
		query += ` AND movements.timestamp < ?`
		args = append(args, formatTime(*to))
	}
	query += ` ORDER BY movements.timestamp DESC, movements.created_at DESC LIMIT ? OFFSET ?`
	if limit <= 0 {
		limit = -1 // SQLite: no limit
	}
	args = append(args, limit, offset)

	return r.queryMovements(ctx, query, args...)
}

func (r *movementRepository) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.credit_card_purchase_id = ? ORDER BY movements.installment_number ASC`,
		purchaseID)
}

func (r *movementRepository) ListByCard(ctx context.Context, cardID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.card_id = ? ORDER BY movements.timestamp ASC`,
		cardID)
}

func (r *movementRepository) ListCardPayments(ctx context.Context, cardID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.card_payment_for_card_id = ? ORDER BY movements.timestamp ASC`,
		cardID)
}

func (r *movementRepository) ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.transfer_id = ? ORDER BY movements.amount ASC`,
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

func (r *movementRepository) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	clause, excludeArgs := excludedUserIDsClause(excludedUserIDs)
	args := []any{formatTime(now), formatTime(now.Add(-retryCooldown))}
	args = append(args, excludeArgs...)
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+`
		 WHERE movements.status = 'active' AND movements.sync_status IN ('pending', 'failed')
		   AND movements.timestamp <= ?
		   AND (movements.last_sync_attempt_at IS NULL OR movements.last_sync_attempt_at <= ?)`+clause+`
		 ORDER BY movements.timestamp ASC`,
		args...)
}

// MarkLocalPending is BACK-13's "re-enable" path: movements created while
// this user's ledger sync was off (SyncStatusLocal) go back to "pending"
// so the next sync pass picks up the accumulated backlog. Zero matching
// rows is a normal outcome (nothing to reclassify), not an error.
func (r *movementRepository) MarkLocalPending(ctx context.Context, userID string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE movements SET sync_status = 'pending' WHERE user_id = ? AND sync_status = 'local' AND status = 'active'`,
		userID); err != nil {
		return fmt.Errorf("sqlite: mark local movements pending: %w", err)
	}
	return nil
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

func (r *movementRepository) UpdateMetadata(ctx context.Context, movementID, description string, categoryID *string, paymentMethod string, accountID, planID *string) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET description = ?, category_id = ?, payment_method = ?, account_id = ?, plan_id = ? WHERE id = ?`,
		nullString(description), categoryID, paymentMethod, accountID, planID, movementID)
}

func (r *movementRepository) UpdateAvoidabilityOverride(ctx context.Context, movementID string, avoidabilityOverridePercent *int) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET avoidability_override_percent = ? WHERE id = ?`,
		avoidabilityOverridePercent, movementID)
}

// SumByPlan sums non-voided movements tagged with planID over [from, to]
// (both inclusive) on their effective timestamp — see the
// application/repositories contract's own doc comment for why "active"
// (not further excluding reversal pairs) is the right rule here, same as
// NetByAccount, and for why "to" is inclusive.
func (r *movementRepository) SumByPlan(ctx context.Context, planID string, from, to *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE plan_id = ? AND status = 'active'`
	args := []any{planID}
	if from != nil {
		query += ` AND timestamp >= ?`
		args = append(args, formatTime(*from))
	}
	if to != nil {
		query += ` AND timestamp <= ?`
		args = append(args, formatTime(*to))
	}

	var sum int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&sum); err != nil {
		return 0, fmt.Errorf("sqlite: sum by plan: %w", err)
	}
	return sum, nil
}

func (r *movementRepository) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET amount = ?, currency = ?, timestamp = ? WHERE id = ?`,
		amount, currency, formatTime(timestamp), movementID)
}

func (r *movementRepository) Void(ctx context.Context, movementID string) error {
	return r.execOnRow(ctx, `UPDATE movements SET status = 'voided' WHERE id = ?`, movementID)
}

func (r *movementRepository) CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
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

func (r *movementRepository) Transact(ctx context.Context, fn func(repositories.MovementRepository) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(&movementRepositoryTx{tx: tx}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit transaction: %w", err)
	}
	return nil
}

// movementRepositoryTx wraps a *sql.Tx and satisfies MovementRepository.
// It is unexported and only created inside movementRepository.Transact.
// Callers must not retain a reference to the value passed to Transact's
// callback beyond the callback's return, as the underlying transaction will
// have been committed or rolled back by then.
type movementRepositoryTx struct {
	tx *sql.Tx
}

func (r *movementRepositoryTx) execOnRow(ctx context.Context, query string, args ...any) error {
	res, err := r.tx.ExecContext(ctx, query, args...)
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

func (r *movementRepositoryTx) queryMovements(ctx context.Context, query string, args ...any) ([]*dto.MovementDTO, error) {
	rows, err := r.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query movements: %w", err)
	}
	defer rows.Close()
	out := make([]*dto.MovementDTO, 0)
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *movementRepositoryTx) Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error) {
	if movement.ID == "" {
		movement.ID = id.NewUUID()
	}
	if err := insertMovement(ctx, r.tx, movement); err != nil {
		return nil, err
	}
	return movement, nil
}

func (r *movementRepositoryTx) CreateBatch(ctx context.Context, movements []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
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

func (r *movementRepositoryTx) GetByID(ctx context.Context, movementID string) (*dto.MovementDTO, error) {
	row := r.tx.QueryRowContext(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.id = ?`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepositoryTx) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error) {
	query := `SELECT ` + movementSelectColumns + ` FROM ` + movementFromClause + ` WHERE movements.user_id = ?`
	args := []any{userID}
	if currency != nil {
		query += ` AND movements.currency = ?`
		args = append(args, *currency)
	}
	if from != nil {
		query += ` AND movements.timestamp >= ?`
		args = append(args, formatTime(*from))
	}
	if to != nil {
		query += ` AND movements.timestamp < ?`
		args = append(args, formatTime(*to))
	}
	query += ` ORDER BY movements.timestamp DESC, movements.created_at DESC LIMIT ? OFFSET ?`
	if limit <= 0 {
		limit = -1
	}
	args = append(args, limit, offset)
	return r.queryMovements(ctx, query, args...)
}

func (r *movementRepositoryTx) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.credit_card_purchase_id = ? ORDER BY movements.installment_number ASC`,
		purchaseID)
}

func (r *movementRepositoryTx) ListByCard(ctx context.Context, cardID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.card_id = ? ORDER BY movements.timestamp ASC`,
		cardID)
}

func (r *movementRepositoryTx) ListCardPayments(ctx context.Context, cardID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.card_payment_for_card_id = ? ORDER BY movements.timestamp ASC`,
		cardID)
}

func (r *movementRepositoryTx) ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.transfer_id = ? ORDER BY movements.amount ASC`,
		transferID)
}

func (r *movementRepositoryTx) NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error) {
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
	if err := r.tx.QueryRowContext(ctx, query, args...).Scan(&net); err != nil {
		return 0, fmt.Errorf("sqlite: net by account: %w", err)
	}
	return net, nil
}

func (r *movementRepositoryTx) SumByPlan(ctx context.Context, planID string, from, to *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE plan_id = ? AND status = 'active'`
	args := []any{planID}
	if from != nil {
		query += ` AND timestamp >= ?`
		args = append(args, formatTime(*from))
	}
	if to != nil {
		query += ` AND timestamp <= ?`
		args = append(args, formatTime(*to))
	}
	var sum int64
	if err := r.tx.QueryRowContext(ctx, query, args...).Scan(&sum); err != nil {
		return 0, fmt.Errorf("sqlite: sum by plan: %w", err)
	}
	return sum, nil
}

func (r *movementRepositoryTx) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	clause, excludeArgs := excludedUserIDsClause(excludedUserIDs)
	args := []any{formatTime(now), formatTime(now.Add(-retryCooldown))}
	args = append(args, excludeArgs...)
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+`
		 WHERE movements.status = 'active' AND movements.sync_status IN ('pending', 'failed')
		   AND movements.timestamp <= ?
		   AND (movements.last_sync_attempt_at IS NULL OR movements.last_sync_attempt_at <= ?)`+clause+`
		 ORDER BY movements.timestamp ASC`,
		args...)
}

func (r *movementRepositoryTx) MarkLocalPending(ctx context.Context, userID string) error {
	if _, err := r.tx.ExecContext(ctx,
		`UPDATE movements SET sync_status = 'pending' WHERE user_id = ? AND sync_status = 'local' AND status = 'active'`,
		userID); err != nil {
		return fmt.Errorf("sqlite: mark local movements pending: %w", err)
	}
	return nil
}

func (r *movementRepositoryTx) MarkSynced(ctx context.Context, movementID, ledgerTransactionID string, at time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements
		 SET sync_status = 'synced', ledger_transaction_id = ?, synced_at = ?,
		     last_sync_attempt_at = ?, last_sync_error = NULL, sync_attempts = sync_attempts + 1
		 WHERE id = ?`,
		ledgerTransactionID, formatTime(at), formatTime(at), movementID)
}

func (r *movementRepositoryTx) MarkSyncFailed(ctx context.Context, movementID, syncErr string, at time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements
		 SET sync_status = 'failed', last_sync_error = ?, last_sync_attempt_at = ?,
		     sync_attempts = sync_attempts + 1
		 WHERE id = ?`,
		syncErr, formatTime(at), movementID)
}

func (r *movementRepositoryTx) UpdateMetadata(ctx context.Context, movementID, description string, categoryID *string, paymentMethod string, accountID, planID *string) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET description = ?, category_id = ?, payment_method = ?, account_id = ?, plan_id = ? WHERE id = ?`,
		nullString(description), categoryID, paymentMethod, accountID, planID, movementID)
}

func (r *movementRepositoryTx) UpdateAvoidabilityOverride(ctx context.Context, movementID string, avoidabilityOverridePercent *int) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET avoidability_override_percent = ? WHERE id = ?`,
		avoidabilityOverridePercent, movementID)
}

func (r *movementRepositoryTx) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return r.execOnRow(ctx,
		`UPDATE movements SET amount = ?, currency = ?, timestamp = ? WHERE id = ?`,
		amount, currency, formatTime(timestamp), movementID)
}

func (r *movementRepositoryTx) Void(ctx context.Context, movementID string) error {
	return r.execOnRow(ctx, `UPDATE movements SET status = 'voided' WHERE id = ?`, movementID)
}

func (r *movementRepositoryTx) CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
	if reversal.CancelsMovementID == nil {
		return nil, fmt.Errorf("sqlite: reversal has no cancels_movement_id")
	}
	if reversal.ID == "" {
		reversal.ID = id.NewUUID()
	}

	var reversedBy sql.NullString
	var status string
	err := r.tx.QueryRowContext(ctx, `SELECT reversed_by_movement_id, status FROM movements WHERE id = ?`,
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

	if err := insertMovement(ctx, r.tx, reversal); err != nil {
		return nil, err
	}
	res, err := r.tx.ExecContext(ctx,
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
	return reversal, nil
}

// Transact re-uses the current transaction — nested Transact calls join the
// outer transaction instead of creating a new one.
func (r *movementRepositoryTx) Transact(_ context.Context, fn func(repositories.MovementRepository) error) error {
	return fn(r)
}

func (r *movementRepository) queryMovements(ctx context.Context, query string, args ...any) ([]*dto.MovementDTO, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query movements: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.MovementDTO, 0)
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

func insertMovement(ctx context.Context, ex execer, m *dto.MovementDTO) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO movements (`+movementInsertColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.UserID, m.Amount, m.Currency,
		nullString(m.Description), m.CategoryID, m.PaymentMethod,
		m.CreditCardPurchaseID, m.InstallmentNumber,
		m.Status, m.CancelsMovementID, m.ReversedByMovementID,
		formatTime(m.Timestamp), m.SyncStatus, m.LedgerTransactionID,
		m.SyncAttempts, m.LastSyncError, nullTime(m.LastSyncAttemptAt),
		nullTime(m.SyncedAt), formatTime(m.CreatedAt), m.AccountID, m.TransferID,
		m.AvoidabilityOverridePercent, m.RecurringRuleID,
		m.CardID, m.CardPaymentForCardID, m.PlanID)
	if err != nil {
		return fmt.Errorf("sqlite: insert movement: %w", err)
	}
	return nil
}

// scannable covers both *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

// scanMovement adapts one movement row to the application layer's
// MovementDTO — the contract this repository implements. The row shape
// stays private to this package.
func scanMovement(row scannable) (*dto.MovementDTO, error) {
	var (
		m                                   dto.MovementDTO
		description, lastSyncError          sql.NullString
		categoryID                          sql.NullString
		purchaseID, cancelsID, reversedByID sql.NullString
		ledgerTxID, accountID, transferID   sql.NullString
		recurringRuleID                     sql.NullString
		cardID, cardPaymentForCardID        sql.NullString
		planID                              sql.NullString
		installmentNumber                   sql.NullInt64
		timestamp, createdAt                string
		lastAttemptAt, syncedAt             sql.NullString
		avoidabilityOverride                sql.NullInt64
	)

	err := row.Scan(
		&m.ID, &m.UserID, &m.Amount, &m.Currency,
		&description, &m.Category, &categoryID, &m.PaymentMethod,
		&purchaseID, &installmentNumber,
		&m.Status, &cancelsID, &reversedByID,
		&timestamp, &m.SyncStatus, &ledgerTxID,
		&m.SyncAttempts, &lastSyncError, &lastAttemptAt,
		&syncedAt, &createdAt, &accountID, &transferID,
		&avoidabilityOverride, &recurringRuleID,
		&cardID, &cardPaymentForCardID, &planID)
	if err != nil {
		return nil, err
	}

	m.Description = description.String
	m.CategoryID = stringPtr(categoryID)
	m.AccountID = stringPtr(accountID)
	m.TransferID = stringPtr(transferID)
	m.RecurringRuleID = stringPtr(recurringRuleID)
	m.PlanID = stringPtr(planID)
	m.CreditCardPurchaseID = stringPtr(purchaseID)
	m.CancelsMovementID = stringPtr(cancelsID)
	m.ReversedByMovementID = stringPtr(reversedByID)
	m.LedgerTransactionID = stringPtr(ledgerTxID)
	m.LastSyncError = stringPtr(lastSyncError)
	m.CardID = stringPtr(cardID)
	m.CardPaymentForCardID = stringPtr(cardPaymentForCardID)
	if installmentNumber.Valid {
		n := int(installmentNumber.Int64)
		m.InstallmentNumber = &n
	}
	if avoidabilityOverride.Valid {
		n := int(avoidabilityOverride.Int64)
		m.AvoidabilityOverridePercent = &n
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
