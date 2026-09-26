package service_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/service"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithService() (chi.Router, *mockServiceRepo, *security.JWTManager) {
	svcRepo := newMockServiceRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	svcUseCase := service.NewService(svcRepo, log)
	svcHdl := service.NewHandler(svcUseCase, log)
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
		ServiceHandler: svcHdl,
		JWTManager:     jwtMgr,
	})

	return router, svcRepo, jwtMgr
}

func TestHandler_Service_Create_Success(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithService()

	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	owner := &domain.User{ID: ownerID, Email: "owner@bengkol.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	createPayload := map[string]interface{}{
		"name":             "Tune Up Mesin",
		"description":      "Pembersihan injektor & ruang bakar",
		"price":            250000,
		"duration_minutes": 60,
	}
	body, _ := json.Marshal(createPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workshops/"+wsID.String()+"/services", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Service_ListByWorkshop(t *testing.T) {
	app, repo, _ := setupTestAppWithService()

	wsID := uuid.New()
	id := uuid.New()
	repo.services[id] = &domain.Service{
		ID:              id,
		WorkshopID:      wsID,
		Name:            "Ganti Kampas Rem",
		Price:           75000,
		DurationMinutes: 30,
		IsActive:        true,
		CreatedAt:       time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/services", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
