// Package reqctx carries the authenticated user id from the auth
// middleware (internal/interfaces/api) into handlers
// (internal/interfaces/api/handlers) via the request context.
//
// It exists as its own leaf package purely to avoid an import cycle:
// internal/interfaces/api's router.go imports
// internal/interfaces/api/handlers to wire routes, so the auth middleware
// living in internal/interfaces/api can't itself be imported back by
// internal/interfaces/api/handlers. Both sides import this tiny package
// instead.
package reqctx

import "context"

type contextKey int

const userIDKey contextKey = iota

// WithUserID returns a copy of ctx carrying the authenticated user's id.
// Call from the auth middleware only — never from a handler or usecase.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserID returns the authenticated user id the auth middleware attached
// to ctx, and whether one was present. Every route is behind the auth
// middleware (or its AUTH_DISABLED dev stand-in), so "not present" here
// means a route was wired without it — a bug to guard against, not a
// silent fallback to any default.
func UserID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok && v != ""
}
