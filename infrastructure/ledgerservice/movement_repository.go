package ledgerservice

import (
	"context"
	"errors"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// MovementRepository implements application/repositories.MovementRepository
// by delegating every operation to ledger-service. This is the only
// persistence backend financial-tracker has today; a Postgres-backed
// implementation can later satisfy the same interface without any change
// to usecases or handlers.
type MovementRepository struct {
	client *Client
}

func NewMovementRepository(client *Client) *MovementRepository {
	return &MovementRepository{client: client}
}

func (r *MovementRepository) Create(ctx context.Context, input dto.CreateMovementInput) (dto.MovementOutput, error) {
	tx, err := r.client.CreateTransaction(ctx, transactionRequest{
		UserID:   input.UserID,
		Amount:   input.Amount,
		Currency: input.Currency,
	})
	if err != nil {
		return dto.MovementOutput{}, mapError(err)
	}
	return toMovementOutput(tx), nil
}

func (r *MovementRepository) GetByID(ctx context.Context, id string) (dto.MovementOutput, error) {
	tx, err := r.client.GetTransaction(ctx, id)
	if err != nil {
		return dto.MovementOutput{}, mapError(err)
	}
	return toMovementOutput(tx), nil
}

func (r *MovementRepository) ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]dto.MovementOutput, error) {
	txs, err := r.client.ListTransactions(ctx, userID, currency, limit, offset)
	if err != nil {
		return nil, mapError(err)
	}

	out := make([]dto.MovementOutput, 0, len(txs))
	for _, tx := range txs {
		out = append(out, toMovementOutput(tx))
	}
	return out, nil
}

func toMovementOutput(tx transaction) dto.MovementOutput {
	return dto.MovementOutput{
		ID:        tx.ID,
		UserID:    tx.UserID,
		Amount:    tx.Amount,
		Currency:  tx.Currency,
		Timestamp: tx.Timestamp,
	}
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
