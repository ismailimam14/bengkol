package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
)

func TestAuthenticate_ValidToken(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	user := &domain.User{
		ID:    uuid.New(),
		Email: "customer@bengkol.com",
		Name:  "Test Customer",
		Role:  domain.RoleCustomer,
	}
	tokens, _, _ := jwtMgr.GenerateTokenPair(user)

	var extractedUserID uuid.UUID
	var extractedRole domain.UserRole

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := middleware.GetUserID(r.Context()); ok {
			extractedUserID = id
		}
		if role, ok := middleware.GetUserRole(r.Context()); ok {
			extractedRole = role
		}
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := middleware.Authenticate(jwtMgr, log)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()

	authMiddleware(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if extractedUserID != user.ID {
		t.Errorf("expected user ID %s, got %s", user.ID, extractedUserID)
	}

	if extractedRole != domain.RoleCustomer {
		t.Errorf("expected role CUSTOMER, got %s", extractedRole)
	}
}

func TestAuthenticate_MalformedHeader(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := middleware.Authenticate(jwtMgr, log)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "InvalidFormatWithoutBearerToken")
	rec := httptest.NewRecorder()

	authMiddleware(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 on malformed header, got %d", rec.Code)
	}
}

func TestRequireRoles_PermissionGrantAndDenial(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	ownerUser := &domain.User{
		ID:    uuid.New(),
		Email: "owner@bengkol.com",
		Role:  domain.RoleOwner,
	}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(ownerUser)

	customerUser := &domain.User{
		ID:    uuid.New(),
		Email: "customer@bengkol.com",
		Role:  domain.RoleCustomer,
	}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customerUser)

	adminUser := &domain.User{
		ID:    uuid.New(),
		Email: "admin@bengkol.com",
		Role:  domain.RoleAdmin,
	}
	adminTokens, _, _ := jwtMgr.GenerateTokenPair(adminUser)

	ownerEndpointHandler := middleware.Authenticate(jwtMgr, log)(
		middleware.RequireAuthenticated(
			middleware.RequireRoles(domain.RoleOwner)(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
			),
		),
	)

	// 1. Owner accessing Owner endpoint -> 200 OK
	reqOwner := httptest.NewRequest(http.MethodPost, "/owner/service", nil)
	reqOwner.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recOwner := httptest.NewRecorder()
	ownerEndpointHandler.ServeHTTP(recOwner, reqOwner)
	if recOwner.Code != http.StatusOK {
		t.Errorf("expected OWNER to be allowed (200), got %d", recOwner.Code)
	}

	// 2. Customer accessing Owner endpoint -> 403 Forbidden
	reqCustomer := httptest.NewRequest(http.MethodPost, "/owner/service", nil)
	reqCustomer.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recCustomer := httptest.NewRecorder()
	ownerEndpointHandler.ServeHTTP(recCustomer, reqCustomer)
	if recCustomer.Code != http.StatusForbidden {
		t.Errorf("expected CUSTOMER to be forbidden (403), got %d", recCustomer.Code)
	}

	// 3. Admin accessing Owner endpoint -> 200 OK (universal access)
	reqAdmin := httptest.NewRequest(http.MethodPost, "/owner/service", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+adminTokens.AccessToken)
	recAdmin := httptest.NewRecorder()
	ownerEndpointHandler.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK {
		t.Errorf("expected ADMIN to bypass role checks (200), got %d", recAdmin.Code)
	}
}
