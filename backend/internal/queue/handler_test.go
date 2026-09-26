package queue_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/queue"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestAppWithQueue() (chi.Router, *mockQueueRepo, *security.JWTManager) {
	qRepo := newMockQueueRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	qUseCase := queue.NewService(qRepo, log, nil, nil)
	qHdl := queue.NewHandler(qUseCase, log)
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
		QueueHandler:  qHdl,
		JWTManager:    jwtMgr,
	})

	return router, qRepo, jwtMgr
}

func TestHandler_Queue_CheckInAndStatusTransitions(t *testing.T) {
	app, repo, jwtMgr := setupTestAppWithQueue()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()
	today := time.Now().Format("2006-01-02")

	customer := &domain.User{ID: custID, Email: "cust@test.com", Role: domain.RoleCustomer}
	customerTokens, _, _ := jwtMgr.GenerateTokenPair(customer)

	owner := &domain.User{ID: ownerID, Email: "owner@test.com", Role: domain.RoleOwner}
	ownerTokens, _, _ := jwtMgr.GenerateTokenPair(owner)

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		BookingDate: today,
		Status:      domain.BookingStatusConfirmed,
	}

	// 1. Customer Check-in -> 201 Created
	reqCheckIn := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+bookingID.String()+"/check-in", nil)
	reqCheckIn.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recCheckIn := httptest.NewRecorder()

	app.ServeHTTP(recCheckIn, reqCheckIn)
	if recCheckIn.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on check-in, got %d: %s", recCheckIn.Code, recCheckIn.Body.String())
	}

	// 2. Customer Active Queue -> 200 OK
	reqActive := httptest.NewRequest(http.MethodGet, "/api/v1/me/queue/active", nil)
	reqActive.Header.Set("Authorization", "Bearer "+customerTokens.AccessToken)
	recActive := httptest.NewRecorder()

	app.ServeHTTP(recActive, reqActive)
	if recActive.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get active queue, got %d: %s", recActive.Code, recActive.Body.String())
	}

	// Extract queue ID from repo
	var createdQueue *domain.Queue
	for _, q := range repo.queues {
		if q.BookingID == bookingID {
			createdQueue = q
			break
		}
	}
	if createdQueue == nil {
		t.Fatalf("expected queue to exist in repo")
	}

	// 3. Workshop Owner lists queues -> 200 OK
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/queues", nil)
	reqList.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recList := httptest.NewRecorder()

	app.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected status 200 on list workshop queues, got %d: %s", recList.Code, recList.Body.String())
	}

	// 4. Workshop Owner calls next -> 200 OK
	reqCall := httptest.NewRequest(http.MethodPost, "/api/v1/workshops/"+wsID.String()+"/queues/call-next", nil)
	reqCall.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recCall := httptest.NewRecorder()

	app.ServeHTTP(recCall, reqCall)
	if recCall.Code != http.StatusOK {
		t.Fatalf("expected status 200 on call next, got %d: %s", recCall.Code, recCall.Body.String())
	}

	// 5. Workshop Owner starts service -> 200 OK
	reqStart := httptest.NewRequest(http.MethodPost, "/api/v1/queues/"+createdQueue.ID.String()+"/start", nil)
	reqStart.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recStart := httptest.NewRecorder()

	app.ServeHTTP(recStart, reqStart)
	if recStart.Code != http.StatusOK {
		t.Fatalf("expected status 200 on start service, got %d: %s", recStart.Code, recStart.Body.String())
	}

	// 6. Workshop Owner completes service -> 200 OK
	reqComplete := httptest.NewRequest(http.MethodPost, "/api/v1/queues/"+createdQueue.ID.String()+"/complete", nil)
	reqComplete.Header.Set("Authorization", "Bearer "+ownerTokens.AccessToken)
	recComplete := httptest.NewRecorder()

	app.ServeHTTP(recComplete, reqComplete)
	if recComplete.Code != http.StatusOK {
		t.Fatalf("expected status 200 on complete service, got %d: %s", recComplete.Code, recComplete.Body.String())
	}
}

func TestHandler_Queue_GetSummary(t *testing.T) {
	app, _, _ := setupTestAppWithQueue()

	wsID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workshops/"+wsID.String()+"/queues/summary", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on queue summary, got %d: %s", rec.Code, rec.Body.String())
	}
}
