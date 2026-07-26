package entities

// CategoryTransfer and CategoryIncome are the two category names code
// branches on directly (Account.Send/Receive set CategoryTransfer;
// getCashflowUseCase excludes it from income/expense totals) — kept as
// plain string constants so those comparisons still compile, even though
// categories themselves are no longer a fixed enum (BACK-14 turned them
// into a per-user registry, see application/repositories/category_repository.go).
// Both are "system" categories: not spend, so they carry no avoidability
// and can't be created/renamed/deleted through the category API.
const (
	CategoryTransfer = "transfer"
	CategoryIncome   = "income"
)
