package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
)

func TestRequestID_GeneratesIfMissing(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	reqID := rec.Header().Get(middleware.RequestIDHeader)
	if reqID == "" {
		t.Fatalf("expected X-Request-ID header to be set in response")
	}
}

func TestRequestID_PreservesExisting(t *testing.T) {
	customID := "custom-req-id-123"
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(middleware.RequestIDHeader, customID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get(middleware.RequestIDHeader) != customID {
		t.Errorf("expected %s, got %s", customID, rec.Header().Get(middleware.RequestIDHeader))
	}
}

func TestRecoverer_RecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	panickingHandler := middleware.Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic simulation")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	panickingHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestCORS_SetsHeaders(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}

	handler := middleware.CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS allow origin header, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
