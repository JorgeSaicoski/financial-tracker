package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// UserDTO is the application layer's representation of a user.
type UserDTO struct {
	ID          string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string

	CreatedAt time.Time
	UpdatedAt time.Time
}

func UserFromEntity(u *entities.User) *UserDTO {
	if u == nil {
		return nil
	}
	return &UserDTO{
		ID:          u.ID,
		Provider:    u.Provider,
		ExternalID:  u.ExternalID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

func (u *UserDTO) ToEntity() *entities.User {
	if u == nil {
		return nil
	}
	return &entities.User{
		ID:          u.ID,
		Provider:    u.Provider,
		ExternalID:  u.ExternalID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
