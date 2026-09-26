package workshop

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles workshop REST API requests.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Workshop Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// PublicRoutes returns routes accessible without authentication or with optional auth.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Get("/nearby", h.FindNearby)
	r.Get("/{id}", h.GetByID)
	r.Get("/{id}/operating-hours", h.GetOperatingHours)

	return r
}

// ProtectedRoutes returns routes requiring OWNER or ADMIN authentication.
func (h *Handler) ProtectedRoutes() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequireAuthenticated)

	// Owner workshop creation
	r.With(middleware.RequireRoles(domain.RoleOwner)).Post("/", h.Create)

	// Owner / Admin workshop modifications
	r.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Patch("/{id}", h.Update)
	r.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Put("/{id}/operating-hours", h.UpdateOperatingHours)

	return r
}

// List returns a paginated list of workshops.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	}
	search := r.URL.Query().Get("search")

	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	filter := WorkshopFilter{
		Search: search,
	}

	statusParam := r.URL.Query().Get("status")
	if statusParam != "" {
		st := domain.WorkshopStatus(statusParam)
		filter.Status = &st
	}

	list, meta, err := h.service.ListWorkshops(r.Context(), pagination, filter)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to list workshops", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve workshops")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// FindNearby performs a PostGIS radius search for workshops.
func (h *Handler) FindNearby(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	if latStr == "" {
		latStr = r.URL.Query().Get("latitude")
	}
	lngStr := r.URL.Query().Get("lng")
	if lngStr == "" {
		lngStr = r.URL.Query().Get("longitude")
	}
	radiusStr := r.URL.Query().Get("radius")
	limitStr := r.URL.Query().Get("limit")

	if latStr == "" || lngStr == "" {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Query parameters 'lat' (or 'latitude') and 'lng' (or 'longitude') are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid 'lat' parameter")
		return
	}

	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid 'lng' parameter")
		return
	}

	radius, _ := strconv.ParseFloat(radiusStr, 64)
	limit, _ := strconv.Atoi(limitStr)

	params := NearbyParams{
		Latitude:     lat,
		Longitude:    lng,
		RadiusMeters: radius,
		Limit:        limit,
	}

	list, err := h.service.FindNearbyWorkshops(r.Context(), params)
	if err != nil {
		if errors.Is(err, ErrInvalidCoordinates) {
			response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid latitude or longitude range")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to find nearby workshops", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to find nearby workshops")
		return
	}

	response.Success(w, http.StatusOK, list)
}

// GetByID returns the details of a workshop including operating hours.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	ws, err := h.service.GetWorkshopByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get workshop", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve workshop")
		return
	}

	response.Success(w, http.StatusOK, ws)
}

// Create handles workshop creation by authenticated owners.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req CreateWorkshopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	ws, valErrors, err := h.service.CreateWorkshop(r.Context(), ownerID, req)
	if err != nil {
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create workshop", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create workshop")
		return
	}

	response.Success(w, http.StatusCreated, ws)
}

// Update handles workshop profile updates with ownership authorization.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	var req UpdateWorkshopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	ws, valErrors, err := h.service.UpdateWorkshop(r.Context(), id, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not own this workshop")
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update workshop", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update workshop")
		return
	}

	response.Success(w, http.StatusOK, ws)
}

// GetOperatingHours returns operating hours of a workshop.
func (h *Handler) GetOperatingHours(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	hours, err := h.service.GetOperatingHours(r.Context(), id)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to get operating hours", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve operating hours")
		return
	}

	response.Success(w, http.StatusOK, hours)
}

// UpdateOperatingHours updates operating hours with ownership validation.
func (h *Handler) UpdateOperatingHours(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	var inputs []OperatingHourInput
	if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body; expected array of operating hours")
		return
	}

	hours, valErrors, err := h.service.UpdateOperatingHours(r.Context(), id, userID, role, inputs)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not own this workshop")
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update operating hours", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update operating hours")
		return
	}

	response.Success(w, http.StatusOK, hours)
}

// GetMyWorkshops returns all workshops owned by the authenticated owner.
func (h *Handler) GetMyWorkshops(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	list, err := h.service.GetMyWorkshops(r.Context(), ownerID)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to get owner workshops", "owner_id", ownerID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve workshops")
		return
	}

	response.Success(w, http.StatusOK, list)
}
