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
}

// NewImportMovements returns interface type for dependency injection.
func NewImportMovements(movements repositories.MovementRepository, accounts repositories.AccountRepository, currencies repositories.CurrencyRepository) ImportMovementsUseCase {
	return &importMovementsUseCase{movements: movements, accounts: accounts, currencies: currencies}
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
	category      entities.Category
	paymentMethod entities.PaymentMethod
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

	var valid []validRow
	for i, row := range input.Rows {
		rowNum := i + 1 // 1-based, counting only data rows
		vr, rowErrs := validateImportRow(rowNum, row, accountsByName, currencySet)
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
			Category:      vr.category,
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

// validateImportRow applies BACK-03's CSV model rules to one row. It
// deliberately doesn't call normalizeCategoryAndMethod (used by
// CreateMovement) because that helper reports one combined error;
// per-row-per-field reporting is the whole point of an import preview,
// so category and payment_method are checked separately here even
// though the underlying rule (blank -> "other", otherwise must be a
// valid domain value) is identical.
func validateImportRow(rowNum int, row ImportRowInput, accountsByName map[string]*dto.AccountDTO, currencySet map[string]bool) (validRow, []ImportRowError) {
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

	category := entities.Category(strings.ToLower(strings.TrimSpace(row.Category)))
	if category == "" {
		category = entities.CategoryOther
	}
	if !category.IsValid() {
		addErr("category", "not a recognized category (see GET /categories)")
	}

	paymentMethod := entities.PaymentMethod(strings.ToLower(strings.TrimSpace(row.PaymentMethod)))
	if paymentMethod == "" {
		paymentMethod = entities.PaymentMethodOther
	}
	if !paymentMethod.IsValid() {
		addErr("payment_method", "not a recognized payment method (see GET /categories)")
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
		category:      category,
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
