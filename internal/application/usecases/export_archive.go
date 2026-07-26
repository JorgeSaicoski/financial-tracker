package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type exportArchiveUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
	purchases repositories.CreditCardPurchaseRepository
}

// NewExportArchive returns interface type for dependency injection.
func NewExportArchive(accounts repositories.AccountRepository, movements repositories.MovementRepository, purchases repositories.CreditCardPurchaseRepository) ExportArchiveUseCase {
	return &exportArchiveUseCase{accounts: accounts, movements: movements, purchases: purchases}
}

// Execute gathers everything BACK-15's archive needs to be a complete,
// restorable copy of the user's data — not just movements (BACK-09's CSV
// export is the movements-only subset for the standalone-mode use case).
func (uc *exportArchiveUseCase) Execute(ctx context.Context, userID string) (ArchiveBundle, error) {
	if userID == "" {
		return ArchiveBundle{}, apperrors.ErrInvalidInput
	}

	accounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return ArchiveBundle{}, err
	}
	// limit=0 means "no limit" per MovementRepository.ListByUser's
	// convention — an archive must carry every movement, not a page.
	movements, err := uc.movements.ListByUser(ctx, userID, nil, nil, nil, 0, 0)
	if err != nil {
		return ArchiveBundle{}, err
	}
	purchases, err := uc.purchases.ListByUser(ctx, userID)
	if err != nil {
		return ArchiveBundle{}, err
	}

	return ArchiveBundle{Accounts: accounts, Movements: movements, CreditCardPurchases: purchases}, nil
}
