package crypto

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
)

// encryptingMovementRepository decorates a repositories.MovementRepository
// so movements.description is encrypted before it ever reaches the
// underlying repository and decrypted on every read — the underlying
// repository (and its own SQL) never sees plaintext, and callers of this
// decorator never see ciphertext. Used only for the Postgres ("cloud")
// backend; see cmd/api/main.go.
type encryptingMovementRepository struct {
	inner   repositories.MovementRepository
	cryptor services.FieldCryptor
}

// NewEncryptingMovementRepository wraps inner with BACK-16's field-level
// encryption for movements.description.
func NewEncryptingMovementRepository(inner repositories.MovementRepository, cryptor services.FieldCryptor) repositories.MovementRepository {
	return &encryptingMovementRepository{inner: inner, cryptor: cryptor}
}

func (r *encryptingMovementRepository) Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error) {
	enc, err := r.encrypt(ctx, movement)
	if err != nil {
		return nil, err
	}
	created, err := r.inner.Create(ctx, enc)
	if err != nil {
		return nil, err
	}
	return r.decrypt(ctx, created)
}

func (r *encryptingMovementRepository) CreateBatch(ctx context.Context, movements []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
	enc := make([]*dto.MovementDTO, len(movements))
	for i, m := range movements {
		e, err := r.encrypt(ctx, m)
		if err != nil {
			return nil, err
		}
		enc[i] = e
	}
	created, err := r.inner.CreateBatch(ctx, enc)
	if err != nil {
		return nil, err
	}
	return r.decryptAll(ctx, created)
}

func (r *encryptingMovementRepository) GetByID(ctx context.Context, id string) (*dto.MovementDTO, error) {
	m, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.decrypt(ctx, m)
}

func (r *encryptingMovementRepository) ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error) {
	list, err := r.inner.ListByUser(ctx, userID, currency, from, to, limit, offset)
	if err != nil {
		return nil, err
	}
	return r.decryptAll(ctx, list)
}

func (r *encryptingMovementRepository) ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error) {
	list, err := r.inner.ListByCreditCardPurchase(ctx, purchaseID)
	if err != nil {
		return nil, err
	}
	return r.decryptAll(ctx, list)
}

func (r *encryptingMovementRepository) ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error) {
	list, err := r.inner.ListByTransferID(ctx, transferID)
	if err != nil {
		return nil, err
	}
	return r.decryptAll(ctx, list)
}

func (r *encryptingMovementRepository) NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error) {
	return r.inner.NetByAccount(ctx, accountID, after, until)
}

func (r *encryptingMovementRepository) ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration, excludedUserIDs []string) ([]*dto.MovementDTO, error) {
	list, err := r.inner.ListPendingSync(ctx, now, retryCooldown, excludedUserIDs)
	if err != nil {
		return nil, err
	}
	return r.decryptAll(ctx, list)
}

func (r *encryptingMovementRepository) MarkSynced(ctx context.Context, id, ledgerTransactionID string, at time.Time) error {
	return r.inner.MarkSynced(ctx, id, ledgerTransactionID, at)
}

func (r *encryptingMovementRepository) MarkSyncFailed(ctx context.Context, id, syncErr string, at time.Time) error {
	return r.inner.MarkSyncFailed(ctx, id, syncErr, at)
}

func (r *encryptingMovementRepository) MarkLocalPending(ctx context.Context, userID string) error {
	return r.inner.MarkLocalPending(ctx, userID)
}

func (r *encryptingMovementRepository) SumByPlan(ctx context.Context, planID string, from, to *time.Time) (int64, error) {
	return r.inner.SumByPlan(ctx, planID, from, to)
}

// UpdateMetadata's own signature (unlike Create/CreateReversal) doesn't
// carry the movement's user id, so encrypting the incoming description
// under the right per-user key needs one extra lookup first.
func (r *encryptingMovementRepository) UpdateMetadata(ctx context.Context, id, description string, categoryID *string, paymentMethod string, accountID, planID *string) error {
	existing, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return err
	}
	ciphertext, err := r.cryptor.Encrypt(ctx, existing.UserID, description)
	if err != nil {
		return fmt.Errorf("crypto: encrypt movement description: %w", err)
	}
	return r.inner.UpdateMetadata(ctx, id, ciphertext, categoryID, paymentMethod, accountID, planID)
}

func (r *encryptingMovementRepository) UpdateAvoidabilityOverride(ctx context.Context, id string, avoidabilityOverridePercent *int) error {
	return r.inner.UpdateAvoidabilityOverride(ctx, id, avoidabilityOverridePercent)
}

func (r *encryptingMovementRepository) UpdateFinancial(ctx context.Context, id string, amount int64, currency string, timestamp time.Time) error {
	return r.inner.UpdateFinancial(ctx, id, amount, currency, timestamp)
}

func (r *encryptingMovementRepository) Void(ctx context.Context, id string) error {
	return r.inner.Void(ctx, id)
}

func (r *encryptingMovementRepository) CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error) {
	enc, err := r.encrypt(ctx, reversal)
	if err != nil {
		return nil, err
	}
	created, err := r.inner.CreateReversal(ctx, enc)
	if err != nil {
		return nil, err
	}
	return r.decrypt(ctx, created)
}

func (r *encryptingMovementRepository) Transact(ctx context.Context, fn func(repositories.MovementRepository) error) error {
	return r.inner.Transact(ctx, func(tx repositories.MovementRepository) error {
		return fn(&encryptingMovementRepository{inner: tx, cryptor: r.cryptor})
	})
}

func (r *encryptingMovementRepository) encrypt(ctx context.Context, m *dto.MovementDTO) (*dto.MovementDTO, error) {
	ciphertext, err := r.cryptor.Encrypt(ctx, m.UserID, m.Description)
	if err != nil {
		return nil, fmt.Errorf("crypto: encrypt movement description: %w", err)
	}
	out := *m
	out.Description = ciphertext
	return &out, nil
}

func (r *encryptingMovementRepository) decrypt(ctx context.Context, m *dto.MovementDTO) (*dto.MovementDTO, error) {
	if m == nil {
		return nil, nil
	}
	plaintext, err := r.cryptor.Decrypt(ctx, m.UserID, m.Description)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt movement description: %w", err)
	}
	out := *m
	out.Description = plaintext
	return &out, nil
}

func (r *encryptingMovementRepository) decryptAll(ctx context.Context, list []*dto.MovementDTO) ([]*dto.MovementDTO, error) {
	out := make([]*dto.MovementDTO, len(list))
	for i, m := range list {
		d, err := r.decrypt(ctx, m)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}
