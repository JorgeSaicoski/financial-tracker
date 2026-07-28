package usecases

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type importMovementsUseCase struct {
	movements  repositories.MovementRepository
	accounts   repositories.AccountRepository
	currencies repositories.CurrencyRepository
	methods    repositories.PaymentMethodRepository
	categories repositories.CategoryRepository
}

// NewImportMovements returns interface type for dependency injection.
func NewImportMovements(movements repositories.MovementRepository, accounts repositories.AccountRepository, currencies repositories.CurrencyRepository, methods repositories.PaymentMethodRepository, categories repositories.CategoryRepository) ImportMovementsUseCase {
	return &importMovementsUseCase{movements: movements, accounts: accounts, currencies: currencies, methods: methods, categories: categories}
}

// validRow is a CSV row that passed every field-level check, ready to
// become a movement — kept separate from the raw ImportRowInput so
// validation and insertion never share mutable state.
type validRow struct {
	rowNum        int
	timestamp     time.Time
	amount        int64
	currency      string
	description   string
	categoryID    *string
	paymentMethod string
	accountID     *string
	dupKey        string // date|amount|currency|normalized description
}

func (uc *importMovementsUseCase) Execute(ctx context.Context, input ImportMovementsInput) (ImportMovementsResult, error) {
	var result ImportMovementsResult
	if input.UserID == "" {
		return result, apperrors.ErrInvalidInput
	}

	accountsByName, err := uc.accountsByLowerName(ctx, input.UserID)
	if err != nil {
		return result, err
	}
	currencySet, err := uc.currencySet(ctx)
	if err != nil {
		return result, err
	}
	categoriesByName, err := uc.categoriesByLowerName(ctx)
	if err != nil {
		return result, err
	}

	var valid []validRow
	for i, row := range input.Rows {
		rowNum := i + 1 // 1-based, counting only data rows
		vr, rowErrs := validateImportRow(rowNum, row, accountsByName, currencySet, categoriesByName)
		if len(rowErrs) > 0 {
			result.Errors = append(result.Errors, rowErrs...)
			continue
		}
		valid = append(valid, vr)
	}

	duplicateRows, err := uc.flagDuplicates(ctx, input.UserID, valid)
	if err != nil {
		return result, err
	}
	result.Duplicates = duplicateRows

	// Strict mode aborts the whole file on any row error — nothing is
	// written, nothing is "skipped" either (the errors list alone
	// explains why); duplicates are still reported so a dry-run preview
	// (or a strict failure report) shows the full picture.
	if len(result.Errors) > 0 && !input.AllowPartial {
		return result, nil
	}

	skippedDuplicate := make(map[int]bool, len(duplicateRows))
	if input.SkipDuplicates {
		for _, d := range duplicateRows {
			skippedDuplicate[d.Row] = true
		}
	}

	var toInsert []*dto.MovementDTO
	skipped := len(result.Errors) // invalid rows only ever reach here in partial mode
	now := time.Now().UTC()
	for _, vr := range valid {
		if skippedDuplicate[vr.rowNum] {
			skipped++
			continue
		}
		toInsert = append(toInsert, dto.MovementFromEntity(&entities.Movement{
			UserID:        input.UserID,
			Amount:        vr.amount,
			Currency:      vr.currency,
			Description:   vr.description,
			CategoryID:    vr.categoryID,
			PaymentMethod: vr.paymentMethod,
			AccountID:     vr.accountID,
			Status:        entities.MovementStatusActive,
			SyncStatus:    entities.SyncStatusPending,
			Timestamp:     vr.timestamp,
			CreatedAt:     now,
		}))
	}

	result.Imported = len(toInsert)
	result.Skipped = skipped

	if input.DryRun || len(toInsert) == 0 {
		return result, nil
	}

	// Register any payment method used in this batch that isn't already in
	// the caller's registry (BACK-17) — only here, never during the
	// validate/preview pass above, so a dry run can't mutate the registry.
	seenMethods := make(map[string]bool, len(toInsert))
	for _, m := range toInsert {
		if seenMethods[m.PaymentMethod] {
			continue
		}
		seenMethods[m.PaymentMethod] = true
		if _, err := uc.methods.EnsureByName(ctx, input.UserID, m.PaymentMethod); err != nil {
			return ImportMovementsResult{}, err
		}
	}

	if _, err := uc.movements.CreateBatch(ctx, toInsert); err != nil {
		return ImportMovementsResult{}, err
	}
	return result, nil
}

