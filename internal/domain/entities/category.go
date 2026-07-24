package entities

// Category is the fixed, predefined category list for movements. The
// frontend fetches it from GET /categories rather than hardcoding it.
type Category string

const (
	CategoryFood          Category = "food"
	CategoryTransport     Category = "transport"
	CategoryHousing       Category = "housing"
	CategoryUtilities     Category = "utilities"
	CategoryHealth        Category = "health"
	CategoryEntertainment Category = "entertainment"
	CategoryShopping      Category = "shopping"
	CategoryEducation     Category = "education"
	CategoryIncome        Category = "income"
	CategoryTransfer      Category = "transfer"
	CategoryOther         Category = "other"
)

func Categories() []Category {
	return []Category{
		CategoryFood,
		CategoryTransport,
		CategoryHousing,
		CategoryUtilities,
		CategoryHealth,
		CategoryEntertainment,
		CategoryShopping,
		CategoryEducation,
		CategoryIncome,
		CategoryTransfer,
		CategoryOther,
	}
}

func (c Category) IsValid() bool {
	for _, cat := range Categories() {
		if c == cat {
			return true
		}
	}
	return false
}
