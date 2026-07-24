package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// AccountDTO is the application layer's representation of an account.
// Type is a plain string; validation against the fixed AccountType list
// happens in usecases via the domain type.
type AccountDTO struct {
	ID        string
	UserID    string
	Name      string
	Type      string
	Currency  string
	CreatedAt time.Time
}

func AccountFromEntity(a *entities.Account) *AccountDTO {
	if a == nil {
		return nil
	}
	return &AccountDTO{
		ID:        a.ID,
		UserID:    a.UserID,
		Name:      a.Name,
		Type:      string(a.Type),
		Currency:  a.Currency,
		CreatedAt: a.CreatedAt,
	}
}

func (a *AccountDTO) ToEntity() *entities.Account {
	if a == nil {
		return nil
	}
	return &entities.Account{
		ID:        a.ID,
		UserID:    a.UserID,
		Name:      a.Name,
		Type:      entities.AccountType(a.Type),
		Currency:  a.Currency,
		CreatedAt: a.CreatedAt,
	}
}

// AccountSnapshotDTO mirrors entities.AccountSnapshot: a user-reported
// real balance at a point in time.
type AccountSnapshotDTO struct {
	ID        string
	AccountID string
	Balance   int64 // smallest currency unit, same convention as MovementDTO.Amount
	Timestamp time.Time
	CreatedAt time.Time
}

func AccountSnapshotFromEntity(s *entities.AccountSnapshot) *AccountSnapshotDTO {
	if s == nil {
		return nil
	}
	return &AccountSnapshotDTO{
		ID:        s.ID,
		AccountID: s.AccountID,
		Balance:   s.Balance,
		Timestamp: s.Timestamp,
		CreatedAt: s.CreatedAt,
	}
}

func (s *AccountSnapshotDTO) ToEntity() *entities.AccountSnapshot {
	if s == nil {
		return nil
	}
	return &entities.AccountSnapshot{
		ID:        s.ID,
		AccountID: s.AccountID,
		Balance:   s.Balance,
		Timestamp: s.Timestamp,
		CreatedAt: s.CreatedAt,
	}
}
