package ledgerservice

import (
	"context"
	"errors"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
	wire "github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// movementRepository implements domain/repositories.MovementRepository by
// delegating every operation to ledger-service. This is the only
// persistence backend financial-tracker has today; a Postgres-backed
// implementation can later satisfy the same interface without any change
// to usecases or handlers.
type movementRepository struct {
	client *Client
}

// NewMovementRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewMovementRepository(client *Client) repositories.MovementRepository {
	return &movementRepository{client: client}
}

func (r *movementRepository) Create(ctx context.Context, movement *entities.Movement) (*entities.Movement, error) {
	tx, err := r.client.CreateTransaction(ctx, wire.TransactionRequest{
		UserID:   movement.UserID,
		Amount:   movement.Amount,
		Currency: movement.Currency,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return tx.ToEntity(), nil
}

func (r *movementRepository) GetByID(ctx context.Context, id string) (*entities.Movement, error) {
	tx, err := r.client.GetTransaction(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return tx.ToEntity(), nil
}

func (r *movementRepository) ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]*entities.Movement, error) {
	txs, err := r.client.ListTransactions(ctx, userID, currency, limit, offset)
	if err != nil {
		return nil, mapError(err)
	}

	out := make([]*entities.Movement, 0, len(txs))
	for _, tx := range txs {
		out = append(out, tx.ToEntity())
	}
	return out, nil
}

// mapError translates ledger-service's HTTP status codes into
// financial-tracker's own error kinds so the API layer never needs to know
// which backend is behind MovementRepository.
func mapError(err error) error {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return apperrors.ErrUpstream
	}

	switch apiErr.StatusCode {
	case http.StatusBadRequest:
		return apperrors.ErrInvalidInput
	case http.StatusNotFound:
		return apperrors.ErrNotFound
	default:
		return apperrors.ErrUpstream
	}
}
