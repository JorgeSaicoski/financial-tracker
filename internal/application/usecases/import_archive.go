package usecases

import (
	"context"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// importArchiveUseCase restores a BACK-15 archive onto whatever local
// state already exists. It is idempotent by ID: a row whose ID is already
// present is left untouched (counted "skipped"), not overwritten — restore
// is a disaster-recovery path, not a merge/sync tool, so it never risks
// clobbering newer local data with an older archive's copy.
//
// Order matters: accounts and credit-card purchases are restored before
// movements, since movements can reference either by ID and both tables
// enforce that with a foreign key.
type importArchiveUseCase struct {
	accounts   repositories.AccountRepository
	movements  repositories.MovementRepository
	purchases  repositories.CreditCardPurchaseRepository
	categories repositories.CategoryRepository
}

// NewImportArchive returns interface type for dependency injection.
func NewImportArchive(accounts repositories.AccountRepository, movements repositories.MovementRepository, purchases repositories.CreditCardPurchaseRepository, categories repositories.CategoryRepository) ImportArchiveUseCase {
	return &importArchiveUseCase{accounts: accounts, movements: movements, purchases: purchases, categories: categories}
}

func (uc *importArchiveUseCase) Execute(ctx context.Context, userID string, bundle ArchiveBundle) (ImportArchiveResult, error) {
	if userID == "" {
		return ImportArchiveResult{}, apperrors.ErrInvalidInput
	}

	// Existing IDs are fetched once per entity type up front — three
	// queries total — instead of one GetByID per bundle row. A restored
	// archive can carry years of movements, so an O(n) round-trip per row
	// would make large restores very slow.
	existingAccounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return ImportArchiveResult{}, err
	}
	existingAccountIDs := toIDSet(existingAccounts, func(a *dto.AccountDTO) string { return a.ID })

	existingPurchases, err := uc.purchases.ListByUser(ctx, userID)
	if err != nil {
		return ImportArchiveResult{}, err
	}
	existingPurchaseIDs := toIDSet(existingPurchases, func(p *dto.CreditCardPurchaseDTO) string { return p.ID })

	existingMovements, err := uc.movements.ListByUser(ctx, userID, nil, nil, nil, 0, 0)
	if err != nil {
		return ImportArchiveResult{}, err
	}
	existingMovementIDs := toIDSet(existingMovements, func(m *dto.MovementDTO) string { return m.ID })

	// The movement/purchase repositories now resolve Category by name
	// against the registry at insert time (BACK-14 follow-up: category_id
	// is a real FK) — unlike the normal create paths, restore writes the
	// archive's category names directly, so every distinct one must be
	// registered here first or the batch insert below fails outright.
	// EnsureByName's neutral 50% default matches what implicit
	// registration on the normal write path already does.
	categoryNames := make(map[string]bool)
	for _, m := range bundle.Movements {
		if m != nil && m.Category != "" {
			categoryNames[m.Category] = true
		}
	}
	for _, p := range bundle.CreditCardPurchases {
		if p != nil && p.Category != "" {
			categoryNames[p.Category] = true
		}
	}
	for name := range categoryNames {
		var avoidabilityPercent *int
		if !isSystemCategory(name) {
			v := defaultCategoryAvoidability
			avoidabilityPercent = &v
		}
		if _, err := uc.categories.EnsureByName(ctx, userID, name, avoidabilityPercent); err != nil {
			return ImportArchiveResult{}, err
		}
	}

	var result ImportArchiveResult

	// allowedAccountIDs/allowedPurchaseIDs start as *copies* of the
	// existing-ID sets (maps alias their backing storage, and existingXIDs
	// is still used below to decide restored-vs-skipped) and grow as the
	// bundle's own accounts/purchases are validated — by the time
	// movements are restored, both sets cover every account/purchase
	// userID will own by the end of this call, whether pre-existing or
	// newly created here.
	allowedAccountIDs := make(map[string]bool, len(existingAccountIDs))
	for id := range existingAccountIDs {
		allowedAccountIDs[id] = true
	}
	allowedPurchaseIDs := make(map[string]bool, len(existingPurchaseIDs))
	for id := range existingPurchaseIDs {
		allowedPurchaseIDs[id] = true
	}

	for _, a := range bundle.Accounts {
		if a == nil || a.ID == "" {
			return ImportArchiveResult{}, fmt.Errorf("%w: account missing id", apperrors.ErrInvalidInput)
		}
		a.UserID = userID
		allowedAccountIDs[a.ID] = true
		if existingAccountIDs[a.ID] {
			result.AccountsSkipped++
			continue
		}
		if _, err := uc.accounts.Create(ctx, a); err != nil {
			return ImportArchiveResult{}, err
		}
		result.AccountsRestored++
	}

	for _, p := range bundle.CreditCardPurchases {
		if p == nil || p.ID == "" {
			return ImportArchiveResult{}, fmt.Errorf("%w: credit card purchase missing id", apperrors.ErrInvalidInput)
		}
		p.UserID = userID
		allowedPurchaseIDs[p.ID] = true
		if existingPurchaseIDs[p.ID] {
			result.CreditCardPurchasesSkipped++
			continue
		}
		// nil installments: the purchase's installment movements are
		// restored separately below, from bundle.Movements — inserting
		// them here too would duplicate them.
		if _, _, err := uc.purchases.CreateWithInstallments(ctx, p, nil); err != nil {
			return ImportArchiveResult{}, err
		}
		result.CreditCardPurchasesRestored++
	}

	var toCreate []*dto.MovementDTO
	for _, m := range bundle.Movements {
		if m == nil || m.ID == "" {
			return ImportArchiveResult{}, fmt.Errorf("%w: movement missing id", apperrors.ErrInvalidInput)
		}
		m.UserID = userID
		if existingMovementIDs[m.ID] {
			result.MovementsSkipped++
			continue
		}
		// A movement's AccountID/CreditCardPurchaseID must resolve to an
		// account/purchase userID actually owns (pre-existing or restored
		// earlier in this same call) — otherwise a hand-crafted archive
		// body could attach a movement to another user's account by
		// naming its id, the exact gap create_movement.go's ownership
		// check (BACK-02) exists to close for the normal write path.
		if m.AccountID != nil && !allowedAccountIDs[*m.AccountID] {
			return ImportArchiveResult{}, fmt.Errorf("%w: movement references an account not owned by this user", apperrors.ErrInvalidInput)
		}
		if m.CreditCardPurchaseID != nil && !allowedPurchaseIDs[*m.CreditCardPurchaseID] {
			return ImportArchiveResult{}, fmt.Errorf("%w: movement references a credit card purchase not owned by this user", apperrors.ErrInvalidInput)
		}
		// CancelsMovementID/ReversedByMovementID are dropped on restore:
		// both are self-referencing foreign keys on movements, checked
		// immediately (not deferred) by both SQLite and Postgres here, and
		// an original/reversal pair references each other in opposite
		// directions — no insertion order satisfies both within one
		// batch. Everything else about both rows (amount, status,
		// currency, ...) restores exactly; only the explicit cross-link
		// between them is lost. A documented limitation, not an oversight.
		m.CancelsMovementID = nil
		m.ReversedByMovementID = nil
		toCreate = append(toCreate, m)
	}
	if len(toCreate) > 0 {
		if _, err := uc.movements.CreateBatch(ctx, toCreate); err != nil {
			return ImportArchiveResult{}, err
		}
		result.MovementsRestored = len(toCreate)
	}

	return result, nil
}

// toIDSet builds a membership set from a slice using keyFn to extract each
// element's ID — shared by Execute's three up-front existence prefetches.
func toIDSet[T any](items []T, keyFn func(T) string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[keyFn(item)] = true
	}
	return set
}
