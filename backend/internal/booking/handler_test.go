package booking_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/booking"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithBooking() (chi.Router, *mockBookingRepo, *security.JWTManager) {
	bRepo := newMockBookingRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	bUseCase := booking.NewService(bRepo, log)
	bHdl := booking.NewHandler(bUseCase, log)
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
		BookingHandler: bHdl,
		JWTManager:     jwtMgr,
	})

	return router, bRepo, jwtMgr
}

func getNextMonday() string {
	now := time.Now().UTC()
	daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	return now.AddDate(0, 0, daysUntilMonday).Format("2006-01-02")
}

func TestHandler_Booking_GetAvailableSlots(t *testing.T) {
	app, repo, _ := setupTestAppWithBooking()

	wsID := uuid.New()
	repo.operatingHours[wsID.String()+":1"] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
		IsClosed:   false,
	}

	mondayDate := getNextMonday()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/available-slots?date="+mondayDate, nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Booking_CreateAndCancel(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithBooking()

	wsID := uuid.New()
	srvID := uuid.New()
	customerID := uuid.New()

	customer := &domain.User{ID: customerID, Email: "customer@bengkol.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.operatingHours[wsID.String()+":1"] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	// 1. Create Booking -> 201 Created
	createPayload := map[string]interface{}{
		"workshop_id":  wsID.String(),
		"service_id":   srvID.String(),
		"booking_date": mondayDate,
		"booking_time": "10:00:00",
	}
	body, _ := json.Marshal(createPayload)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewReader(body))
	reqCreate.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()

	app.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on create booking, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp struct {
		Success bool            `json:"success"`
		Data    *domain.Booking `json:"data"`
	}
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	bookingID := createResp.Data.ID

	// 2. Get Booking Detail -> 200 OK
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/"+bookingID.String(), nil)
	reqGet.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recGet := httptest.NewRecorder()

	app.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get booking, got %d", recGet.Code)
	}

	// 3. Cancel Booking -> 200 OK
	reqCancel := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+bookingID.String()+"/cancel", nil)
	reqCancel.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recCancel := httptest.NewRecorder()

	app.ServeHTTP(recCancel, reqCancel)
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected status 200 on cancel booking, got %d: %s", recCancel.Code, recCancel.Body.String())
	}
}

func TestHandler_Booking_ListMyBookings(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithBooking()

	customerID := uuid.New()
	customer := &domain.User{ID: customerID, Email: "cust@bengkol.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	bID := uuid.New()
	repo.bookings[bID] = &domain.Booking{
		ID:          bID,
		CustomerID:  customerID,
		BookingDate: getNextMonday(),
		Status:      domain.BookingStatusConfirmed,
		CreatedAt:   time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bookings", nil)
	req.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list my bookings, got %d", rec.Code)
	}
}