func (uc *importMovementsUseCase) accountsByLowerName(ctx context.Context, userID string) (map[string]*dto.AccountDTO, error) {
	accounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*dto.AccountDTO, len(accounts))
	for _, a := range accounts {
		byName[strings.ToLower(a.Name)] = a
	}
	return byName, nil
}

func (uc *importMovementsUseCase) currencySet(ctx context.Context) (map[string]bool, error) {
	codes, err := uc.currencies.List(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(codes))
	for _, c := range codes {
		set[c] = true
	}
	return set, nil
}

// categoriesByLowerName maps every existing category's lowercased name to
// its id (BACK-14 follow-up: categories are a real, globally-shared
// registry now, referenced by id — a CSV's "category" column is still a
// human-readable name, so import resolves it to an id here rather than
// requiring the operator to know ids). Two categories with the same
// name (allowed under the shared model) collide in this map; whichever
// was returned last by CategoryRepository.ListAll wins, an acceptable
// ambiguity for a CSV convenience lookup.
func (uc *importMovementsUseCase) categoriesByLowerName(ctx context.Context) (map[string]string, error) {
	categories, err := uc.categories.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]string, len(categories))
	for _, c := range categories {
		byName[strings.ToLower(c.Name)] = c.ID
	}
	return byName, nil
}

// validateImportRow applies BACK-03's CSV model rules to one row. It
// deliberately doesn't call resolvePaymentMethod (used by CreateMovement)
// because that reports one field's error at a time; per-row-per-field
// reporting is the whole point of an import preview. category_id (BACK-14
// follow-up) is resolved by case-insensitive name lookup against
// categoriesByName instead — CSV is still human-readable text, not raw
// ids — with the same "blank stays uncategorized, otherwise must
// resolve" rule CreateMovement's own resolveCategoryID applies.
// payment_method has no such check (BACK-17: no longer a fixed enum) —
// any non-blank value is accepted here and implicitly registered against
// the caller's registry at actual-insert time (Execute, only outside a
// dry run — a preview must never mutate the registry).
func validateImportRow(rowNum int, row ImportRowInput, accountsByName map[string]*dto.AccountDTO, currencySet map[string]bool, categoriesByName map[string]string) (validRow, []ImportRowError) {
	var errs []ImportRowError
	addErr := func(field, message string) {
		errs = append(errs, ImportRowError{Row: rowNum, Field: field, Message: message})
	}

	timestamp, err := time.Parse("2006-01-02", strings.TrimSpace(row.Date))
	if err != nil {
		addErr("date", "must be YYYY-MM-DD")
	} else {
		// Interpreted at noon UTC: far enough from midnight that no
		// timezone display shifts it to the adjacent calendar day.
		timestamp = time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 12, 0, 0, 0, time.UTC)
	}

	amount, err := strconv.ParseInt(strings.TrimSpace(row.Amount), 10, 64)
	if err != nil {
		addErr("amount", "must be an integer (smallest currency unit)")
	} else if amount == 0 {
		addErr("amount", "must not be zero")
	}

	currency := strings.ToLower(strings.TrimSpace(row.Currency))
	if currency == "" {
		addErr("currency", "is required")
	} else if !currencySet[currency] {
		addErr("currency", "not a registered currency (POST /currencies first)")
	}

	var categoryID *string
	categoryName := strings.ToLower(strings.TrimSpace(row.Category))
	if categoryName != "" {
		id, ok := categoriesByName[categoryName]
		if !ok {
			addErr("category", "not a recognized category (see GET /categories)")
		} else {
			categoryID = &id
		}
	}

	paymentMethod := strings.ToLower(strings.TrimSpace(row.PaymentMethod))
	if paymentMethod == "" {
		paymentMethod = "other"
	}

	var accountID *string
	accountName := strings.TrimSpace(row.Account)
	if accountName != "" {
		account, ok := accountsByName[strings.ToLower(accountName)]
		if !ok {
			addErr("account", "no account with this name")
		} else if currency != "" && account.Currency != currency {
			addErr("account", "account currency does not match the row's currency")
		} else {
			accountID = &account.ID
		}
	}

	description := strings.TrimSpace(row.Description)

	if len(errs) > 0 {
		return validRow{}, errs
	}

	dupKey := timestamp.Format("2006-01-02") + "|" + strconv.FormatInt(amount, 10) + "|" + currency + "|" + strings.ToLower(description)
	return validRow{
		rowNum:        rowNum,
		timestamp:     timestamp,
		amount:        amount,
		currency:      currency,
		description:   description,
		categoryID:    categoryID,
		paymentMethod: paymentMethod,
		accountID:     accountID,
		dupKey:        dupKey,
	}, nil
}

