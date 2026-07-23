// migrate-sqlite copies financial-tracker's local SQLite database
// (source of truth in the pre-Postgres/standalone deployments) into a
// target Postgres database faithfully — preserving ids, timestamps, sync
// state, and every link (reversals, installments, transfers, snapshots) —
// so switching a deployment to DB_DRIVER=postgres (INFRA-01) doesn't lose
// history the way re-importing via CSV would.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

// tableCount is one table's source vs. target row count, reported in the
// migration summary and used to detect a partial/mismatched copy.
type tableCount struct {
	table  string
	source int
	target int
}

// preflight refuses to run against a target that already has data unless
// force is set — this is a one-shot copy, not a merge, and silently
// mixing data from two sources would be far worse than a loud refusal.
func preflight(ctx context.Context, dst *sql.DB) error {
	// currencies is excluded: migrations/postgres/003 seeds it with
	// usd/brl unconditionally, so it is never actually empty on a fresh
	// target — checking it here would make preflight refuse every run.
	// It's a soft, idempotent registry (ON CONFLICT DO NOTHING both in
	// the migration and in copyCurrencies below), not ledger data, so
	// pre-existing rows there don't indicate a prior migration.
	tables := []string{"accounts", "account_snapshots", "credit_card_purchases", "movements"}
	var nonEmpty []string
	for _, t := range tables {
		var n int
		if err := dst.QueryRowContext(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return fmt.Errorf("preflight: count %s: %w", t, err)
		}
		if n > 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s (%d rows)", t, n))
		}
	}
	if len(nonEmpty) > 0 {
		return fmt.Errorf("target database already has data in: %v — refusing to migrate into a non-empty target (pass --force to override)", nonEmpty)
	}
	return nil
}

// run performs the whole copy in one Postgres transaction: any error
// leaves the target exactly as it was before (empty, per preflight).
func run(ctx context.Context, src, dst *sql.DB, force bool) ([]tableCount, error) {
	if !force {
		if err := preflight(ctx, dst); err != nil {
			return nil, err
		}
	}

	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin target transaction: %w", err)
	}
	defer tx.Rollback()

	steps := []struct {
		table string
		copy  func(context.Context, *sql.DB, *sql.Tx) (int, int, error)
	}{
		{"currencies", copyCurrencies},
		{"accounts", copyAccounts},
		{"account_snapshots", copyAccountSnapshots},
		{"credit_card_purchases", copyCreditCardPurchases},
		{"movements", copyMovements},
	}

	counts := make([]tableCount, 0, len(steps))
	for _, step := range steps {
		s, d, err := step.copy(ctx, src, tx)
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", step.table, err)
		}
		counts = append(counts, tableCount{step.table, s, d})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	for _, c := range counts {
		if c.source != c.target {
			return counts, fmt.Errorf("row count mismatch in %s: source=%d target=%d", c.table, c.source, c.target)
		}
	}
	return counts, nil
}

