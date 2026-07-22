package ledgerservice

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/services"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	wire "github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice/entities"
)

// gateway adapts Client to the application layer's LedgerGateway port
// (application/services). Only the
// money facts cross the wire — ledger-service's transaction model doesn't
// know about descriptions, categories, or payment methods.
type gateway struct {
	client *Client
}

func NewLedgerGateway(client *Client) services.LedgerGateway {
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
