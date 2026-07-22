package repositories

import "context"

// CurrencyRepository is the registry of currency codes the user tracks
// (usd, brl, btc, ...). It exists so the frontend dropdown is data, not
// hardcoded — movements themselves store the code as plain text.
type CurrencyRepository interface {
	List(ctx context.Context) ([]string, error)
	// Add registers a code; adding an existing code is a no-op.
	Add(ctx context.Context, code string) error
}
