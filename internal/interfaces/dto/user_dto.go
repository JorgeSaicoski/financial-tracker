package dto

import "time"

// UserResponse is the API response body for GET /me.
type UserResponse struct {
	ID               string    `json:"id"`
	Email            string    `json:"email,omitempty"`
	DisplayName      string    `json:"display_name,omitempty"`
	CloudSyncEnabled bool      `json:"cloud_sync_enabled"`
	CreatedAt        time.Time `json:"created_at"`
}
