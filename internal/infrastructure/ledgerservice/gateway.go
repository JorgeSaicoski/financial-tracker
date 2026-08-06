package ledgerservice

import (
	"context"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	wire "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/ledgerservice/entities"
)

// gateway adapts Client to the application layer's LedgerGateway port
// (application/services). Only the
// money facts cross the wire — ledger-service's transaction model doesn't
// know about descriptions, categories, or payment methods. pseudonymizer
// (BACK-16) swaps the real user id and plain currency code for a
// pseudonym UUID and a deterministic HMAC token right at this narrowing
// point — nothing upstream of Publish needs to change, and ledger-service
// itself never sees the real values.
type gateway struct {
	client        *Client
	pseudonymizer services.LedgerPseudonymizer
}

func NewLedgerGateway(client *Client, pseudonymizer services.LedgerPseudonymizer) services.LedgerGateway {
	return &gateway{client: client, pseudonymizer: pseudonymizer}
}

func (g *gateway) Publish(ctx context.Context, movement *dto.MovementDTO) (string, error) {
	pseudonym, err := g.pseudonymizer.PseudonymFor(ctx, movement.UserID)
	if err != nil {
		return "", fmt.Errorf("ledgerservice: resolve pseudonym: %w", err)
	}
	currencyToken, err := g.pseudonymizer.TokenizeCurrency(ctx, movement.UserID, movement.Currency)
	if err != nil {
		return "", fmt.Errorf("ledgerservice: tokenize currency: %w", err)
	}

	tx, err := g.client.CreateTransaction(ctx, wire.TransactionRequest{
		UserID:   pseudonym,
		Amount:   movement.Amount,
		Currency: currencyToken,
	})
	if err != nil {
		return "", err
	}
	return tx.ID, nil
}
