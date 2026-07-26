package usecases

import (
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// EffectiveAvoidability resolves one movement's avoidability_percent
// (BACK-14): its own AvoidabilityOverridePercent wins when set; else it
// resolves the movement's Category against the user's category registry
// (categoriesByName, see CategoriesByName) and uses that category's
// AvoidabilityPercent; else (no category, or one not present in the map
// — e.g. deleted) there is no value, excluded from any
// avoidability-weighted aggregate — same exclusion shape cashflow
// already gives the "transfer" category. Exported: BACK-12's
// purchasing-power report reuses this.
func EffectiveAvoidability(m *dto.MovementDTO, categoriesByName map[string]*dto.CategoryDTO) *int {
	if m.AvoidabilityOverridePercent != nil {
		return m.AvoidabilityOverridePercent
	}
	if m.Category == "" {
		return nil
	}
	c, ok := categoriesByName[strings.ToLower(m.Category)]
	if !ok {
		return nil
	}
	return c.AvoidabilityPercent
}

// CategoriesByName indexes a user's category list by lowercased name —
// the lookup shape EffectiveAvoidability needs.
func CategoriesByName(categories []*dto.CategoryDTO) map[string]*dto.CategoryDTO {
	out := make(map[string]*dto.CategoryDTO, len(categories))
	for _, c := range categories {
		out[strings.ToLower(c.Name)] = c
	}
	return out
}
