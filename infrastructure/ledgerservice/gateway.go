package ledgerservice

import (
	"context"

	syncapp "github.com/JorgeSaicoski/financial-tracker/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	wire "github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice/entities"
)

// gateway adapts Client to application/sync's LedgerGateway port. Only the
// money facts cross the wire — ledger-service's transaction model doesn't
// know about descriptions, categories, or payment methods.
type gateway struct {
	client *Client
}

func NewLedgerGateway(client *Client) syncapp.LedgerGateway {
	return &gateway{client: client}
}

func (g *gateway) Publish(ctx context.Context, movement *entities.Movement) (string, error) {
	tx, err := g.client.CreateTransaction(ctx, wire.TransactionRequest{
		UserID:   movement.UserID,
		Amount:   movement.Amount,
		Currency: movement.Currency,
	})
	if err != nil {
		return "", err
	}
	return tx.ID, nil
}
