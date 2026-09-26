package history_test

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
	"github.com/bengkol/backend/internal/history"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithHistory() (chi.Router, *mockHistoryRepo, *security.JWTManager) {
	hRepo := newMockHistoryRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	hUseCase := history.NewService(hRepo, log)
	hHdl := history.NewHandler(hUseCase, log)
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
		HistoryHandler: hHdl,
		JWTManager:     jwtMgr,
	})

	return router, hRepo, jwtMgr
}

func TestHandler_History_CreateAndGet(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithHistory()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()
	bookingID := uuid.New()

	owner := &domain.User{ID: ownerID, Email: "owner@history.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	customer := &domain.User{ID: custID, Email: "cust@history.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Kampas Rem Depan",
		SellingPrice: 85000,
		Stock:        5,
		IsActive:     true,
	}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusInService,
		Service:     &domain.Service{ID: srvID, WorkshopID: wsID, Name: "Ganti Rem", Price: 50000},
	}

	// 1. Owner creates service history record -> 201 Created
	payload := history.CreateHistoryRequest{
		BookingID: bookingID,
		Notes:     "Pekerjaan selesai dengan baik",
		SpareParts: []history.CreateHistoryItemRequest{
			{
				SparePartID: spID,
				Quantity:    1,
			},
		},
	}
	body, _ := json.Marshal(payload)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/workshops/"+wsID.String()+"/service-histories", bytes.NewReader(body))
	reqCreate.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()

	app.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on create history, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp struct {
		Success bool                   `json:"success"`
		Data    *domain.ServiceHistory `json:"data"`
	}
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	historyID := createResp.Data.ID

	// 2. Customer gets own service history -> 200 OK
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/service-histories/"+historyID.String(), nil)
	reqGet.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recGet := httptest.NewRecorder()

	app.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get history, got %d: %s", recGet.Code, recGet.Body.String())
	}

	// 3. Customer lists past service histories -> 200 OK
	reqListMe := httptest.NewRequest(http.MethodGet, "/api/v1/me/service-histories", nil)
	reqListMe.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recListMe := httptest.NewRecorder()

	app.ServeHTTP(recListMe, reqListMe)
	if recListMe.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list customer histories, got %d: %s", recListMe.Code, recListMe.Body.String())
	}

	// 4. Workshop owner lists workshop histories -> 200 OK
	reqListWs := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/service-histories", nil)
	reqListWs.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recListWs := httptest.NewRecorder()

	app.ServeHTTP(recListWs, reqListWs)
	if recListWs.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list workshop histories, got %d: %s", recListWs.Code, recListWs.Body.String())
	}
}

func TestHandler_History_GetByBookingID(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithHistory()

	wsID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()
	hID := uuid.New()

	customer := &domain.User{ID: custID, Email: "cust2@history.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	repo.histories[hID] = &domain.ServiceHistory{
		ID:          hID,
		BookingID:   bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		TotalPrice:  120000,
		ServiceDate: "2026-09-28",
		CreatedAt:   time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/"+bookingID.String()+"/service-history", nil)
	req.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get history by booking id, got %d: %s", rec.Code, rec.Body.String())
	}
}
