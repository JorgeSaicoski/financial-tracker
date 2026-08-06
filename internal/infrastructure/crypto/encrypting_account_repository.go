package crypto

import (
	"context"
	"fmt"
	"sort"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
)

// encryptingAccountRepository decorates a repositories.AccountRepository
// so accounts.name is encrypted before it ever reaches the underlying
// repository and decrypted on every read. Used only for the Postgres
// ("cloud") backend; see cmd/api/main.go.
type encryptingAccountRepository struct {
	inner   repositories.AccountRepository
	cryptor services.FieldCryptor
}

// NewEncryptingAccountRepository wraps inner with BACK-16's field-level
// encryption for accounts.name.
func NewEncryptingAccountRepository(inner repositories.AccountRepository, cryptor services.FieldCryptor) repositories.AccountRepository {
	return &encryptingAccountRepository{inner: inner, cryptor: cryptor}
}

func (r *encryptingAccountRepository) Create(ctx context.Context, account *dto.AccountDTO) (*dto.AccountDTO, error) {
	ciphertext, err := r.cryptor.Encrypt(ctx, account.UserID, account.Name)
	if err != nil {
		return nil, fmt.Errorf("crypto: encrypt account name: %w", err)
	}
	enc := *account
	enc.Name = ciphertext
	created, err := r.inner.Create(ctx, &enc)
	if err != nil {
		return nil, err
	}
	return r.decrypt(ctx, created)
}

func (r *encryptingAccountRepository) GetByID(ctx context.Context, id string) (*dto.AccountDTO, error) {
	a, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.decrypt(ctx, a)
}

// ListByUser decrypts every row's name, then re-sorts alphabetically in
// Go: the underlying repository's own `ORDER BY name` sorted ciphertext
// (each encryption uses a random nonce, so it carries no relationship to
// the plaintext's alphabetical order), which would otherwise return
// accounts in an effectively random order.
func (r *encryptingAccountRepository) ListByUser(ctx context.Context, userID string) ([]*dto.AccountDTO, error) {
	list, err := r.inner.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.AccountDTO, len(list))
	for i, a := range list {
		d, err := r.decrypt(ctx, a)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *encryptingAccountRepository) AddSnapshot(ctx context.Context, snapshot *dto.AccountSnapshotDTO) (*dto.AccountSnapshotDTO, error) {
	return r.inner.AddSnapshot(ctx, snapshot)
}

func (r *encryptingAccountRepository) LatestSnapshots(ctx context.Context, accountID string, n int) ([]*dto.AccountSnapshotDTO, error) {
	return r.inner.LatestSnapshots(ctx, accountID, n)
}

func (r *encryptingAccountRepository) decrypt(ctx context.Context, a *dto.AccountDTO) (*dto.AccountDTO, error) {
	if a == nil {
		return nil, nil
	}
	plaintext, err := r.cryptor.Decrypt(ctx, a.UserID, a.Name)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt account name: %w", err)
	}
	out := *a
	out.Name = plaintext
	return &out, nil
}
