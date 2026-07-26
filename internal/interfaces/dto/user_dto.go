package dto

import "time"

// UserResponse is the API response body for GET /me. Whether this user's
// movements sync to ledger-service is GET /settings' concern (BACK-13),
// not this endpoint's.
type UserResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
