package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/pkg/response"
)

type mockPinger struct {
	err error
}

func (m *mockPinger) PingCheck(ctx context.Context) error {
	return m.err
}

func TestHealthHandler_Health(t *testing.T) {
	h := handler.NewHealthHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true")
	}
}

func TestHealthHandler_Ready_Success(t *testing.T) {
	mockDB := &mockPinger{err: nil}
	h := handler.NewHealthHandler(mockDB)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	h.Ready(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true")
	}
}

func TestHealthHandler_Ready_DatabaseDown(t *testing.T) {
	mockDB := &mockPinger{err: errors.New("connection refused")}
	h := handler.NewHealthHandler(mockDB)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	h.Ready(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Errorf("expected success to be false on database down")
	}

	if resp.Error == nil || resp.Error.Code != response.ErrCodeDatabaseUnavailable {
		t.Errorf("expected error code %s, got %+v", response.ErrCodeDatabaseUnavailable, resp.Error)
	}
}
