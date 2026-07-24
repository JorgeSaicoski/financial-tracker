// Package dto holds the application layer's data contracts — what
// usecases, repository interfaces, and service ports pass to each other.
// Following CleanExampleGo's application/dto: infrastructure adapts
// whatever it holds (DB rows, external-service JSON) into these types at
// its boundary, and the interfaces layer converts them to API shapes.
// Domain entities never cross a contract; the converters here are the
// only bridge, so a schema or entity change can't silently ripple
// through every layer.
package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// MovementDTO is the application layer's representation of one financial
// movement. Enum-like fields (Category, PaymentMethod, Status,
// SyncStatus) are plain strings here; validation against the fixed lists
// happens in usecases via the domain types.
type MovementDTO struct {
	ID            string
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string

	AccountID  *string
	TransferID *string

	CreditCardPurchaseID *string
	InstallmentNumber    *int // 1-based

	Status               string
	CancelsMovementID    *string
	ReversedByMovementID *string

	Timestamp time.Time

	SyncStatus          string
	LedgerTransactionID *string
	SyncAttempts        int
	LastSyncError       *string
	LastSyncAttemptAt   *time.Time
	SyncedAt            *time.Time

	CreatedAt time.Time
}

// MovementFromEntity converts a domain movement to the application DTO.
// Pointer fields are shared, not deep-copied — same aliasing the layers
// already had when they passed the entity itself around.
func MovementFromEntity(m *entities.Movement) *MovementDTO {
	if m == nil {
		return nil
	}
	return &MovementDTO{
		ID:                   m.ID,
		UserID:               m.UserID,
		Amount:               m.Amount,
		Currency:             m.Currency,
		Description:          m.Description,
		Category:             string(m.Category),
		PaymentMethod:        string(m.PaymentMethod),
		AccountID:            m.AccountID,
		TransferID:           m.TransferID,
		CreditCardPurchaseID: m.CreditCardPurchaseID,
		InstallmentNumber:    m.InstallmentNumber,
		Status:               string(m.Status),
		CancelsMovementID:    m.CancelsMovementID,
		ReversedByMovementID: m.ReversedByMovementID,
		Timestamp:            m.Timestamp,
		SyncStatus:           string(m.SyncStatus),
		LedgerTransactionID:  m.LedgerTransactionID,
		SyncAttempts:         m.SyncAttempts,
		LastSyncError:        m.LastSyncError,
		LastSyncAttemptAt:    m.LastSyncAttemptAt,
		SyncedAt:             m.SyncedAt,
		CreatedAt:            m.CreatedAt,
	}
}

// MovementsFromEntities converts a slice, preserving order.
func MovementsFromEntities(ms []*entities.Movement) []*MovementDTO {
	out := make([]*MovementDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, MovementFromEntity(m))
	}
	return out
}

// ToEntity converts back to the domain type, so usecases can run entity
// business rules (IsSynced, IsCancelled, ...) on data a repository
// returned.
func (m *MovementDTO) ToEntity() *entities.Movement {
	if m == nil {
		return nil
	}
	return &entities.Movement{
		ID:                   m.ID,
		UserID:               m.UserID,
		Amount:               m.Amount,
		Currency:             m.Currency,
		Description:          m.Description,
		Category:             entities.Category(m.Category),
		PaymentMethod:        entities.PaymentMethod(m.PaymentMethod),
		AccountID:            m.AccountID,
		TransferID:           m.TransferID,
		CreditCardPurchaseID: m.CreditCardPurchaseID,
		InstallmentNumber:    m.InstallmentNumber,
		Status:               entities.MovementStatus(m.Status),
		CancelsMovementID:    m.CancelsMovementID,
		ReversedByMovementID: m.ReversedByMovementID,
		Timestamp:            m.Timestamp,
		SyncStatus:           entities.SyncStatus(m.SyncStatus),
		LedgerTransactionID:  m.LedgerTransactionID,
		SyncAttempts:         m.SyncAttempts,
		LastSyncError:        m.LastSyncError,
		LastSyncAttemptAt:    m.LastSyncAttemptAt,
		SyncedAt:             m.SyncedAt,
		CreatedAt:            m.CreatedAt,
	}
}
