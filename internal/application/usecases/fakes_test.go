package usecases

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// fakeCurrencyRepo is an in-memory CurrencyRepository. Add is idempotent,
// matching the real SQLite implementation's INSERT OR IGNORE.
type fakeCurrencyRepo struct {
	codes map[string]bool
}

func newFakeCurrencyRepo(seed ...string) *fakeCurrencyRepo {
	f := &fakeCurrencyRepo{codes: map[string]bool{}}
	for _, c := range seed {
		f.codes[c] = true
	}
	return f
}

func (f *fakeCurrencyRepo) List(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(f.codes))
	for c := range f.codes {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeCurrencyRepo) Add(_ context.Context, code string) error {
	f.codes[code] = true
	return nil
}

// fakeMovementRepo is an in-memory MovementRepository mirroring the
// semantics the SQLite implementation guarantees (see its own tests).
// It stores application/dto types, same as the real contract.
type fakeMovementRepo struct {
	byID              map[string]*dto.MovementDTO
	nextID            int
	updateMetadataErr error
	// createErr, if set, is returned by Create instead of inserting —
	// lets tests simulate the second write of a multi-step Transact
	// (e.g. update_movement's reversal-then-replacement) failing, to
	// verify the first write rolls back with it.
	createErr error
	// createReversalErrForID, if set, makes CreateReversal fail only when
	// reversing this specific movement ID — used to fail the *second*
	// leg of a two-leg cancel while letting the first succeed.
	createReversalErrForID string
}

func newFakeMovementRepo() *fakeMovementRepo {
	return &fakeMovementRepo{byID: map[string]*dto.MovementDTO{}}
}

func (f *fakeMovementRepo) add(m *dto.MovementDTO) *dto.MovementDTO {
	if m.ID == "" {
		f.nextID++
		m.ID = fmt.Sprintf("m-%d", f.nextID)
	}
	cp := *m
	f.byID[m.ID] = &cp
	return m
}

func (f *fakeMovementRepo) Create(_ context.Context, m *dto.MovementDTO) (*dto.MovementDTO, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.add(m), nil
}

func (f *fakeMovementRepo) CreateBatch(_ context.Context, movements []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
	for _, m := range movements {
		f.add(m)
	}
	return movements, nil
}

func (f *fakeMovementRepo) ListByTransferID(_ context.Context, transferID string) ([]*dto.MovementDTO, error) {
	var out []*dto.MovementDTO
	for _, m := range f.byID {
		if m.TransferID != nil && *m.TransferID == transferID {
			cp := *m
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount < out[j].Amount })
	return out, nil
}

func (f *fakeMovementRepo) GetByID(_ context.Context, id string) (*dto.MovementDTO, error) {
	m, ok := f.byID[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeMovementRepo) ListByUser(_ context.Context, userID string, currency *string, from, to *time.Time, _, _ int) ([]*dto.MovementDTO, error) {
	var out []*dto.MovementDTO
	for _, m := range f.byID {
		if m.UserID != userID {
			continue
		}
		if currency != nil && m.Currency != *currency {
			continue
		}
		if from != nil && m.Timestamp.Before(*from) {
			continue
		}
		if to != nil && !m.Timestamp.Before(*to) {
			continue
		}
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeMovementRepo) NetByAccount(_ context.Context, accountID string, after, until *time.Time) (int64, error) {
	var net int64
	for _, m := range f.byID {
		if m.AccountID == nil || *m.AccountID != accountID || m.Status != string(entities.MovementStatusActive) {
			continue
		}
		if after != nil && !m.Timestamp.After(*after) {
			continue
		}
		if until != nil && m.Timestamp.After(*until) {
			continue
		}
		net += m.Amount
	}
	return net, nil
}

func (f *fakeMovementRepo) ListByCreditCardPurchase(_ context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	var out []*dto.MovementDTO
	for _, m := range f.byID {
		if m.CreditCardPurchaseID != nil && *m.CreditCardPurchaseID == purchaseID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeMovementRepo) ListPendingSync(_ context.Context, now time.Time, retryCooldown time.Duration) ([]*dto.MovementDTO, error) {
	var out []*dto.MovementDTO
	for _, m := range f.byID {
		if m.Status != string(entities.MovementStatusActive) || m.SyncStatus == string(entities.SyncStatusSynced) {
			continue
		}
		if m.Timestamp.After(now) {
			continue
		}
		if m.LastSyncAttemptAt != nil && m.LastSyncAttemptAt.After(now.Add(-retryCooldown)) {
			continue
		}
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeMovementRepo) MarkSynced(_ context.Context, id, ledgerTransactionID string, at time.Time) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.SyncStatus = string(entities.SyncStatusSynced)
	m.LedgerTransactionID = &ledgerTransactionID
	m.SyncedAt = &at
	m.LastSyncAttemptAt = &at
	m.LastSyncError = nil
	m.SyncAttempts++
	return nil
}

func (f *fakeMovementRepo) MarkSyncFailed(_ context.Context, id, syncErr string, at time.Time) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.SyncStatus = string(entities.SyncStatusFailed)
	m.LastSyncError = &syncErr
	m.LastSyncAttemptAt = &at
	m.SyncAttempts++
	return nil
}

func (f *fakeMovementRepo) UpdateMetadata(_ context.Context, id, description, category, paymentMethod string, accountID *string) error {
	if f.updateMetadataErr != nil {
		return f.updateMetadataErr
	}
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.Description = description
	m.Category = category
	m.PaymentMethod = paymentMethod
	m.AccountID = accountID
	return nil
}

func (f *fakeMovementRepo) UpdateFinancial(_ context.Context, id string, amount int64, currency string, timestamp time.Time) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.Amount = amount
	m.Currency = currency
	m.Timestamp = timestamp
	return nil
}

func (f *fakeMovementRepo) Void(_ context.Context, id string) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.Status = string(entities.MovementStatusVoided)
	return nil
}

