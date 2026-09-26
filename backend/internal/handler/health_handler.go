package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/bengkol/backend/pkg/response"
)

// Pinger defines an interface for checking dependency connectivity.
type Pinger interface {
	PingCheck(ctx context.Context) error
}

// HealthHandler handles liveness and readiness probes.
type HealthHandler struct {
	db        Pinger
	startTime time.Time
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{
		db:        db,
		startTime: time.Now(),
	}
}

// HealthStatus represents the payload for /health and /ready.
type HealthStatus struct {
	Status    string            `json:"status"`
	Uptime    string            `json:"uptime"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Health returns basic service liveness.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:    "UP",
		Uptime:    time.Since(h.startTime).String(),
		Timestamp: time.Now().UTC(),
	}

	response.Success(w, http.StatusOK, status)
}

// Ready returns service readiness including database connectivity check.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	isReady := true

	if h.db != nil {
		if err := h.db.PingCheck(r.Context()); err != nil {
			checks["database"] = "DOWN: " + err.Error()
			isReady = false
		} else {
			checks["database"] = "UP"
		}
	} else {
		checks["database"] = "NOT_CONFIGURED"
	}

	status := HealthStatus{
		Status:    "READY",
		Uptime:    time.Since(h.startTime).String(),
		Timestamp: time.Now().UTC(),
		Checks:    checks,
	}

	if !isReady {
		status.Status = "UNHEALTHY"
		response.JSON(w, http.StatusServiceUnavailable, response.Response{
			Success: false,
			Data:    status,
			Error: &response.APIError{
				Code:    response.ErrCodeDatabaseUnavailable,
				Message: "Database connection check failed",
			},
		})
		return
	}

	response.Success(w, http.StatusOK, status)
}
