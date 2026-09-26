package sparepart_test

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
	"github.com/bengkol/backend/internal/sparepart"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithSparePart() (chi.Router, *mockSparePartRepo, *security.JWTManager) {
	spRepo := newMockSparePartRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	spUseCase := sparepart.NewService(spRepo, log)
	spHdl := sparepart.NewHandler(spUseCase, log)
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
		Config:           cfg,
		Logger:           log,
		HealthHandler:    healthHdl,
		SparePartHandler: spHdl,
		JWTManager:       jwtMgr,
	})

	return router, spRepo, jwtMgr
}

func TestHandler_SparePart_Create_Success(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithSparePart()

	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	owner := &domain.User{ID: ownerID, Email: "owner@bengkol.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	createPayload := map[string]interface{}{
		"name":           "Busi Iridium",
		"description":    "Busi performa tinggi",
		"purchase_price": 45000,
		"selling_price":  70000,
		"stock":          20,
	}
	body, _ := json.Marshal(createPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workshops/"+wsID.String()+"/spare-parts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_SparePart_ListByWorkshop(t *testing.T) {
	app, repo, _ := setupTestAppWithSparePart()

	wsID := uuid.New()
	id := uuid.New()
	repo.spareParts[id] = &domain.SparePart{
		ID:           id,
		WorkshopID:   wsID,
		Name:         "Filter Udara",
		SellingPrice: 35000,
		Stock:        8,
		IsActive:     true,
		CreatedAt:    time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/spare-parts", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
