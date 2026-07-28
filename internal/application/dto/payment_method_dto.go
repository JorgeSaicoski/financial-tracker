package dto

import "time"

// PaymentMethodDTO is the application layer's representation of one
// user's payment-method registry row (BACK-17) — payment methods are no
// longer a fixed enum, but a per-user registry, mirroring CategoryDTO's
// shape minus the avoidability attribute (payment methods don't carry
// one).
type PaymentMethodDTO struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
}
