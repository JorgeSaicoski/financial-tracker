// Package id generates the lowercase UUIDv4 identifiers used for local
// rows. Kept dependency-free (crypto/rand) since generation is the only
// UUID operation financial-tracker needs.
package id

import (
	"crypto/rand"
	"fmt"
)

func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing means the platform is broken beyond what a
		// request handler can recover from.
		panic(fmt.Sprintf("id: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
