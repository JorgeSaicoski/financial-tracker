package entities

import "time"

type Asset struct {
	ID               string
	Name             string
	PurchaseAmount   int64
	UsefulLifeMonths int
	CreatedAt        time.Time
}
