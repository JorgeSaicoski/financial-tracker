package usecases

import (
	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// EffectiveAvoidability resolves one movement's avoidability_percent
// (BACK-14): its own AvoidabilityOverridePercent wins when set; else it
// resolves the movement's CategoryID against categoriesByID (see
// CategoriesByID) and uses that category's AvoidabilityPercent; else (no
// category, or one not present in the map — e.g. hidden) there is no
// value, excluded from any avoidability-weighted aggregate — same
// exclusion shape cashflow already gives the "transfer" category.
// Exported: BACK-12's purchasing-power report reuses this.
func EffectiveAvoidability(m *dto.MovementDTO, categoriesByID map[string]*dto.CategoryDTO) *int {
	if m.AvoidabilityOverridePercent != nil {
		return m.AvoidabilityOverridePercent
	}
	if m.CategoryID == nil {
		return nil
	}
	c, ok := categoriesByID[*m.CategoryID]
	if !ok {
		return nil
	}
	return c.AvoidabilityPercent
}

// CategoriesByID indexes a category list by id — the lookup shape
// EffectiveAvoidability needs, now that names are not unique (BACK-14
// follow-up: categories are shared/global, so two different categories
// may share a name).
func CategoriesByID(categories []*dto.CategoryDTO) map[string]*dto.CategoryDTO {
	out := make(map[string]*dto.CategoryDTO, len(categories))
	for _, c := range categories {
		out[c.ID] = c
	}
	return out
}
