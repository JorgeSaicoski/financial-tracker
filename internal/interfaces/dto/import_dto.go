package dto

// ImportSpecColumn describes one column of BACK-03's CSV import model.
// AllowedValues is populated from real data (categories, payment
// methods, the user's own account names, registered currencies) — the
// frontend renders this instead of hardcoding the spec.
type ImportSpecColumn struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

// ImportSpecResponse is GET /import/movements/spec's body: what
// POST /import/movements accepts, plus a ready-to-copy template.
type ImportSpecResponse struct {
	Columns  []ImportSpecColumn `json:"columns"`
	Template string             `json:"template"`
}

// ImportRowErrorResponse is one row's validation failure. Row is
// 1-based, counting only data rows (the row right after the header is
// row 1).
type ImportRowErrorResponse struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ImportDuplicateResponse flags a row matching an existing movement
// (existing_movement_id set) or an earlier row in the same file
// (duplicate_of_row set). Still imported by default; skip_duplicates=true
// excludes it.
type ImportDuplicateResponse struct {
	Row                int     `json:"row"`
	ExistingMovementID *string `json:"existing_movement_id,omitempty"`
	DuplicateOfRow     *int    `json:"duplicate_of_row,omitempty"`
}

// ImportMovementsResponse is POST /import/movements's body — the same
// shape whether dry_run=true (nothing written, describes what would
// happen) or a real commit (describes what did happen).
type ImportMovementsResponse struct {
	Imported   int                       `json:"imported"`
	Skipped    int                       `json:"skipped"`
	Errors     []ImportRowErrorResponse  `json:"errors"`
	Duplicates []ImportDuplicateResponse `json:"duplicates"`
}
