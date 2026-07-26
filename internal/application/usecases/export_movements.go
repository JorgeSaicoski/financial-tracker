package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type exportMovementsUseCase struct {
	movements repositories.MovementRepository
	accounts  repositories.AccountRepository
}

// NewExportMovements returns interface type for dependency injection.
func NewExportMovements(movements repositories.MovementRepository, accounts repositories.AccountRepository) ExportMovementsUseCase {
	return &exportMovementsUseCase{movements: movements, accounts: accounts}
}

func (uc *exportMovementsUseCase) Execute(ctx context.Context, userID string, includeCancelled bool) ([]ExportedRow, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}

	movements, err := uc.movements.ListByUser(ctx, userID, nil, nil, nil, 0, 0)
	if err != nil {
		return nil, err
	}

	accounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	accountNames := make(map[string]string, len(accounts))
	for _, a := range accounts {
		accountNames[a.ID] = a.Name
	}

	rows := make([]ExportedRow, 0, len(movements))
	for _, m := range movements {
		isCancelled := m.Status == string(entities.MovementStatusVoided) ||
			m.CancelsMovementID != nil ||
			m.ReversedByMovementID != nil
		if isCancelled && !includeCancelled {
			continue
		}

		var account string
		if m.AccountID != nil {
			account = accountNames[*m.AccountID]
		}

		row := ExportedRow{
			Date:          m.Timestamp.Format("2006-01-02"),
			Amount:        m.Amount,
			Currency:      m.Currency,
			Description:   m.Description,
			Category:      m.Category,
			PaymentMethod: m.PaymentMethod,
			Account:       account,
		}
		if includeCancelled {
			row.Status = m.Status
			if m.CancelsMovementID != nil {
				row.CancelsMovementID = *m.CancelsMovementID
			}
			if m.ReversedByMovementID != nil {
				row.ReversedByMovementID = *m.ReversedByMovementID
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}
