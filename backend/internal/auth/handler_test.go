package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/auth"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestApp() (chi.Router, auth.Service, *security.JWTManager) {
	repo := newMockAuthRepo()
	jwtMgr := security.NewJWTManager("test-secret-key-at-least-32-chars-long", "test-refresh-secret", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	authSvc := auth.NewService(repo, jwtMgr, log)
	authHdl := auth.NewHandler(authSvc, log)
	healthHdl := handler.NewHealthHandler(nil)

	cfg := &config.Config{
		AppEnv: "development",
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
	}

	router := handler.NewRouter(handler.RouterConfig{
		Config:        cfg,
		Logger:        log,
		HealthHandler: healthHdl,
		AuthHandler:   authHdl,
		JWTManager:    jwtMgr,
	})

	return router, authSvc, jwtMgr
}

func TestHandler_Register_Success(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]interface{}{
		"name":     "Customer Test",
		"email":    "customertest@example.com",
		"password": "strongPassword123",
		"phone":    "0812345678",
		"role":     "CUSTOMER",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: body=%s", rec.Code, rec.Body.String())
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected response.success to be true")
	}
}

func TestHandler_Register_ValidationError(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]interface{}{
		"name":     "",
		"email":    "bad-email",
		"password": "123",
		"phone":    "",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", rec.Code)
	}

	var resp response.Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Error == nil || resp.Error.Code != response.ErrCodeValidationFailed {
		t.Errorf("expected error code VALIDATION_FAILED, got %+v", resp.Error)
	}
}

func TestHandler_Login_Success(t *testing.T) {
	app, svc, _ := setupTestApp()

	_, _, _ = svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Test Login",
		Email:    "testlogin@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	})

	payload := map[string]string{
		"email":    "testlogin@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Login_InvalidCredentials(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]string{
		"email":    "nonexistent@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestHandler_GetMe_Authenticated(t *testing.T) {
	app, svc, _ := setupTestApp()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Authenticated User",
		Email:    "authme@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+regResp.Tokens.AccessToken)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_GetMe_Unauthenticated(t *testing.T) {
	app, _, _ := setupTestApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestHandler_Logout_Success(t *testing.T) {
	app, svc, _ := setupTestApp()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Logout Tester",
		Email:    "logouttest@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	})

	payload := map[string]string{
		"refresh_token": regResp.Tokens.RefreshToken,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHandler_GetMe_ContextUserID(t *testing.T) {
	uID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, uID)
	req = req.WithContext(ctx)

	var retrievedID uuid.UUID
	if id, ok := middleware.GetUserID(req.Context()); ok {
		retrievedID = id
	}

	if retrievedID != uID {
		t.Errorf("expected user ID %s, got %s", uID, retrievedID)
	}
}
