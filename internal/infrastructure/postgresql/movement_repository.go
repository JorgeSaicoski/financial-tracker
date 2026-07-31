package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// excludedUserIDsClause builds a "AND movements.user_id NOT IN ($3, $4,
// ...)" SQL fragment for ListPendingSync (BACK-13's per-user ledger sync
// toggle), numbering placeholders from paramOffset+1 so callers can
// append it after their own positional params. Empty when there's
// nothing to exclude, so callers don't need an empty-IN-list special
// case.
func excludedUserIDsClause(excludedUserIDs []string, paramOffset int) (string, []any) {
	if len(excludedUserIDs) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(excludedUserIDs))
	args := make([]any, len(excludedUserIDs))
	for i, uid := range excludedUserIDs {
		placeholders[i] = "$" + strconv.Itoa(paramOffset+i+1)
		args[i] = uid
	}
	return " AND movements.user_id NOT IN (" + strings.Join(placeholders, ", ") + ")", args
}

type movementRepository struct {
	db *sql.DB
}

// NewMovementRepository returns the application interface type, not the
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
	synced_at, created_at, account_id, transfer_id, plan_id,
	avoidability_override_percent, recurring_rule_id`

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
	movements.created_at, movements.account_id, movements.transfer_id, movements.plan_id,
	movements.avoidability_override_percent, movements.recurring_rule_id`

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

func (r *movementRepository) GetByID(ctx context.Context, movementID string) (*dto.MovementDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.id = $1`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepository) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error) {
	query := `SELECT ` + movementSelectColumns + ` FROM ` + movementFromClause + ` WHERE movements.user_id = $1`
	args := []any{userID}
	if currency != nil {
		args = append(args, *currency)
		query += fmt.Sprintf(` AND movements.currency = $%d`, len(args))
	}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND movements.timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND movements.timestamp < $%d`, len(args))
	}
	if limit <= 0 {
		limit = -1 // sentinel for "no limit", matching SQLite's convention
	}
	args = append(args, limit)
	// Unlike SQLite, Postgres has no "-1 means unlimited" LIMIT behavior, so
	// the sentinel is converted to NULL (Postgres's actual "no limit" spelling).
	query += fmt.Sprintf(` ORDER BY movements.timestamp DESC, movements.created_at DESC LIMIT NULLIF($%d, -1)`, len(args))
	args = append(args, offset)
	query += fmt.Sprintf(` OFFSET $%d`, len(args))

	return r.queryMovements(ctx, query, args...)
}

func (r *movementRepository) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.credit_card_purchase_id = $1 ORDER BY movements.installment_number ASC`,
		purchaseID)
}

func (r *movementRepository) ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error) {
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.transfer_id = $1 ORDER BY movements.amount ASC`,
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

// SumByPlan sums non-voided movements tagged with planID over [from, to]
// (both inclusive) — see the application/repositories contract's own doc
// comment for why "to" is inclusive.
func (r *movementRepository) SumByPlan(ctx context.Context, planID string, from, to *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE plan_id = $1 AND status = 'active'`
	args := []any{planID}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND timestamp <= $%d`, len(args))
	}

	var sum int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&sum); err != nil {
		return 0, fmt.Errorf("postgresql: sum by plan: %w", err)
	}
	return sum, nil
}

func (r *movementRepository) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	clause, excludeArgs := excludedUserIDsClause(excludedUserIDs, 2)
	args := []any{now, now.Add(-retryCooldown)}
	args = append(args, excludeArgs...)
	return r.queryMovements(ctx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+`
		 WHERE movements.status = 'active' AND movements.sync_status IN ('pending', 'failed')
		   AND movements.timestamp <= $1
		   AND (movements.last_sync_attempt_at IS NULL OR movements.last_sync_attempt_at <= $2)`+clause+`
		 ORDER BY movements.timestamp ASC`,
		args...)
}

// MarkLocalPending is BACK-13's "re-enable" path: movements created while
// this user's ledger sync was off (SyncStatusLocal) go back to "pending"
// so the next sync pass picks up the accumulated backlog. Zero matching
// rows is a normal outcome (nothing to reclassify), not an error.
func (r *movementRepository) MarkLocalPending(ctx context.Context, userID string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE movements SET sync_status = 'pending' WHERE user_id = $1 AND sync_status = 'local' AND status = 'active'`,
		userID); err != nil {
		return fmt.Errorf("postgresql: mark local movements pending: %w", err)
	}
	return nil
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

func (r *movementRepository) UpdateMetadata(ctx context.Context, movementID, description string, categoryID *string, paymentMethod string, accountID, planID *string) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements SET description = $1, category_id = $2, payment_method = $3, account_id = $4, plan_id = $5 WHERE id = $6`,
		nullString(description), strOrNil(categoryID), paymentMethod, strOrNil(accountID), strOrNil(planID), movementID)
}

