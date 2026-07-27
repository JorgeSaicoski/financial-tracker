package usecases

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// fakeCurrencyRepo is an in-memory CurrencyRepository. Add is idempotent,
// matching the real SQLite implementation's INSERT OR IGNORE. Every seeded
// or added code defaults to 2 decimals (the real table's DEFAULT); tests
// that need a different value (e.g. a 0-decimal currency) call
// setDecimals.
type fakeCurrencyRepo struct {
	codes    map[string]bool
	decimals map[string]int
}

func newFakeCurrencyRepo(seed ...string) *fakeCurrencyRepo {
	f := &fakeCurrencyRepo{codes: map[string]bool{}, decimals: map[string]int{}}
	for _, c := range seed {
		f.codes[c] = true
		f.decimals[c] = 2
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
	if _, ok := f.decimals[code]; !ok {
		f.decimals[code] = 2
	}
	return nil
}

func (f *fakeCurrencyRepo) Decimals(_ context.Context, code string) (int, error) {
	if !f.codes[code] {
		return 0, apperrors.ErrNotFound
	}
	return f.decimals[code], nil
}

func (f *fakeCurrencyRepo) setDecimals(code string, n int) {
	f.decimals[code] = n
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

func (f *fakeMovementRepo) ListPendingSync(_ context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	excluded := make(map[string]bool, len(excludedUserIDs))
	for _, uid := range excludedUserIDs {
		excluded[uid] = true
	}
	var out []*dto.MovementDTO
	for _, m := range f.byID {
		if m.Status != string(entities.MovementStatusActive) || (m.SyncStatus != string(entities.SyncStatusPending) && m.SyncStatus != string(entities.SyncStatusFailed)) {
			continue
		}
		if excluded[m.UserID] {
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

func (f *fakeMovementRepo) MarkLocalPending(_ context.Context, userID string) error {
	for _, m := range f.byID {
		if m.UserID == userID && m.SyncStatus == string(entities.SyncStatusLocal) {
			m.SyncStatus = string(entities.SyncStatusPending)
		}
	}
	return nil
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

func (f *fakeMovementRepo) UpdateAvoidabilityOverride(_ context.Context, id string, avoidabilityOverridePercent *int) error {
	m, ok := f.byID[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	m.AvoidabilityOverridePercent = avoidabilityOverridePercent
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

func (f *fakePurchaseRepo) ListByUser(_ context.Context, userID string) ([]*dto.CreditCardPurchaseDTO, error) {
	var out []*dto.CreditCardPurchaseDTO
	for _, p := range f.byID {
		if p.UserID != userID {
			continue
		}
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PurchaseDate.After(out[j].PurchaseDate) })
	return out, nil
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

// fakeExchangeRateRepo is an in-memory ExchangeRateRepository. Create
// upserts on (user_id, currency, effective_from), matching the real
// SQLite/Postgres implementations' ON CONFLICT behavior.
type fakeExchangeRateRepo struct {
	byID   map[string]*dto.ExchangeRateDTO
	nextID int
}

func newFakeExchangeRateRepo() *fakeExchangeRateRepo {
	return &fakeExchangeRateRepo{byID: map[string]*dto.ExchangeRateDTO{}}
}

func (f *fakeExchangeRateRepo) Create(_ context.Context, rate *dto.ExchangeRateDTO) (*dto.ExchangeRateDTO, error) {
	for _, existing := range f.byID {
		if existing.UserID == rate.UserID && existing.Currency == rate.Currency && existing.EffectiveFrom.Equal(rate.EffectiveFrom) {
			existing.UnitsPerUSD = rate.UnitsPerUSD
			cp := *existing
			return &cp, nil
		}
	}
	if rate.ID == "" {
		f.nextID++
		rate.ID = fmt.Sprintf("er-%d", f.nextID)
	}
	cp := *rate
	f.byID[rate.ID] = &cp
	out := *rate
	return &out, nil
}

func (f *fakeExchangeRateRepo) ListByUser(_ context.Context, userID string) ([]*dto.ExchangeRateDTO, error) {
	var out []*dto.ExchangeRateDTO
	for _, r := range f.byID {
		if r.UserID != userID {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Currency != out[j].Currency {
			return out[i].Currency < out[j].Currency
		}
		return out[i].EffectiveFrom.After(out[j].EffectiveFrom)
	})
	return out, nil
}

func (f *fakeExchangeRateRepo) RateAt(_ context.Context, userID, currency string, at time.Time) (*dto.ExchangeRateDTO, error) {
	var best *dto.ExchangeRateDTO
	for _, r := range f.byID {
		if r.UserID != userID || r.Currency != currency {
			continue
		}
		if r.EffectiveFrom.After(at) {
			continue
		}
		if best == nil || r.EffectiveFrom.After(best.EffectiveFrom) {
			best = r
		}
	}
	if best == nil {
		return nil, apperrors.ErrNotFound
	}
	cp := *best
	return &cp, nil
}

func (f *fakeExchangeRateRepo) Delete(_ context.Context, userID, id string) error {
	r, ok := f.byID[id]
	if !ok || r.UserID != userID {
		return apperrors.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}

// fakeLocalArchiveSettingsRepo is an in-memory LocalArchiveSettingsRepository.
// A user with no entry defaults to false, matching the real SQLite/Postgres
// implementations' "no row yet" behavior.
type fakeLocalArchiveSettingsRepo struct {
	enabled map[string]bool
}

func newFakeLocalArchiveSettingsRepo() *fakeLocalArchiveSettingsRepo {
	return &fakeLocalArchiveSettingsRepo{enabled: map[string]bool{}}
}

func (f *fakeLocalArchiveSettingsRepo) IsEnabled(_ context.Context, userID string) (bool, error) {
	return f.enabled[userID], nil
}

func (f *fakeLocalArchiveSettingsRepo) SetEnabled(_ context.Context, userID string, enabled bool) error {
	f.enabled[userID] = enabled
	return nil
}

// fakeUserSettingsRepo is an in-memory UserSettingsRepository. Absence of
// a row means "everything true", mirroring the real implementations'
// contract — Get never creates a row, only UpdateEnabled does.
type fakeUserSettingsRepo struct {
	byUserID map[string]*dto.UserSettingsDTO
}

func newFakeUserSettingsRepo() *fakeUserSettingsRepo {
	return &fakeUserSettingsRepo{byUserID: map[string]*dto.UserSettingsDTO{}}
}

func (f *fakeUserSettingsRepo) Get(_ context.Context, userID string) (*dto.UserSettingsDTO, error) {
	if s, ok := f.byUserID[userID]; ok {
		cp := *s
		return &cp, nil
	}
	return dto.DefaultUserSettings(userID, time.Now().UTC()), nil
}

func (f *fakeUserSettingsRepo) UpdateEnabled(_ context.Context, userID string, ledgerSyncEnabled bool) (*dto.UserSettingsDTO, error) {
	s, ok := f.byUserID[userID]
	if !ok {
		now := time.Now().UTC()
		s = dto.DefaultUserSettings(userID, now)
	}
	s.LedgerSyncEnabled = ledgerSyncEnabled
	s.UpdatedAt = time.Now().UTC()
	f.byUserID[userID] = s
	cp := *s
	return &cp, nil
}

func (f *fakeUserSettingsRepo) ListSyncDisabledUserIDs(_ context.Context) ([]string, error) {
	var out []string
	for uid, s := range f.byUserID {
		if !s.EffectiveLedgerSync() {
			out = append(out, uid)
		}
	}
	sort.Strings(out)
	return out, nil
}

// setEntitled is a test-only helper simulating the operator flipping an
// entitlement flag directly (v1 has no API for this — see README).
func (f *fakeUserSettingsRepo) setEntitled(userID string, ledgerSyncEntitled bool) {
	s, ok := f.byUserID[userID]
	if !ok {
		now := time.Now().UTC()
		s = dto.DefaultUserSettings(userID, now)
		f.byUserID[userID] = s
	}
	s.LedgerSyncEntitled = ledgerSyncEntitled
}

// fakeCategoryRepo is an in-memory CategoryRepository, mirroring the
// semantics the SQLite implementation guarantees: EnsureByName is a
// case-insensitive check-then-insert, Update/Delete/GetByID are scoped
// to (userID, id) and return apperrors.ErrNotFound otherwise.
type fakeCategoryRepo struct {
	byID   map[string]*dto.CategoryDTO
	nextID int
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{byID: map[string]*dto.CategoryDTO{}}
}

func (f *fakeCategoryRepo) EnsureByName(_ context.Context, userID, name string, avoidabilityPercent *int) (*dto.CategoryDTO, error) {
	for _, c := range f.byID {
		if c.UserID == userID && strings.EqualFold(c.Name, name) {
			cp := *c
			return &cp, nil
		}
	}
	return f.Create(context.Background(), &dto.CategoryDTO{
		UserID:              userID,
		Name:                name,
		AvoidabilityPercent: avoidabilityPercent,
	})
}

func (f *fakeCategoryRepo) GetByID(_ context.Context, userID, id string) (*dto.CategoryDTO, error) {
	c, ok := f.byID[id]
	if !ok || c.UserID != userID {
		return nil, apperrors.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (f *fakeCategoryRepo) ListByUser(_ context.Context, userID string) ([]*dto.CategoryDTO, error) {
	var out []*dto.CategoryDTO
	for _, c := range f.byID {
		if c.UserID == userID {
			cp := *c
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeCategoryRepo) Create(_ context.Context, c *dto.CategoryDTO) (*dto.CategoryDTO, error) {
	if c.ID == "" {
		f.nextID++
		c.ID = fmt.Sprintf("cat-%d", f.nextID)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	cp := *c
	f.byID[c.ID] = &cp
	return c, nil
}

func (f *fakeCategoryRepo) Update(_ context.Context, userID, id, name string, avoidabilityPercent *int) error {
	c, ok := f.byID[id]
	if !ok || c.UserID != userID {
		return apperrors.ErrNotFound
	}
	c.Name = name
	c.AvoidabilityPercent = avoidabilityPercent
	return nil
}

func (f *fakeCategoryRepo) Delete(_ context.Context, userID, id string) error {
	c, ok := f.byID[id]
	if !ok || c.UserID != userID {
		return apperrors.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}