// flagDuplicates checks every valid row against (a) earlier rows in the
// same file and (b) already-existing movements, on (user, date, amount,
// currency, normalized description). Existing-movement matches take
// priority when a row happens to match both.
func (uc *importMovementsUseCase) flagDuplicates(ctx context.Context, userID string, rows []validRow) ([]ImportDuplicate, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	existingByKey, err := uc.existingMovementsByKey(ctx, userID, rows)
	if err != nil {
		return nil, err
	}

	var duplicates []ImportDuplicate
	seenInFile := make(map[string]int, len(rows)) // dupKey -> first row number that used it
	for _, vr := range rows {
		if existingID, ok := existingByKey[vr.dupKey]; ok {
			id := existingID
			duplicates = append(duplicates, ImportDuplicate{Row: vr.rowNum, ExistingMovementID: &id})
			continue
		}
		if firstRow, ok := seenInFile[vr.dupKey]; ok {
			row := firstRow
			duplicates = append(duplicates, ImportDuplicate{Row: vr.rowNum, DuplicateOfRow: &row})
			continue
		}
		seenInFile[vr.dupKey] = vr.rowNum
	}
	return duplicates, nil
}

// existingMovementsByKey loads the user's movements covering the
// imported rows' date range once, rather than one query per row.
func (uc *importMovementsUseCase) existingMovementsByKey(ctx context.Context, userID string, rows []validRow) (map[string]string, error) {
	minDate, maxDate := rows[0].timestamp, rows[0].timestamp
	for _, vr := range rows[1:] {
		if vr.timestamp.Before(minDate) {
			minDate = vr.timestamp
		}
		if vr.timestamp.After(maxDate) {
			maxDate = vr.timestamp
		}
	}
	from := time.Date(minDate.Year(), minDate.Month(), minDate.Day(), 0, 0, 0, 0, time.UTC)
	to := time.Date(maxDate.Year(), maxDate.Month(), maxDate.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	existing, err := uc.movements.ListByUser(ctx, userID, nil, &from, &to, 0, 0)
	if err != nil {
		return nil, err
	}

	byKey := make(map[string]string, len(existing))
	for _, m := range existing {
		if m.Status != string(entities.MovementStatusActive) {
			continue
		}
		key := m.Timestamp.Format("2006-01-02") + "|" + strconv.FormatInt(m.Amount, 10) + "|" + m.Currency + "|" + strings.ToLower(strings.TrimSpace(m.Description))
		if _, ok := byKey[key]; !ok {
			byKey[key] = m.ID
		}
	}
	return byKey, nil
}
