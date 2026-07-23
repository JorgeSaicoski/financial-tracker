package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

type fakeRepo struct {
	movements map[string]*dto.MovementDTO
}

func newFakeRepo(movements ...*dto.MovementDTO) *fakeRepo {
	f := &fakeRepo{movements: map[string]*dto.MovementDTO{}}
	for _, m := range movements {
		f.movements[m.ID] = m
	}
	return f
}

func (f *fakeRepo) Create(context.Context, *dto.MovementDTO) (*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) GetByID(context.Context, string) (*dto.MovementDTO, error) { panic("not used") }
func (f *fakeRepo) CreateBatch(context.Context, []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) ListByTransferID(context.Context, string) ([]*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) ListByUser(context.Context, string, *string, *time.Time, *time.Time, int, int) ([]*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) ListByCreditCardPurchase(context.Context, string) ([]*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) Void(context.Context, string) error { panic("not used") }
func (f *fakeRepo) UpdateMetadata(context.Context, string, string, string, string, *string) error {
	panic("not used")
}
func (f *fakeRepo) UpdateFinancial(context.Context, string, int64, string, time.Time) error {
	panic("not used")
}
func (f *fakeRepo) NetByAccount(context.Context, string, *time.Time, *time.Time) (int64, error) {
	panic("not used")
}
func (f *fakeRepo) CreateReversal(context.Context, *dto.MovementDTO) (*dto.MovementDTO, error) {
	panic("not used")
}
func (f *fakeRepo) Transact(_ context.Context, fn func(repositories.MovementRepository) error) error {
	panic("not used")
}

func (f *fakeRepo) ListPendingSync(_ context.Context, now time.Time, retryCooldown time.Duration) ([]*dto.MovementDTO, error) {
	var out []*dto.MovementDTO
	for _, m := range f.movements {
		if m.Status != string(entities.MovementStatusActive) || m.SyncStatus == string(entities.SyncStatusSynced) {
			continue
		}
		if m.Timestamp.After(now) {
			continue
		}
		if m.LastSyncAttemptAt != nil && m.LastSyncAttemptAt.After(now.Add(-retryCooldown)) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeRepo) MarkSynced(_ context.Context, id, ledgerTransactionID string, at time.Time) error {
	m, ok := f.movements[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.SyncStatus = string(entities.SyncStatusSynced)
	m.LedgerTransactionID = &ledgerTransactionID
	m.SyncedAt = &at
	m.LastSyncAttemptAt = &at
	m.SyncAttempts++
	return nil
}

func (f *fakeRepo) MarkSyncFailed(_ context.Context, id, syncErr string, at time.Time) error {
	m, ok := f.movements[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.SyncStatus = string(entities.SyncStatusFailed)
	m.LastSyncError = &syncErr
	m.LastSyncAttemptAt = &at
	m.SyncAttempts++
	return nil
}

type fakeGateway struct {
	err       error
	published []string
}

func (g *fakeGateway) Publish(_ context.Context, m *dto.MovementDTO) (string, error) {
	if g.err != nil {
		return "", g.err
	}
	g.published = append(g.published, m.ID)
	return "ledger-" + m.ID, nil
}

func pendingMovement(id string, timestamp time.Time) *dto.MovementDTO {
	return &dto.MovementDTO{
		ID:         id,
		UserID:     "u1",
		Amount:     -100,
		Currency:   "usd",
		Status:     string(entities.MovementStatusActive),
		SyncStatus: string(entities.SyncStatusPending),
		Timestamp:  timestamp,
	}
}

func TestRunPassSyncsDueMovements(t *testing.T) {
	now := time.Now().UTC()
	due := pendingMovement("due", now.Add(-time.Hour))
	future := pendingMovement("future", now.Add(24*time.Hour)) // not-yet-due installment
	repo := newFakeRepo(due, future)
	gateway := &fakeGateway{}

	sum := NewService(repo, gateway, logger.New(), time.Minute).RunPass(context.Background())

	if sum.Synced != 1 || sum.Failed != 0 {
		t.Fatalf("summary = %+v, want 1 synced / 0 failed", sum)
	}
	if due.SyncStatus != string(entities.SyncStatusSynced) {
		t.Errorf("due movement sync status = %q", due.SyncStatus)
	}
	if due.LedgerTransactionID == nil || *due.LedgerTransactionID != "ledger-due" {
		t.Error("ledger transaction id not recorded")
	}
	if future.SyncStatus != string(entities.SyncStatusPending) {
		t.Error("future installment must not sync before its date")
	}
}

func TestRunPassRecordsFailures(t *testing.T) {
	now := time.Now().UTC()
	m := pendingMovement("m1", now.Add(-time.Hour))
	repo := newFakeRepo(m)
	gateway := &fakeGateway{err: errors.New("connection refused")}
	service := NewService(repo, gateway, logger.New(), time.Minute)

	sum := service.RunPass(context.Background())
	if sum.Synced != 0 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want 0 synced / 1 failed", sum)
	}
	if m.SyncStatus != string(entities.SyncStatusFailed) || m.SyncAttempts != 1 {
		t.Errorf("movement = %s attempts %d, want failed/1", m.SyncStatus, m.SyncAttempts)
	}
	if m.LastSyncError == nil || *m.LastSyncError != "connection refused" {
		t.Error("sync error not recorded")
	}

	// Cooldown: the background pass skips the fresh failure, the manual
	// pass (POST /sync) does not — and succeeds once the gateway is back.
	gateway.err = nil
	if sum := service.RunPass(context.Background()); sum.Synced != 0 || sum.Failed != 0 {
		t.Fatalf("cooldown pass should skip fresh failure, got %+v", sum)
	}
	if sum := service.RunPassNow(context.Background()); sum.Synced != 1 {
		t.Fatalf("manual pass should retry immediately, got %+v", sum)
	}
	if m.SyncStatus != string(entities.SyncStatusSynced) {
		t.Errorf("movement status = %q, want synced after recovery", m.SyncStatus)
	}
}
