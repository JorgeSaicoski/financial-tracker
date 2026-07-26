package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type fakeVerifier struct {
	identity services.Identity
	err      error
	gotToken string
}

func (f *fakeVerifier) Verify(_ context.Context, token string) (services.Identity, error) {
	f.gotToken = token
	if f.err != nil {
		return services.Identity{}, f.err
	}
	return f.identity, nil
}

type fakeEnsureUser struct {
	err      error
	gotInput usecases.EnsureUserInput
}

func (f *fakeEnsureUser) Execute(_ context.Context, input usecases.EnsureUserInput) (*dto.UserDTO, error) {
	f.gotInput = input
	if f.err != nil {
		return nil, f.err
	}
	return &dto.UserDTO{ID: input.UserID}, nil
}

func newTestMiddleware(verifier services.IdentityVerifier, ensureUser usecases.EnsureUserUseCase) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := reqctx.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(userID))
	})
	return Middleware(verifier, ensureUser, logger.New())(next)
}

func TestMiddlewareAcceptsValidBearerToken(t *testing.T) {
	verifier := &fakeVerifier{identity: services.Identity{UserID: "u1", Provider: "authentik", ExternalID: "sub-1"}}
	ensureUser := &fakeEnsureUser{}
	handler := newTestMiddleware(verifier, ensureUser)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "u1" {
		t.Errorf("downstream handler saw user_id %q, want %q", rec.Body.String(), "u1")
	}
	if verifier.gotToken != "good-token" {
		t.Errorf("verifier received token %q, want %q", verifier.gotToken, "good-token")
	}
	if ensureUser.gotInput.UserID != "u1" || ensureUser.gotInput.Provider != "authentik" {
		t.Errorf("ensureUser received %+v", ensureUser.gotInput)
	}
}

func TestMiddlewareAcceptsLowercaseBearerScheme(t *testing.T) {
	// RFC 7235: the auth-scheme token is case-insensitive.
	verifier := &fakeVerifier{identity: services.Identity{UserID: "u1"}}
	handler := newTestMiddleware(verifier, &fakeEnsureUser{})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "bearer good-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if verifier.gotToken != "good-token" {
		t.Errorf("verifier received token %q, want %q", verifier.gotToken, "good-token")
	}
}

func TestMiddlewareRejectsMissingAuthorizationHeader(t *testing.T) {
	verifier := &fakeVerifier{identity: services.Identity{UserID: "u1"}}
	handler := newTestMiddleware(verifier, &fakeEnsureUser{})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejectsMalformedAuthorizationHeader(t *testing.T) {
	cases := []string{"Token good-token", "Bearer", ""}
	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			verifier := &fakeVerifier{identity: services.Identity{UserID: "u1"}}
			handler := newTestMiddleware(verifier, &fakeEnsureUser{})

			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestMiddlewareRejectsVerificationFailure(t *testing.T) {
	verifier := &fakeVerifier{err: errors.New("invalid token")}
	handler := newTestMiddleware(verifier, &fakeEnsureUser{})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareReturns500WhenEnsureUserFails(t *testing.T) {
	verifier := &fakeVerifier{identity: services.Identity{UserID: "u1"}}
	ensureUser := &fakeEnsureUser{err: errors.New("db down")}
	handler := newTestMiddleware(verifier, ensureUser)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestDevUserMiddlewareAttributesFixedUser(t *testing.T) {
	ensureUser := &fakeEnsureUser{}
	var reachedUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedUserID, _ = reqctx.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := DevUserMiddleware("dev-user", ensureUser, logger.New())(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if reachedUserID != "dev-user" {
		t.Errorf("downstream handler saw user_id %q, want %q", reachedUserID, "dev-user")
	}
	if ensureUser.gotInput.UserID != "dev-user" {
		t.Errorf("ensureUser received %+v", ensureUser.gotInput)
	}
}