func (f *fakeMovementRepo) CreateReversal(_ context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
	if f.createReversalErrForID != "" && *reversal.CancelsMovementID == f.createReversalErrForID {
		return nil, fmt.Errorf("forced failure reversing %s", f.createReversalErrForID)
	}
	original, ok := f.byID[*reversal.CancelsMovementID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	if original.ReversedByMovementID != nil || original.Status != string(entities.MovementStatusActive) {
		return nil, apperrors.ErrConflict
	}
	reversal = f.add(reversal)
	original.ReversedByMovementID = &reversal.ID
	return reversal, nil
}

// Transact mirrors the real SQLite implementation's all-or-nothing
// guarantee: fn's writes only stick if it returns nil, otherwise the
// repo's state is restored to what it was before fn ran.
func (f *fakeMovementRepo) Transact(_ context.Context, fn func(tx repositories.MovementRepository) error) error {
	snapshot := make(map[string]*dto.MovementDTO, len(f.byID))
	for id, m := range f.byID {
		cp := *m
		snapshot[id] = &cp
	}
	if err := fn(f); err != nil {
		f.byID = snapshot
		return err
	}
	return nil
}

type fakePurchaseRepo struct {
	byID      map[string]*dto.CreditCardPurchaseDTO
	movements *fakeMovementRepo
	nextID    int
}

func newFakePurchaseRepo(movements *fakeMovementRepo) *fakePurchaseRepo {
	return &fakePurchaseRepo{byID: map[string]*dto.CreditCardPurchaseDTO{}, movements: movements}
}

func (f *fakePurchaseRepo) CreateWithInstallments(_ context.Context, purchase *dto.CreditCardPurchaseDTO, installments []*dto.MovementDTO) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error) {
	if purchase.ID == "" {
		f.nextID++
		purchase.ID = fmt.Sprintf("p-%d", f.nextID)
	}
	cp := *purchase
	f.byID[purchase.ID] = &cp
	for _, m := range installments {
		m.CreditCardPurchaseID = &purchase.ID
		f.movements.add(m)
	}
	return purchase, installments, nil
}

func (f *fakePurchaseRepo) GetByID(_ context.Context, id string) (*dto.CreditCardPurchaseDTO, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakePurchaseRepo) MarkCancelled(_ context.Context, id string) error {
	p, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	p.Status = string(entities.CreditCardPurchaseStatusCancelled)
	return nil
}

type fakeSyncTrigger struct {
	calls int
}

func (f *fakeSyncTrigger) TriggerAsync() { f.calls++ }

// fakeAccountRepo is an in-memory AccountRepository.
type fakeAccountRepo struct {
	byID      map[string]*dto.AccountDTO
	snapshots map[string][]*dto.AccountSnapshotDTO
	nextID    int
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		byID:      map[string]*dto.AccountDTO{},
		snapshots: map[string][]*dto.AccountSnapshotDTO{},
	}
}

func (f *fakeAccountRepo) Create(_ context.Context, account *dto.AccountDTO) (*dto.AccountDTO, error) {
	if account.ID == "" {
		f.nextID++
		account.ID = fmt.Sprintf("a-%d", f.nextID)
	}
	cp := *account
	f.byID[account.ID] = &cp
	return account, nil
}

func (f *fakeAccountRepo) GetByID(_ context.Context, id string) (*dto.AccountDTO, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (f *fakeAccountRepo) ListByUser(_ context.Context, userID string) ([]*dto.AccountDTO, error) {
	var out []*dto.AccountDTO
	for _, a := range f.byID {
		if a.UserID != userID {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeAccountRepo) AddSnapshot(_ context.Context, snapshot *dto.AccountSnapshotDTO) (*dto.AccountSnapshotDTO, error) {
	if snapshot.ID == "" {
		f.nextID++
		snapshot.ID = fmt.Sprintf("s-%d", f.nextID)
	}
	cp := *snapshot
	f.snapshots[snapshot.AccountID] = append(f.snapshots[snapshot.AccountID], &cp)
	return snapshot, nil
}

func (f *fakeAccountRepo) LatestSnapshots(_ context.Context, accountID string, n int) ([]*dto.AccountSnapshotDTO, error) {
	snaps := append([]*dto.AccountSnapshotDTO(nil), f.snapshots[accountID]...)
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Timestamp.After(snaps[j].Timestamp) })
	if len(snaps) > n {
		snaps = snaps[:n]
	}
	return snaps, nil
}