func (r *movementRepository) UpdateAvoidabilityOverride(ctx context.Context, movementID string, avoidabilityOverridePercent *int) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements SET avoidability_override_percent = $1 WHERE id = $2`,
		intOrNil(avoidabilityOverridePercent), movementID)
}

func (r *movementRepository) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return execOnRow(ctx, r.db,
		`UPDATE movements SET amount = $1, currency = $2, timestamp = $3 WHERE id = $4`,
		amount, currency, timestamp, movementID)
}

func (r *movementRepository) Void(ctx context.Context, movementID string) error {
	return execOnRow(ctx, r.db, `UPDATE movements SET status = 'voided' WHERE id = $1`, movementID)
}

func (r *movementRepository) CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
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
func createReversalTx(ctx context.Context, tx *sql.Tx, reversal *dto.MovementDTO) error {
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
		// A concurrent cancel won the race between our SELECT and this
		// UPDATE, so our own reversal insert above is now an orphan: it
		// exists but nothing points at it. Delete it explicitly rather than
		// relying solely on the caller rolling back — CreateReversal must
		// not leave an unlinked reversal row behind even if it's called
		// inside a Transact whose caller went on to commit anyway despite
		// this error.
		if _, delErr := tx.ExecContext(ctx, `DELETE FROM movements WHERE id = $1`, reversal.ID); delErr != nil {
			return fmt.Errorf("postgresql: clean up orphan reversal after conflict: %w", delErr)
		}
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

func (r *movementRepository) queryMovements(ctx context.Context, query string, args ...any) ([]*dto.MovementDTO, error) {
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
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.id = $1`, movementID)
	m, err := scanMovement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return m, err
}

func (r *movementRepositoryTx) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error) {
	query := `SELECT ` + movementSelectColumns + ` FROM ` + movementFromClause + ` WHERE movements.user_id = $1`
	args := []any{userID}
	if currency != nil {
		args = append(args, *currency)
		query += fmt.Sprintf(` AND movements.currency = $%d`, len(args))
	}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND movements.timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND movements.timestamp < $%d`, len(args))
	}
	if limit <= 0 {
		limit = -1
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY movements.timestamp DESC, movements.created_at DESC LIMIT NULLIF($%d, -1)`, len(args))
	args = append(args, offset)
	query += fmt.Sprintf(` OFFSET $%d`, len(args))

	return queryMovements(ctx, r.tx, query, args...)
}

func (r *movementRepositoryTx) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	return queryMovements(ctx, r.tx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.credit_card_purchase_id = $1 ORDER BY movements.installment_number ASC`,
		purchaseID)
}

func (r *movementRepositoryTx) ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error) {
	return queryMovements(ctx, r.tx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+` WHERE movements.transfer_id = $1 ORDER BY movements.amount ASC`,
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

func (r *movementRepositoryTx) SumByPlan(ctx context.Context, planID string, from, to *time.Time) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM movements WHERE plan_id = $1 AND status = 'active'`
	args := []any{planID}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(` AND timestamp >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(` AND timestamp <= $%d`, len(args))
	}
	var sum int64
	if err := r.tx.QueryRowContext(ctx, query, args...).Scan(&sum); err != nil {
		return 0, fmt.Errorf("postgresql: sum by plan: %w", err)
	}
	return sum, nil
}

func (r *movementRepositoryTx) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	clause, excludeArgs := excludedUserIDsClause(excludedUserIDs, 2)
	args := []any{now, now.Add(-retryCooldown)}
	args = append(args, excludeArgs...)
	return queryMovements(ctx, r.tx,
		`SELECT `+movementSelectColumns+` FROM `+movementFromClause+`
		 WHERE movements.status = 'active' AND movements.sync_status IN ('pending', 'failed')
		   AND movements.timestamp <= $1
		   AND (movements.last_sync_attempt_at IS NULL OR movements.last_sync_attempt_at <= $2)`+clause+`
		 ORDER BY movements.timestamp ASC`,
		args...)
}

func (r *movementRepositoryTx) MarkLocalPending(ctx context.Context, userID string) error {
	if _, err := r.tx.ExecContext(ctx,
		`UPDATE movements SET sync_status = 'pending' WHERE user_id = $1 AND sync_status = 'local' AND status = 'active'`,
		userID); err != nil {
		return fmt.Errorf("postgresql: mark local movements pending: %w", err)
	}
	return nil
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

func (r *movementRepositoryTx) UpdateMetadata(ctx context.Context, movementID, description string, categoryID *string, paymentMethod string, accountID, planID *string) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements SET description = $1, category_id = $2, payment_method = $3, account_id = $4, plan_id = $5 WHERE id = $6`,
		nullString(description), strOrNil(categoryID), paymentMethod, strOrNil(accountID), strOrNil(planID), movementID)
}

