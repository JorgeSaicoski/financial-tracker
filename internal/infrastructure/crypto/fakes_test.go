package crypto

import (
	"context"
	"sync"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// fakeUserDataKeyRepo is an in-memory repositories.UserDataKeyRepository
// used to test FieldCryptor without a real database. Mirrors the real
// repositories' documented contract: Get returns apperrors.ErrNotFound
// when absent; Create is race-safe (first writer wins).
type fakeUserDataKeyRepo struct {
	mu   sync.Mutex
	rows map[string]*dto.UserDataKeyDTO
}

func newFakeUserDataKeyRepo() *fakeUserDataKeyRepo {
	return &fakeUserDataKeyRepo{rows: make(map[string]*dto.UserDataKeyDTO)}
}

func (f *fakeUserDataKeyRepo) Get(_ context.Context, userID string) (*dto.UserDataKeyDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[userID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	copied := *row
	return &copied, nil
}

func (f *fakeUserDataKeyRepo) Create(_ context.Context, row *dto.UserDataKeyDTO) (*dto.UserDataKeyDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.rows[row.UserID]; ok {
		copied := *existing
		return &copied, nil
	}
	copied := *row
	f.rows[row.UserID] = &copied
	return &copied, nil
}

// fakeLedgerPseudonymRepo mirrors fakeUserDataKeyRepo for
// repositories.LedgerPseudonymRepository.
type fakeLedgerPseudonymRepo struct {
	mu   sync.Mutex
	rows map[string]*dto.LedgerPseudonymDTO
}

func newFakeLedgerPseudonymRepo() *fakeLedgerPseudonymRepo {
	return &fakeLedgerPseudonymRepo{rows: make(map[string]*dto.LedgerPseudonymDTO)}
}

func (f *fakeLedgerPseudonymRepo) Get(_ context.Context, userID string) (*dto.LedgerPseudonymDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[userID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	copied := *row
	return &copied, nil
}

func (f *fakeLedgerPseudonymRepo) Create(_ context.Context, row *dto.LedgerPseudonymDTO) (*dto.LedgerPseudonymDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.rows[row.UserID]; ok {
		copied := *existing
		return &copied, nil
	}
	copied := *row
	f.rows[row.UserID] = &copied
	return &copied, nil
}
