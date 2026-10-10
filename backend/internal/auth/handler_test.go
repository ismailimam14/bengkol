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
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
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

func setupTestAppWithRepo() (chi.Router, auth.Service, *mockAuthRepo, *security.JWTManager) {
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
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
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

	return router, authSvc, repo, jwtMgr
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

func TestHandler_Register_Success_WithoutEmail(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]interface{}{
		"name":     "No Email Customer",
		"password": "strongPassword123",
		"phone":    "089988776655",
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

func TestHandler_Register_MissingPhone(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]interface{}{
		"name":     "Customer Without Phone",
		"email":    "cust@example.com",
		"password": "strongPassword123",
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
		t.Fatalf("expected VALIDATION_FAILED, got %+v", resp.Error)
	}
	if _, ok := resp.Error.Details["phone"]; !ok {
		t.Errorf("expected validation error on phone")
	}
}

func TestHandler_Register_DuplicatePhone(t *testing.T) {
	app, _, _ := setupTestApp()

	payload1 := map[string]interface{}{
		"name":     "First User",
		"email":    "first@example.com",
		"password": "strongPassword123",
		"phone":    "081122334455",
	}
	body1, _ := json.Marshal(payload1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on first registration, got %d", rec1.Code)
	}

	// Register second user with same phone but different email
	payload2 := map[string]interface{}{
		"name":     "Second User",
		"email":    "second@example.com",
		"password": "strongPassword123",
		"phone":    "081122334455",
	}
	body2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict, got %d: body=%s", rec2.Code, rec2.Body.String())
	}

	var resp response.Response
	_ = json.Unmarshal(rec2.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != response.ErrCodeConflict {
		t.Errorf("expected CONFLICT code, got %+v", resp.Error)
	}
	if _, ok := resp.Error.Details["phone"]; !ok {
		t.Errorf("expected conflict details for phone")
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

func TestHandler_Login_Success_WithPhone(t *testing.T) {
	app, svc, _ := setupTestApp()

	_, _, _ = svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Test Phone Login",
		Email:    "testphonelogin@example.com",
		Password: "password123",
		Phone:    "081987654321",
		Role:     domain.RoleCustomer,
	})

	payload := map[string]string{
		"phone":    "081987654321",
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

func TestHandler_Login_Success_WithPhoneNumber(t *testing.T) {
	app, svc, _ := setupTestApp()

	_, _, _ = svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Test PhoneNumber Login",
		Email:    "testphonenumberlogin@example.com",
		Password: "password123",
		Phone:    "081987654322",
		Role:     domain.RoleCustomer,
	})

	payload := map[string]string{
		"phone_number": "081987654322",
		"password":     "password123",
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

func TestHandler_Login_MissingBothEmailAndPhone(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]string{
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

func TestHandler_Login_InvalidPhone(t *testing.T) {
	app, _, _ := setupTestApp()

	payload := map[string]string{
		"phone":    "089999999999",
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

func TestHandler_ChangePassword(t *testing.T) {
	app, svc, _ := setupTestApp()

	regResp, _, err := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Password Change Tester",
		Email:    "pwchange@example.com",
		Password: "InitialPassword123!",
		Phone:    "0877889900",
		Role:     domain.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	accessToken := regResp.Tokens.AccessToken

	// Change password via PUT /api/v1/auth/password
	payload := auth.ChangePasswordRequest{
		CurrentPassword: "InitialPassword123!",
		NewPassword:     "NewPassword12345!",
		ConfirmPassword: "NewPassword12345!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Login_BengkolAdmin_NoWorkshops_Forbidden(t *testing.T) {
	app, svc, _, _ := setupTestAppWithRepo()

	_, _, err := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Zero Workshop User",
		Email:    "zerows@example.com",
		Password: "password123",
		Phone:    "0888111222",
		Role:     domain.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	payload := map[string]interface{}{
		"email":    "zerows@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("appName", "bengkolAdmin")
	req.Header.Set("appDevice", "web")
	req.Header.Set("appVersion", "1.0")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for zero workshops on bengkolAdmin, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp response.Response
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error == nil || errResp.Error.Code != response.ErrCodeNoWorkshopAccess {
		t.Errorf("expected error code %s, got %+v", response.ErrCodeNoWorkshopAccess, errResp.Error)
	}
}

func TestHandler_Login_BengkolAdmin_SingleWorkshop_AutoSelected(t *testing.T) {
	app, svc, repo, _ := setupTestAppWithRepo()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Single WS Owner",
		Email:    "singlews@example.com",
		Password: "password123",
		Phone:    "0888222333",
		Role:     domain.RoleOwner,
	})

	wsID := uuid.New()
	ws := domain.Workshop{ID: wsID, OwnerID: regResp.User.ID, Name: "Workshop One", Status: domain.WorkshopStatusActive}
	repo.workshopsByUserID[regResp.User.ID] = []domain.Workshop{ws}
	repo.workshopsByID[wsID] = &ws

	payload := map[string]interface{}{
		"email":    "singlews@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("appName", "bengkolAdmin")
	req.Header.Set("appDevice", "web")
	req.Header.Set("appVersion", "1.0")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    auth.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data.SelectedWorkshopID == nil || *resp.Data.SelectedWorkshopID != wsID {
		t.Errorf("expected auto-selected workshop ID %s, got %v", wsID, resp.Data.SelectedWorkshopID)
	}
	if resp.Data.RequiresWorkshopSelection {
		t.Errorf("expected requires_workshop_selection = false for single workshop")
	}
}

func TestHandler_Login_BengkolAdmin_MultipleWorkshops_RequiresSelection(t *testing.T) {
	app, svc, repo, _ := setupTestAppWithRepo()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Multi WS Owner",
		Email:    "multiws@example.com",
		Password: "password123",
		Phone:    "0888333444",
		Role:     domain.RoleOwner,
	})

	ws1 := domain.Workshop{ID: uuid.New(), OwnerID: regResp.User.ID, Name: "Shop A", Status: domain.WorkshopStatusActive}
	ws2 := domain.Workshop{ID: uuid.New(), OwnerID: regResp.User.ID, Name: "Shop B", Status: domain.WorkshopStatusActive}
	repo.workshopsByUserID[regResp.User.ID] = []domain.Workshop{ws1, ws2}
	repo.workshopsByID[ws1.ID] = &ws1
	repo.workshopsByID[ws2.ID] = &ws2

	payload := map[string]interface{}{
		"email":    "multiws@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("appName", "bengkolAdmin")
	req.Header.Set("appDevice", "web")
	req.Header.Set("appVersion", "1.0")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    auth.AuthResponse `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if !resp.Data.RequiresWorkshopSelection {
		t.Errorf("expected requires_workshop_selection = true for multiple workshops")
	}
	if len(resp.Data.Workshops) != 2 {
		t.Errorf("expected 2 workshops, got %d", len(resp.Data.Workshops))
	}
}

func TestHandler_GetPermissions_SuccessAndAlias(t *testing.T) {
	app, svc, repo, _ := setupTestAppWithRepo()

	userResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Perms Tester",
		Email:    "permstester@example.com",
		Password: "password123",
		Phone:    "0888444555",
		Role:     domain.RoleOwner,
	})

	wsID := uuid.New()
	ws := domain.Workshop{ID: wsID, OwnerID: userResp.User.ID, Name: "Perms Workshop", Status: domain.WorkshopStatusActive}
	repo.workshopsByID[wsID] = &ws

	// 1. POST /api/v1/auth/get-permissions
	payload := map[string]interface{}{
		"workshop_id": wsID,
	}
	body, _ := json.Marshal(payload)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/get-permissions", bytes.NewReader(body))
	req1.Header.Set("Authorization", "Bearer "+userResp.Tokens.AccessToken)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()

	app.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /get-permissions, got %d: %s", rec1.Code, rec1.Body.String())
	}

	var resp1 struct {
		Success bool                        `json:"success"`
		Data    auth.GetPermissionsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("failed to decode get-permissions response: %v", err)
	}

	if resp1.Data.WorkshopID != wsID {
		t.Errorf("expected workshop ID %s, got %s", wsID, resp1.Data.WorkshopID)
	}
	if resp1.Data.Permissions == nil || !resp1.Data.Permissions.CanManageRolesAndPermissions {
		t.Errorf("expected owner permissions granted")
	}

	// 2. Alias: POST /api/v1/auth/select-workshop
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/select-workshop", bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+userResp.Tokens.AccessToken)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()

	app.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for alias /select-workshop, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandler_GetPermissions_UnauthorizedWorkshop(t *testing.T) {
	app, svc, repo, _ := setupTestAppWithRepo()

	userResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Other User",
		Email:    "otheruser@example.com",
		Password: "password123",
		Phone:    "0888555666",
		Role:     domain.RoleCustomer,
	})

	alienWsID := uuid.New()
	alienWs := domain.Workshop{ID: alienWsID, OwnerID: uuid.New(), Name: "Alien Workshop", Status: domain.WorkshopStatusActive}
	repo.workshopsByID[alienWsID] = &alienWs

	payload := map[string]interface{}{
		"workshop_id": alienWsID,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/get-permissions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userResp.Tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized workshop, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_GetMe_WithWorkshopContext(t *testing.T) {
	app, svc, repo, jwtMgr := setupTestAppWithRepo()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Profile Detail Tester",
		Email:    "detailme@example.com",
		Password: "password123",
		Phone:    "0888666777",
		Role:     domain.RoleOwner,
	})

	wsID := uuid.New()
	ws := domain.Workshop{ID: wsID, OwnerID: regResp.User.ID, Name: "Profile Detail Workshop", Status: domain.WorkshopStatusActive}
	repo.workshopsByUserID[regResp.User.ID] = []domain.Workshop{ws}
	repo.workshopsByID[wsID] = &ws

	perms := domain.DefaultPermissionsForRole(domain.EmployeeRoleOwner)
	scopedTokens, _, _ := jwtMgr.GenerateScopedTokenPair(regResp.User, &wsID, domain.RoleOwner, &perms)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+scopedTokens.AccessToken)
	req.Header.Set("appName", "bengkolAdmin")
	req.Header.Set("appDevice", "web")
	req.Header.Set("appVersion", "1.0")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool                    `json:"success"`
		Data    auth.UserDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal UserDetailResponse: %v", err)
	}

	if resp.Data.SelectedWorkshopID == nil || *resp.Data.SelectedWorkshopID != wsID {
		t.Errorf("expected selected workshop ID %s, got %v", wsID, resp.Data.SelectedWorkshopID)
	}
	if resp.Data.Role != domain.RoleOwner {
		t.Errorf("expected role OWNER, got %s", resp.Data.Role)
	}
	if resp.Data.Permissions == nil || !resp.Data.Permissions.CanManageRolesAndPermissions {
		t.Errorf("expected permissions in /me response")
	}
}

