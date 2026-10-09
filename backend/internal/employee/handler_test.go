package employee_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/employee"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithEmployee() (chi.Router, *mockEmployeeRepo, *security.JWTManager) {
	empRepo := newMockEmployeeRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	empUseCase := employee.NewService(empRepo, log)
	empHdl := employee.NewHandler(empUseCase, log)
	healthHdl := handler.NewHealthHandler(nil)

	cfg := &config.Config{
		AppEnv: "development",
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
	}

	router := handler.NewRouter(handler.RouterConfig{
		Config:          cfg,
		Logger:          log,
		HealthHandler:   healthHdl,
		EmployeeHandler: empHdl,
		JWTManager:      jwtMgr,
	})

	return router, empRepo, jwtMgr
}

func TestHandler_Employee_CRUD(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithEmployee()

	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	owner := &domain.User{ID: ownerID, Email: "owner@bengkol.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	// 1. Owner creates employee (Mechanic) -> 201 Created
	createBody := employee.CreateEmployeeRequest{
		Name:           "Budi Montir",
		Phone:          "0811111111",
		Role:           "MECHANIC",
		Specialization: "Transmission",
	}
	bodyBytes, _ := json.Marshal(createBody)

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workshops/%s/employees", wsID), bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp struct {
		Data domain.WorkshopEmployee `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	empID := createResp.Data.ID
	if empID == uuid.Nil {
		t.Fatal("expected non-nil employee ID")
	}

	// 2. Owner lists employees -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workshops/%s/employees", wsID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)

	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var listResp struct {
		Data []domain.WorkshopEmployee `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("expected 1 employee in list, got %d", len(listResp.Data))
	}

	// 3. Owner gets employee by ID -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workshops/%s/employees/%s", wsID, empID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)

	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	// 4. Owner gets /me -> 200 OK (Owner profile & permissions)
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workshops/%s/employees/me", wsID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)

	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /me, got %d: %s", rr.Code, rr.Body.String())
	}

	// 5. Owner updates employee role to ADMIN_INVENTORY -> 200 OK
	newRole := "ADMIN_INVENTORY"
	updateBody := employee.UpdateEmployeeRequest{
		Role: &newRole,
	}
	uBytes, _ := json.Marshal(updateBody)

	req, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/workshops/%s/employees/%s", wsID, empID), bytes.NewReader(uBytes))
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	// 6. Owner deletes employee -> 200 OK
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workshops/%s/employees/%s", wsID, empID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)

	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_Employee_UnauthorizedForbidden(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithEmployee()

	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	randomUserID := uuid.New()
	randomUser := &domain.User{ID: randomUserID, Email: "customer@bengkol.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(randomUser)

	createBody := employee.CreateEmployeeRequest{
		Name:  "Test Montir",
		Phone: "0812345678",
		Role:  "MECHANIC",
	}
	bodyBytes, _ := json.Marshal(createBody)

	// Random customer tries to create employee -> 403 Forbidden
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workshops/%s/employees", wsID), bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d: %s", rr.Code, rr.Body.String())
	}
}
