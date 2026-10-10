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

func TestAuthenticate_WorkshopScopedToken(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	wsID := uuid.New()
	user := &domain.User{
		ID:    uuid.New(),
		Email: "mechanic@bengkol.com",
		Role:  domain.RoleMechanic,
	}
	perms := domain.DefaultPermissionsForRole(domain.EmployeeRoleMechanic)

	scopedTokens, _, err := jwtMgr.GenerateScopedTokenPair(user, &wsID, domain.RoleMechanic, &perms)
	if err != nil {
		t.Fatalf("unexpected error generating scoped token: %v", err)
	}

	var extractedWorkshopID uuid.UUID
	var extractedPerms *domain.EmployeePermissions

	handler := middleware.Authenticate(jwtMgr, log)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id, ok := middleware.GetWorkshopID(r.Context()); ok {
				extractedWorkshopID = id
			}
			if p, ok := middleware.GetPermissions(r.Context()); ok {
				extractedPerms = p
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/workshop/action", nil)
	req.Header.Set("Authorization", "Bearer "+scopedTokens.AccessToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if extractedWorkshopID != wsID {
		t.Errorf("expected workshop ID %s, got %s", wsID, extractedWorkshopID)
	}
	if extractedPerms == nil || !extractedPerms.CanAccessRepairJobs {
		t.Errorf("expected mechanic permissions with CanAccessRepairJobs = true")
	}
}

func TestRequireWorkshopContext(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	wsID := uuid.New()
	user := &domain.User{
		ID:    uuid.New(),
		Email: "user@bengkol.com",
		Role:  domain.RoleManager,
	}

	unscopedTokens, _, _ := jwtMgr.GenerateTokenPair(user)
	scopedTokens, _, _ := jwtMgr.GenerateScopedTokenPair(user, &wsID, domain.RoleManager, nil)

	handler := middleware.Authenticate(jwtMgr, log)(
		middleware.RequireAuthenticated(
			middleware.RequireWorkshopContext(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
			),
		),
	)

	// 1. Unscoped token -> 403 Forbidden
	req1 := httptest.NewRequest(http.MethodGet, "/workshop/items", nil)
	req1.Header.Set("Authorization", "Bearer "+unscopedTokens.AccessToken)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusForbidden {
		t.Errorf("expected unscoped token to be rejected with 403, got %d", rec1.Code)
	}

	// 2. Scoped token -> 200 OK
	req2 := httptest.NewRequest(http.MethodGet, "/workshop/items", nil)
	req2.Header.Set("Authorization", "Bearer "+scopedTokens.AccessToken)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected scoped token to succeed with 200, got %d", rec2.Code)
	}
}

func TestRequirePermission_Enforcement(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	wsID := uuid.New()
	user := &domain.User{
		ID:    uuid.New(),
		Email: "staff@bengkol.com",
	}

	// Perms with cashier access but no employee management
	perms := domain.EmployeePermissions{
		CanAccessCashier:   true,
		CanManageEmployees: false,
	}

	tokens, _, _ := jwtMgr.GenerateScopedTokenPair(user, &wsID, domain.RoleAdmin, &perms)

	cashierHandler := middleware.Authenticate(jwtMgr, log)(
		middleware.RequirePermission(domain.PermCanAccessCashier)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	empMgmtHandler := middleware.Authenticate(jwtMgr, log)(
		middleware.RequirePermission(domain.PermCanManageEmployees)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	// Granted permission -> 200 OK
	reqCashier := httptest.NewRequest(http.MethodPost, "/cashier/pay", nil)
	reqCashier.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	recCashier := httptest.NewRecorder()
	cashierHandler.ServeHTTP(recCashier, reqCashier)
	if recCashier.Code != http.StatusOK {
		t.Errorf("expected 200 for granted permission, got %d", recCashier.Code)
	}

	// Denied permission -> 403 Forbidden
	reqEmp := httptest.NewRequest(http.MethodPost, "/employees/hire", nil)
	reqEmp.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	recEmp := httptest.NewRecorder()
	empMgmtHandler.ServeHTTP(recEmp, reqEmp)
	if recEmp.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied permission, got %d", recEmp.Code)
	}

	// Owner token -> 200 OK universal access
	ownerTokens, _, _ := jwtMgr.GenerateScopedTokenPair(user, &wsID, domain.RoleOwner, nil)
	reqOwner := httptest.NewRequest(http.MethodPost, "/employees/hire", nil)
	reqOwner.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recOwner := httptest.NewRecorder()
	empMgmtHandler.ServeHTTP(recOwner, reqOwner)
	if recOwner.Code != http.StatusOK {
		t.Errorf("expected OWNER to bypass permission checks (200), got %d", recOwner.Code)
	}
}
