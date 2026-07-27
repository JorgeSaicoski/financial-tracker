package dto

// CreateCategoryRequest is the API request body for POST /categories.
// AvoidabilityPercent omitted defaults to a neutral 50 (the user edits it
// afterward) — reserved system names ("transfer", "income") are rejected.
type CreateCategoryRequest struct {
	Name                string `json:"name"`
	AvoidabilityPercent *int   `json:"avoidability_percent,omitempty"`
}

// UpdateCategoryRequest is the API request body for PATCH /categories/{id}.
// A field absent from the JSON body leaves that value unchanged; system
// categories reject any edit at all.
type UpdateCategoryRequest struct {
	Name                *string `json:"name,omitempty"`
	AvoidabilityPercent *int    `json:"avoidability_percent,omitempty"`
}
