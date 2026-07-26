// Package id generates the lowercase UUIDv4 identifiers used for local
// rows, plus the UUIDv5 derivation BACK-02's auth middleware uses to turn
// a non-UUID OIDC `sub` claim into a stable user_id. Kept dependency-free
// (crypto/rand, crypto/sha1 — both stdlib) since these are the only UUID
// operations financial-tracker needs.
package id

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
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

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID reports whether s is a canonical lowercase-hex, dashed UUID (any
// version/variant byte — this only checks shape, not RFC 4122 version
// bits). Callers normalize with strings.ToLower first if the input's case
// isn't already known-lowercase.
func IsUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// ParseUUID decodes a canonical dashed UUID string (case-insensitive) into
// its 16 raw bytes.
func ParseUUID(s string) ([16]byte, error) {
	var out [16]byte
	s = strings.ToLower(strings.TrimSpace(s))
	if !IsUUID(s) {
		return out, fmt.Errorf("id: invalid UUID %q", s)
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil {
		return out, fmt.Errorf("id: invalid UUID %q: %w", s, err)
	}
	copy(out[:], raw)
	return out, nil
}

// NewUUIDv5 deterministically derives a lowercase UUIDv5 (RFC 4122 §4.3,
// SHA-1-based) from a namespace UUID and a name: the same (namespace,
// name) pair always produces the same UUID, with no randomness involved.
// Used to derive a stable, lowercase user_id from an OIDC `sub` claim that
// isn't itself already a UUID (see internal/interfaces/api's auth
// middleware) — deterministic (never random) so the same external
// identity always maps back to the same local user_id.
func NewUUIDv5(namespace, name string) (string, error) {
	ns, err := ParseUUID(namespace)
	if err != nil {
		return "", err
	}
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte(name))
	sum := h.Sum(nil) // 20 bytes; UUID only uses the first 16

	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
