package review_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/review"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithReview() (chi.Router, *mockReviewRepo, *security.JWTManager) {
	rRepo := newMockReviewRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	rUseCase := review.NewService(rRepo, log)
	rHdl := review.NewHandler(rUseCase, log)
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
		Config:        cfg,
		Logger:        log,
		HealthHandler: healthHdl,
		ReviewHandler: rHdl,
		JWTManager:    jwtMgr,
	})

	return router, rRepo, jwtMgr
}

func TestHandler_Review_CreateGetAndList(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithReview()

	wsID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()

	customer := &domain.User{ID: custID, Email: "cust@review.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Rating: 0, ReviewCount: 0}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusCompleted,
	}

	// 1. Submit review -> 201 Created
	payload := review.CreateReviewRequest{
		BookingID: bookingID,
		Rating:    5,
		Comment:   "Sangat puas dengan hasilnya!",
	}
	body, _ := json.Marshal(payload)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/reviews", bytes.NewReader(body))
	reqCreate.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()

	app.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on submit review, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp struct {
		Success bool           `json:"success"`
		Data    *domain.Review `json:"data"`
	}
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	reviewID := createResp.Data.ID

	// 2. Get review by ID -> 200 OK
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/"+reviewID.String(), nil)
	recGet := httptest.NewRecorder()

	app.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get review by id, got %d: %s", recGet.Code, recGet.Body.String())
	}

	// 3. Get review by Booking ID -> 200 OK
	reqGetByBooking := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/"+bookingID.String()+"/review", nil)
	recGetByBooking := httptest.NewRecorder()

	app.ServeHTTP(recGetByBooking, reqGetByBooking)
	if recGetByBooking.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get review by booking id, got %d: %s", recGetByBooking.Code, recGetByBooking.Body.String())
	}

	// 4. List reviews for workshop -> 200 OK
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/reviews", nil)
	recList := httptest.NewRecorder()

	app.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list workshop reviews, got %d: %s", recList.Code, recList.Body.String())
	}
}