// parseTimestamp reads the RFC3339Nano text financial-tracker's SQLite
// schema stores timestamps as (see infrastructure/sqlite's timeLayout).
func parseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func parseNullableTimestamp(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTimestamp(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullableString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

// nullString, strOrNil, intOrNil, and timeOrNil mirror infrastructure/
// postgresql's own unexported helpers of the same names (movement_
// repository.go) — kept local rather than exported from that package
// since this tool is explicitly allowed to bypass the application
// contracts (see BACK-06's ticket), not the other way around.
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

func nullableInt(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

type currencyRow struct {
	Code      string
	CreatedAt time.Time
}

func copyCurrencies(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, int, error) {
	rows, err := src.QueryContext(ctx, `SELECT code, created_at FROM currencies`)
	if err != nil {
		return 0, 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	var all []currencyRow
	for rows.Next() {
		var r currencyRow
		var createdAt string
		if err := rows.Scan(&r.Code, &createdAt); err != nil {
			return 0, 0, fmt.Errorf("scan: %w", err)
		}
		if r.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return 0, 0, fmt.Errorf("parse created_at for %s: %w", r.Code, err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	// ON CONFLICT DO NOTHING: the target's schema migration already seeded
	// usd/brl (migrations/postgres/003), so those codes legitimately exist
	// before this runs — same idempotent handling that migration itself
	// uses, not a sign of pre-existing user data.
	written := 0
	for _, r := range all {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO currencies (code, created_at) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			r.Code, r.CreatedAt); err != nil {
			return len(all), written, fmt.Errorf("insert %s: %w", r.Code, err)
		}
		written++
	}
	return len(all), written, nil
}

func copyAccounts(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, user_id, name, type, currency, created_at FROM accounts`)
	if err != nil {
		return 0, 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	var all []*dto.AccountDTO
	for rows.Next() {
		a := &dto.AccountDTO{}
		var createdAt string
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Type, &a.Currency, &createdAt); err != nil {
			return 0, 0, fmt.Errorf("scan: %w", err)
		}
		if a.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return 0, 0, fmt.Errorf("parse created_at for %s: %w", a.ID, err)
		}
		all = append(all, a)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	written := 0
	for _, a := range all {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO accounts (id, user_id, name, type, currency, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			a.ID, a.UserID, a.Name, a.Type, a.Currency, a.CreatedAt); err != nil {
			return len(all), written, fmt.Errorf("insert %s: %w", a.ID, err)
		}
		written++
	}
	return len(all), written, nil
}

func copyAccountSnapshots(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, account_id, balance, timestamp, created_at FROM account_snapshots`)
	if err != nil {
		return 0, 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	var all []*dto.AccountSnapshotDTO
	for rows.Next() {
		s := &dto.AccountSnapshotDTO{}
		var timestamp, createdAt string
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Balance, &timestamp, &createdAt); err != nil {
			return 0, 0, fmt.Errorf("scan: %w", err)
		}
		if s.Timestamp, err = parseTimestamp(timestamp); err != nil {
			return 0, 0, fmt.Errorf("parse timestamp for %s: %w", s.ID, err)
		}
		if s.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return 0, 0, fmt.Errorf("parse created_at for %s: %w", s.ID, err)
		}
		all = append(all, s)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	written := 0
	for _, s := range all {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO account_snapshots (id, account_id, balance, timestamp, created_at) VALUES ($1, $2, $3, $4, $5)`,
			s.ID, s.AccountID, s.Balance, s.Timestamp, s.CreatedAt); err != nil {
			return len(all), written, fmt.Errorf("insert %s: %w", s.ID, err)
		}
		written++
	}
	return len(all), written, nil
}

func copyCreditCardPurchases(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, user_id, description, category, total_amount, currency, installment_count, purchase_date, status, created_at FROM credit_card_purchases`)
	if err != nil {
		return 0, 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	var all []*dto.CreditCardPurchaseDTO
	for rows.Next() {
		p := &dto.CreditCardPurchaseDTO{}
		var description sql.NullString
		var purchaseDate, createdAt string
		if err := rows.Scan(&p.ID, &p.UserID, &description, &p.Category, &p.TotalAmount, &p.Currency, &p.InstallmentCount, &purchaseDate, &p.Status, &createdAt); err != nil {
			return 0, 0, fmt.Errorf("scan: %w", err)
		}
		p.Description = description.String
		if p.PurchaseDate, err = parseTimestamp(purchaseDate); err != nil {
			return 0, 0, fmt.Errorf("parse purchase_date for %s: %w", p.ID, err)
		}
		if p.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return 0, 0, fmt.Errorf("parse created_at for %s: %w", p.ID, err)
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	written := 0
	for _, p := range all {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO credit_card_purchases (id, user_id, description, category, total_amount, currency, installment_count, purchase_date, status, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			p.ID, p.UserID, nullString(p.Description), p.Category, p.TotalAmount, p.Currency, p.InstallmentCount, p.PurchaseDate, p.Status, p.CreatedAt); err != nil {
			return len(all), written, fmt.Errorf("insert %s: %w", p.ID, err)
		}
		written++
	}
	return len(all), written, nil
}

// movementSelfRef holds the two self-referencing FK values pulled out of
// the first pass so the second pass can wire them up once every id exists
// in the target.
type movementSelfRef struct {
	id                   string
	cancelsMovementID    *string
	reversedByMovementID *string
}

func copyMovements(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, int, error) {
	rows, err := src.QueryContext(ctx, `
		SELECT id, user_id, amount, currency, description, category, payment_method,
		       credit_card_purchase_id, installment_number, status,
		       cancels_movement_id, reversed_by_movement_id, timestamp,
		       sync_status, ledger_transaction_id, sync_attempts, last_sync_error,
		       last_sync_attempt_at, synced_at, created_at, account_id, transfer_id
		FROM movements`)
	if err != nil {
		return 0, 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	var all []*dto.MovementDTO
	var selfRefs []movementSelfRef
	for rows.Next() {
		m := &dto.MovementDTO{}
		var description sql.NullString
		var creditCardPurchaseID, accountID, transferID sql.NullString
		var installmentNumber sql.NullInt64
		var cancelsMovementID, reversedByMovementID sql.NullString
		var timestamp string
		var ledgerTransactionID, lastSyncError sql.NullString
		var lastSyncAttemptAt, syncedAt sql.NullString
		var createdAt string

		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Amount, &m.Currency, &description, &m.Category, &m.PaymentMethod,
			&creditCardPurchaseID, &installmentNumber, &m.Status,
			&cancelsMovementID, &reversedByMovementID, &timestamp,
			&m.SyncStatus, &ledgerTransactionID, &m.SyncAttempts, &lastSyncError,
			&lastSyncAttemptAt, &syncedAt, &createdAt, &accountID, &transferID,
		); err != nil {
			return 0, 0, fmt.Errorf("scan: %w", err)
		}

		m.Description = description.String
		m.CreditCardPurchaseID = nullableString(creditCardPurchaseID)
		m.InstallmentNumber = nullableInt(installmentNumber)
		m.AccountID = nullableString(accountID)
		m.TransferID = nullableString(transferID)
		m.LedgerTransactionID = nullableString(ledgerTransactionID)
		m.LastSyncError = nullableString(lastSyncError)

		if m.Timestamp, err = parseTimestamp(timestamp); err != nil {
			return 0, 0, fmt.Errorf("parse timestamp for %s: %w", m.ID, err)
		}
		if m.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return 0, 0, fmt.Errorf("parse created_at for %s: %w", m.ID, err)
		}
		if m.LastSyncAttemptAt, err = parseNullableTimestamp(lastSyncAttemptAt); err != nil {
			return 0, 0, fmt.Errorf("parse last_sync_attempt_at for %s: %w", m.ID, err)
		}
		if m.SyncedAt, err = parseNullableTimestamp(syncedAt); err != nil {
			return 0, 0, fmt.Errorf("parse synced_at for %s: %w", m.ID, err)
		}

		all = append(all, m)
		selfRefs = append(selfRefs, movementSelfRef{
			id:                   m.ID,
			cancelsMovementID:    nullableString(cancelsMovementID),
			reversedByMovementID: nullableString(reversedByMovementID),
		})
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	// Pass 1: insert every movement with the self-referencing FKs left
	// NULL — the row a reversal points at may not exist in the target yet
	// (source order is arbitrary), so those two columns can't be set
	// until every id has landed.
	written := 0
	for _, m := range all {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO movements (
				id, user_id, amount, currency, description, category, payment_method,
				credit_card_purchase_id, installment_number, status,
				cancels_movement_id, reversed_by_movement_id, timestamp,
				sync_status, ledger_transaction_id, sync_attempts, last_sync_error,
				last_sync_attempt_at, synced_at, created_at, account_id, transfer_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10,
				NULL, NULL, $11,
				$12, $13, $14, $15,
				$16, $17, $18, $19, $20
			)`,
			m.ID, m.UserID, m.Amount, m.Currency, nullString(m.Description), m.Category, m.PaymentMethod,
			strOrNil(m.CreditCardPurchaseID), intOrNil(m.InstallmentNumber), m.Status,
			m.Timestamp,
			m.SyncStatus, strOrNil(m.LedgerTransactionID), m.SyncAttempts, strOrNil(m.LastSyncError),
			timeOrNil(m.LastSyncAttemptAt), timeOrNil(m.SyncedAt), m.CreatedAt, strOrNil(m.AccountID), strOrNil(m.TransferID),
		); err != nil {
			return len(all), written, fmt.Errorf("insert %s: %w", m.ID, err)
		}
		written++
	}

	// Pass 2: wire up the self-referencing FKs now that every id exists.
	for _, ref := range selfRefs {
		if ref.cancelsMovementID == nil && ref.reversedByMovementID == nil {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE movements SET cancels_movement_id = $1, reversed_by_movement_id = $2 WHERE id = $3`,
			strOrNil(ref.cancelsMovementID), strOrNil(ref.reversedByMovementID), ref.id); err != nil {
			return len(all), written, fmt.Errorf("link reversal for %s: %w", ref.id, err)
		}
	}

	return len(all), written, nil
}
