package notification_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/notification"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupNotificationTestApp() (chi.Router, *mockDeviceRepo, *security.JWTManager) {
	repo := newMockDeviceRepo()
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	svc := notification.NewService(repo, log)
	hdl := notification.NewHandler(svc, log)
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
		DeviceHandler: hdl,
		JWTManager:    jwtMgr,
	})

	return router, repo, jwtMgr
}

func TestHandler_RegisterDevice_Success(t *testing.T) {
	router, _, jwtMgr := setupNotificationTestApp()
	userID := uuid.New()
	user := &domain.User{ID: userID, Email: "user@example.com", Role: domain.RoleCustomer}
	tokens, _, _ := jwtMgr.GenerateTokenPair(user)

	body, _ := json.Marshal(notification.RegisterDeviceRequest{
		Token:    "fcm_test_device_token_1234567890",
		Platform: domain.PlatformAndroid,
	})

	req := httptest.NewRequest("POST", "/api/v1/devices", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_UnregisterDevice_Success(t *testing.T) {
	router, repo, jwtMgr := setupNotificationTestApp()
	userID := uuid.New()
	user := &domain.User{ID: userID, Email: "user@example.com", Role: domain.RoleCustomer}
	tokens, _, _ := jwtMgr.GenerateTokenPair(user)

	token := "fcm_token_to_remove_1234567890"
	repo.devices[token] = &domain.UserDevice{
		ID:       uuid.New(),
		UserID:   userID,
		Token:    token,
		Platform: domain.PlatformAndroid,
	}

	req := httptest.NewRequest("DELETE", "/api/v1/devices/"+token, nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_GetMyDevices(t *testing.T) {
	router, repo, jwtMgr := setupNotificationTestApp()
	userID := uuid.New()
	user := &domain.User{ID: userID, Email: "user@example.com", Role: domain.RoleCustomer}
	tokens, _, _ := jwtMgr.GenerateTokenPair(user)

	repo.devices["tok_1"] = &domain.UserDevice{ID: uuid.New(), UserID: userID, Token: "tok_1", Platform: domain.PlatformAndroid}

	req := httptest.NewRequest("GET", "/api/v1/me/devices", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
}
