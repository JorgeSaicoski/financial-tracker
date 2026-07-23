package repositories

import "context"

// CurrencyRepository is the registry of currency codes the user tracks
// (usd, brl, btc, ...). It exists so the frontend dropdown is data, not
// hardcoded — movements themselves store the code as plain text.
type CurrencyRepository interface {
	List(ctx context.Context) ([]string, error)
	// Add registers a code; adding an existing code is a no-op.
	Add(ctx context.Context, code string) error
	// Decimals returns the number of minor-unit decimal places a
	// registered currency uses (2 for usd/brl, 8 for btc, ...) — needed to
	// scale smallest-unit amounts correctly during exchange-rate
	// conversion (see BACK-11's ToUSDUseCase). apperrors.ErrNotFound if
	// the code isn't registered.
	Decimals(ctx context.Context, code string) (int, error)
}
