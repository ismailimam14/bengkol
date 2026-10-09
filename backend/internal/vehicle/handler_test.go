package vehicle_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/vehicle"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithVehicle() (chi.Router, *mockVehicleRepo, *security.JWTManager) {
	vRepo := newMockVehicleRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	vUseCase := vehicle.NewService(vRepo, log)
	vHdl := vehicle.NewHandler(vUseCase, log)
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
		Config:         cfg,
		Logger:         log,
		HealthHandler:  healthHdl,
		VehicleHandler: vHdl,
		JWTManager:     jwtMgr,
	})

	return router, vRepo, jwtMgr
}

func TestHandler_Vehicle_CRUD(t *testing.T) {
	app, _, jwtMgr := setupTestAppWithVehicle()

	customerID := uuid.New()
	customer := &domain.User{ID: customerID, Email: "cust@bengkol.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	// 1. Create Vehicle -> 201 Created
	createPayload := map[string]interface{}{
		"license_plate": "B 1234 XYZ",
		"brand":         "Honda",
		"model":         "Vario 150",
		"year":          2022,
		"vehicle_type":  "MOTORCYCLE",
		"color":         "Hitam",
		"notes":         "Motor kesayangan",
	}
	body, _ := json.Marshal(createPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vehicles", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on create vehicle, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp struct {
		Success bool           `json:"success"`
		Data    domain.Vehicle `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	vehID := createResp.Data.ID
	if createResp.Data.LicensePlate != "B 1234 XYZ" {
		t.Errorf("expected license plate B 1234 XYZ, got %s", createResp.Data.LicensePlate)
	}

	// 2. List Vehicles via /api/v1/vehicles -> 200 OK
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/vehicles", nil)
	reqList.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recList := httptest.NewRecorder()

	app.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list vehicles, got %d: %s", recList.Code, recList.Body.String())
	}

	// 3. List Vehicles via /api/v1/me/vehicles -> 200 OK
	reqMeList := httptest.NewRequest(http.MethodGet, "/api/v1/me/vehicles", nil)
	reqMeList.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recMeList := httptest.NewRecorder()

	app.ServeHTTP(recMeList, reqMeList)

	if recMeList.Code != http.StatusOK {
		t.Fatalf("expected status 200 on /me/vehicles, got %d: %s", recMeList.Code, recMeList.Body.String())
	}

	// 4. Get Vehicle by ID -> 200 OK
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/vehicles/"+vehID.String(), nil)
	reqGet.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recGet := httptest.NewRecorder()

	app.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get vehicle, got %d: %s", recGet.Code, recGet.Body.String())
	}

	// 5. Update Vehicle -> 200 OK
	updatePayload := map[string]interface{}{
		"brand": "Honda Modif",
		"color": "Merah",
	}
	updateBody, _ := json.Marshal(updatePayload)

	reqUpdate := httptest.NewRequest(http.MethodPatch, "/api/v1/vehicles/"+vehID.String(), bytes.NewReader(updateBody))
	reqUpdate.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	reqUpdate.Header.Set("Content-Type", "application/json")
	recUpdate := httptest.NewRecorder()

	app.ServeHTTP(recUpdate, reqUpdate)

	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected status 200 on update vehicle, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 6. Delete Vehicle -> 200 OK
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/vehicles/"+vehID.String(), nil)
	reqDel.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recDel := httptest.NewRecorder()

	app.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusOK {
		t.Fatalf("expected status 200 on delete vehicle, got %d: %s", recDel.Code, recDel.Body.String())
	}

	// 7. Get after Delete -> 404 Not Found
	reqGetAfterDel := httptest.NewRequest(http.MethodGet, "/api/v1/vehicles/"+vehID.String(), nil)
	reqGetAfterDel.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recGetAfterDel := httptest.NewRecorder()

	app.ServeHTTP(recGetAfterDel, reqGetAfterDel)

	if recGetAfterDel.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 on get deleted vehicle, got %d", recGetAfterDel.Code)
	}
}
