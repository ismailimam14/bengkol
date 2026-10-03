package workshop_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/auth"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/workshop"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithWorkshop() (chi.Router, *mockWorkshopRepo, *security.JWTManager) {
	wsRepo := newMockWorkshopRepo()
	authRepo := &mockAuthRepoAdapter{users: make(map[string]*domain.User)}
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-refresh-secret", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	wsSvc := workshop.NewService(wsRepo, log)
	wsHdl := workshop.NewHandler(wsSvc, log)

	authSvc := auth.NewService(authRepo, jwtMgr, log)
	authHdl := auth.NewHandler(authSvc, log)
	healthHdl := handler.NewHealthHandler(nil)

	cfg := &config.Config{
		AppEnv: "development",
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
	}

	router := handler.NewRouter(handler.RouterConfig{
		Config:          cfg,
		Logger:          log,
		HealthHandler:   healthHdl,
		AuthHandler:     authHdl,
		WorkshopHandler: wsHdl,
		JWTManager:      jwtMgr,
	})

	return router, wsRepo, jwtMgr
}

type mockAuthRepoAdapter struct {
	users map[string]*domain.User
}

func (m *mockAuthRepoAdapter) CreateUser(ctx context.Context, u *domain.User) error {
	m.users[u.Email] = u
	return nil
}
func (m *mockAuthRepoAdapter) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}
func (m *mockAuthRepoAdapter) GetUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Phone == phone {
			return u, nil
		}
	}
	return nil, auth.ErrUserNotFound
}
func (m *mockAuthRepoAdapter) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, auth.ErrUserNotFound
}
func (m *mockAuthRepoAdapter) SaveRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	return nil
}
func (m *mockAuthRepoAdapter) GetRefreshToken(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	return nil, nil
}
func (m *mockAuthRepoAdapter) RevokeRefreshToken(ctx context.Context, hash string) error {
	return nil
}
func (m *mockAuthRepoAdapter) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func TestHandler_Workshop_NearbySearch(t *testing.T) {
	app, repo, _ := setupTestAppWithWorkshop()

	id := uuid.New()
	repo.workshops[id] = &domain.Workshop{
		ID:        id,
		Name:      "Bengkel Senayan",
		Address:   "Jl. Asia Afrika",
		Latitude:  -6.2200,
		Longitude: 106.8000,
		Phone:     "0812345678",
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/nearby?lat=-6.2200&lng=106.8000&radius=5000", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected response.success to be true")
	}
}

func TestHandler_Workshop_Create_RoleAuthorization(t *testing.T) {
	app, _, jwtMgr := setupTestAppWithWorkshop()

	owner := &domain.User{ID: uuid.New(), Email: "owner@bengkol.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	customer := &domain.User{ID: uuid.New(), Email: "customer@bengkol.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	createPayload := map[string]interface{}{
		"name":      "New Workshop",
		"address":   "Jl. Pemuda No. 1",
		"latitude":  -6.2,
		"longitude": 106.8,
		"phone":     "0811223344",
	}
	body, _ := json.Marshal(createPayload)

	// 1. OWNER creates workshop -> 201 CREATED
	reqOwner := httptest.NewRequest(http.MethodPost, "/api/v1/workshops", bytes.NewReader(body))
	reqOwner.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	reqOwner.Header.Set("Content-Type", "application/json")
	recOwner := httptest.NewRecorder()

	app.ServeHTTP(recOwner, reqOwner)
	if recOwner.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for OWNER, got %d: %s", recOwner.Code, recOwner.Body.String())
	}

	// 2. CUSTOMER creates workshop -> 403 FORBIDDEN
	reqCustomer := httptest.NewRequest(http.MethodPost, "/api/v1/workshops", bytes.NewReader(body))
	reqCustomer.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	reqCustomer.Header.Set("Content-Type", "application/json")
	recCustomer := httptest.NewRecorder()

	app.ServeHTTP(recCustomer, reqCustomer)
	if recCustomer.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for CUSTOMER, got %d", recCustomer.Code)
	}
}

func TestHandler_Workshop_Update_OwnershipForbidden(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithWorkshop()

	realOwner := &domain.User{ID: uuid.New(), Email: "real@bengkol.com", Role: domain.RoleOwner}
	realOwnerTokens, _, _ := jwtMgr.GenerateTokenPair(realOwner)

	otherOwner := &domain.User{ID: uuid.New(), Email: "other@bengkol.com", Role: domain.RoleOwner}
	otherOwnerTokens, _, _ := jwtMgr.GenerateTokenPair(otherOwner)

	wsID := uuid.New()
	repo.workshops[wsID] = &domain.Workshop{
		ID:        wsID,
		OwnerID:   realOwner.ID,
		Name:      "Original Workshop",
		Address:   "Alamat Asli",
		Latitude:  -6.2,
		Longitude: 106.8,
		Phone:     "08123456",
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	updatePayload := map[string]string{
		"name": "Modified Workshop Name",
	}
	body, _ := json.Marshal(updatePayload)

	// 1. Other Owner attempts to modify Real Owner's workshop -> 403 FORBIDDEN
	reqOther := httptest.NewRequest(http.MethodPatch, "/api/v1/workshops/"+wsID.String(), bytes.NewReader(body))
	reqOther.Header.Set("Authorization", "Bearer "+otherOwnerTokens.AccessToken)
	reqOther.Header.Set("Content-Type", "application/json")
	recOther := httptest.NewRecorder()

	app.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 when updating non-owned workshop, got %d: %s", recOther.Code, recOther.Body.String())
	}

	// 2. Real Owner modifies own workshop -> 200 OK
	reqReal := httptest.NewRequest(http.MethodPatch, "/api/v1/workshops/"+wsID.String(), bytes.NewReader(body))
	reqReal.Header.Set("Authorization", "Bearer "+realOwnerTokens.AccessToken)
	reqReal.Header.Set("Content-Type", "application/json")
	recReal := httptest.NewRecorder()

	app.ServeHTTP(recReal, reqReal)
	if recReal.Code != http.StatusOK {
		t.Fatalf("expected status 200 for real owner update, got %d: %s", recReal.Code, recReal.Body.String())
	}
}

func TestHandler_Workshop_GetByID(t *testing.T) {
	app, repo, _ := setupTestAppWithWorkshop()

	wsID := uuid.New()
	repo.workshops[wsID] = &domain.Workshop{
		ID:        wsID,
		Name:      "Detail Test Workshop",
		Address:   "Jl. Sudirman",
		Phone:     "081234",
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String(), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
