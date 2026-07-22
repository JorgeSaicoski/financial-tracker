package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// fakeMovementRepo is an in-memory MovementRepository mirroring the
// semantics the SQLite implementation guarantees (see its own tests).
type fakeMovementRepo struct {
	byID   map[string]*entities.Movement
	nextID int
}

func newFakeMovementRepo() *fakeMovementRepo {
	return &fakeMovementRepo{byID: map[string]*entities.Movement{}}
}

func (f *fakeMovementRepo) add(m *entities.Movement) *entities.Movement {
	if m.ID == "" {
		f.nextID++
		m.ID = fmt.Sprintf("m-%d", f.nextID)
	}
	cp := *m
	f.byID[m.ID] = &cp
	return m
}

func (f *fakeMovementRepo) Create(_ context.Context, m *entities.Movement) (*entities.Movement, error) {
	return f.add(m), nil
}

func (f *fakeMovementRepo) GetByID(_ context.Context, id string) (*entities.Movement, error) {
	m, ok := f.byID[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeMovementRepo) ListByUser(_ context.Context, userID string, currency *string, _, _ int) ([]*entities.Movement, error) {
	var out []*entities.Movement
	for _, m := range f.byID {
		if m.UserID != userID {
			continue
		}
		if currency != nil && m.Currency != *currency {
			continue
		}
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeMovementRepo) ListByCreditCardPurchase(_ context.Context, purchaseID string) ([]*entities.Movement, error) {
	var out []*entities.Movement
	for _, m := range f.byID {
		if m.CreditCardPurchaseID != nil && *m.CreditCardPurchaseID == purchaseID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeMovementRepo) ListPendingSync(_ context.Context, now time.Time, retryCooldown time.Duration) ([]*entities.Movement, error) {
	var out []*entities.Movement
	for _, m := range f.byID {
		if m.Status != entities.MovementStatusActive || m.SyncStatus == entities.SyncStatusSynced {
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
	m.SyncStatus = entities.SyncStatusSynced
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
	m.SyncStatus = entities.SyncStatusFailed
	m.LastSyncError = &syncErr
	m.LastSyncAttemptAt = &at
	m.SyncAttempts++
	return nil
}

func (f *fakeMovementRepo) Void(_ context.Context, id string) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.Status = entities.MovementStatusVoided
	return nil
}

func (f *fakeMovementRepo) CreateReversal(_ context.Context, reversal *entities.Movement) (*entities.Movement, error) {
	original, ok := f.byID[*reversal.CancelsMovementID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	if original.ReversedByMovementID != nil || original.Status != entities.MovementStatusActive {
		return nil, apperrors.ErrConflict
	}
	reversal = f.add(reversal)
	original.ReversedByMovementID = &reversal.ID
	return reversal, nil
}

type fakePurchaseRepo struct {
	byID      map[string]*entities.CreditCardPurchase
	movements *fakeMovementRepo
	nextID    int
}

func newFakePurchaseRepo(movements *fakeMovementRepo) *fakePurchaseRepo {
	return &fakePurchaseRepo{byID: map[string]*entities.CreditCardPurchase{}, movements: movements}
}

func (f *fakePurchaseRepo) CreateWithInstallments(_ context.Context, purchase *entities.CreditCardPurchase, installments []*entities.Movement) (*entities.CreditCardPurchase, []*entities.Movement, error) {
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

func (f *fakePurchaseRepo) GetByID(_ context.Context, id string) (*entities.CreditCardPurchase, error) {
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
	p.Status = entities.CreditCardPurchaseStatusCancelled
	return nil
}

type fakeSyncTrigger struct {
	calls int
}

func (f *fakeSyncTrigger) TriggerAsync() { f.calls++ }