func (r *movementRepositoryTx) UpdateAvoidabilityOverride(ctx context.Context, movementID string, avoidabilityOverridePercent *int) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements SET avoidability_override_percent = $1 WHERE id = $2`,
		intOrNil(avoidabilityOverridePercent), movementID)
}

func (r *movementRepositoryTx) UpdateFinancial(ctx context.Context, movementID string, amount int64, currency string, timestamp time.Time) error {
	return execOnRow(ctx, r.tx,
		`UPDATE movements SET amount = $1, currency = $2, timestamp = $3 WHERE id = $4`,
		amount, currency, timestamp, movementID)
}

func (r *movementRepositoryTx) Void(ctx context.Context, movementID string) error {
	return execOnRow(ctx, r.tx, `UPDATE movements SET status = 'voided' WHERE id = $1`, movementID)
}

func (r *movementRepositoryTx) CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
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

func queryMovements(ctx context.Context, q queryer, query string, args ...any) ([]*dto.MovementDTO, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query movements: %w", err)
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

func insertMovement(ctx context.Context, ex execer, m *dto.MovementDTO) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO movements (`+movementInsertColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)`,
		m.ID, m.UserID, m.Amount, m.Currency,
		nullString(m.Description), strOrNil(m.CategoryID), m.PaymentMethod,
		strOrNil(m.CreditCardPurchaseID), intOrNil(m.InstallmentNumber),
		m.Status, strOrNil(m.CancelsMovementID), strOrNil(m.ReversedByMovementID),
		m.Timestamp, m.SyncStatus, strOrNil(m.LedgerTransactionID),
		m.SyncAttempts, strOrNil(m.LastSyncError), timeOrNil(m.LastSyncAttemptAt),
		timeOrNil(m.SyncedAt), m.CreatedAt, strOrNil(m.AccountID), strOrNil(m.TransferID), strOrNil(m.PlanID),
		intOrNil(m.AvoidabilityOverridePercent), strOrNil(m.RecurringRuleID))
	if err != nil {
		return fmt.Errorf("postgresql: insert movement: %w", err)
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
		planID                              sql.NullString
		recurringRuleID                     sql.NullString
		installmentNumber                   sql.NullInt64
		syncAttempts                        int64
		lastAttemptAt, syncedAt             sql.NullTime
		avoidabilityOverride                sql.NullInt64
	)

	err := row.Scan(
		&m.ID, &m.UserID, &m.Amount, &m.Currency,
		&description, &m.Category, &categoryID, &m.PaymentMethod,
		&purchaseID, &installmentNumber,
		&m.Status, &cancelsID, &reversedByID,
		&m.Timestamp, &m.SyncStatus, &ledgerTxID,
		&syncAttempts, &lastSyncError, &lastAttemptAt,
		&syncedAt, &m.CreatedAt, &accountID, &transferID, &planID,
		&avoidabilityOverride, &recurringRuleID)
	if err != nil {
		return nil, err
	}

	m.Description = description.String
	m.CategoryID = stringPtr(categoryID)
	m.SyncAttempts = int(syncAttempts)
	m.AccountID = stringPtr(accountID)
	m.TransferID = stringPtr(transferID)
	m.PlanID = stringPtr(planID)
	m.RecurringRuleID = stringPtr(recurringRuleID)
	m.CreditCardPurchaseID = stringPtr(purchaseID)
	m.CancelsMovementID = stringPtr(cancelsID)
	m.ReversedByMovementID = stringPtr(reversedByID)
	m.LedgerTransactionID = stringPtr(ledgerTxID)
	m.LastSyncError = stringPtr(lastSyncError)
	if installmentNumber.Valid {
		n := int(installmentNumber.Int64)
		m.InstallmentNumber = &n
	}
	if avoidabilityOverride.Valid {
		n := int(avoidabilityOverride.Int64)
		m.AvoidabilityOverridePercent = &n
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
