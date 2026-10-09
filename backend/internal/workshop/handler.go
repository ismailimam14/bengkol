package workshop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles workshop REST API requests.
type Handler struct {
	service   Service
	logger    *logger.Logger
	uploadDir string
}

// NewHandler creates a new Workshop Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service:   service,
		logger:    log,
		uploadDir: filepath.Join("uploads", "workshops"),
	}
}

// SetUploadDir sets the directory where uploaded workshop photos will be saved.
func (h *Handler) SetUploadDir(dir string) {
	h.uploadDir = dir
}

// PublicRoutes returns routes accessible without authentication or with optional auth.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Get("/nearby", h.FindNearby)
	r.Get("/{id}", h.GetByID)
	r.Get("/{id}/operating-hours", h.GetOperatingHours)
	r.Get("/{id}/photos/{photoID}", h.GetPhoto)
	r.Get("/photos/{photoID}", h.GetPhoto)

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

func (h *Handler) readUploadedPhotos(files []*multipart.FileHeader) ([]RawPhotoInput, error) {
	if len(files) == 0 {
		return nil, nil
	}

	var rawPhotos []RawPhotoInput
	for _, fh := range files {
		if fh.Size <= 0 {
			return nil, errors.New("uploaded photo file is empty")
		}
		if fh.Size > 10<<20 { // 10MB limit per photo
			return nil, errors.New("photo exceeds maximum allowed size of 10MB")
		}

		ct := fh.Header.Get("Content-Type")
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif":
			// valid extension
		case "":
			switch ct {
			case "image/jpeg", "image/png", "image/webp", "image/gif":
				// valid
			default:
				ct = "image/jpeg"
			}
		default:
			switch ct {
			case "image/jpeg", "image/png", "image/webp", "image/gif":
				// valid
			default:
				return nil, fmt.Errorf("invalid photo file format (%s); allowed: jpg, jpeg, png, webp, gif", ext)
			}
		}

		if ct == "" || ct == "application/octet-stream" {
			switch ext {
			case ".png":
				ct = "image/png"
			case ".webp":
				ct = "image/webp"
			case ".gif":
				ct = "image/gif"
			default:
				ct = "image/jpeg"
			}
		}

		src, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}

		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read uploaded file: %w", err)
		}

		rawPhotos = append(rawPhotos, RawPhotoInput{
			Data:        data,
			ContentType: ct,
			Filename:    fh.Filename,
		})
	}

	return rawPhotos, nil
}

func (h *Handler) parseCreateRequest(r *http.Request) (CreateWorkshopRequest, error) {
	var req CreateWorkshopRequest

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return req, fmt.Errorf("invalid multipart form data: %w", err)
		}

		req.Name = r.FormValue("name")
		req.Description = r.FormValue("description")
		req.Address = r.FormValue("address")
		req.Phone = r.FormValue("phone")

		if latStr := r.FormValue("latitude"); latStr != "" {
			if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
				req.Latitude = lat
			}
		}
		if lngStr := r.FormValue("longitude"); lngStr != "" {
			if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
				req.Longitude = lng
			}
		}

		if opHoursRaw := r.FormValue("operating_hours"); opHoursRaw != "" {
			var opHours []OperatingHourInput
			if err := json.Unmarshal([]byte(opHoursRaw), &opHours); err == nil {
				req.OperatingHours = opHours
			}
		}

		if employeesRaw := r.FormValue("employees"); employeesRaw != "" {
			var employees []CreateEmployeeInput
			if err := json.Unmarshal([]byte(employeesRaw), &employees); err == nil {
				req.Employees = employees
			}
		}

		var photos []string
		var rawPhotos []RawPhotoInput
		if r.MultipartForm != nil {
			if fhs := r.MultipartForm.File["photos"]; len(fhs) > 0 {
				read, err := h.readUploadedPhotos(fhs)
				if err != nil {
					return req, err
				}
				rawPhotos = append(rawPhotos, read...)
			}
			if fhs := r.MultipartForm.File["photos[]"]; len(fhs) > 0 {
				read, err := h.readUploadedPhotos(fhs)
				if err != nil {
					return req, err
				}
				rawPhotos = append(rawPhotos, read...)
			}
			if vals := r.MultipartForm.Value["photos"]; len(vals) > 0 {
				for _, v := range vals {
					if trimmed := strings.TrimSpace(v); trimmed != "" {
						photos = append(photos, trimmed)
					}
				}
			}
			if vals := r.MultipartForm.Value["photos[]"]; len(vals) > 0 {
				for _, v := range vals {
					if trimmed := strings.TrimSpace(v); trimmed != "" {
						photos = append(photos, trimmed)
					}
				}
			}
		}
		req.Photos = photos
		req.RawPhotos = rawPhotos
		return req, nil
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (h *Handler) parseUpdateRequest(r *http.Request) (UpdateWorkshopRequest, error) {
	var req UpdateWorkshopRequest

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return req, fmt.Errorf("invalid multipart form data: %w", err)
		}

		if r.MultipartForm != nil {
			if _, ok := r.MultipartForm.Value["name"]; ok {
				val := r.FormValue("name")
				req.Name = &val
			}
			if _, ok := r.MultipartForm.Value["description"]; ok {
				val := r.FormValue("description")
				req.Description = &val
			}
			if _, ok := r.MultipartForm.Value["address"]; ok {
				val := r.FormValue("address")
				req.Address = &val
			}
			if _, ok := r.MultipartForm.Value["phone"]; ok {
				val := r.FormValue("phone")
				req.Phone = &val
			}
			if _, ok := r.MultipartForm.Value["latitude"]; ok {
				if lat, err := strconv.ParseFloat(r.FormValue("latitude"), 64); err == nil {
					req.Latitude = &lat
				}
			}
			if _, ok := r.MultipartForm.Value["longitude"]; ok {
				if lng, err := strconv.ParseFloat(r.FormValue("longitude"), 64); err == nil {
					req.Longitude = &lng
				}
			}
			if _, ok := r.MultipartForm.Value["status"]; ok {
				st := domain.WorkshopStatus(r.FormValue("status"))
				req.Status = &st
			}

			hasPhotos := false
			var photos []string
			var rawPhotos []RawPhotoInput
			if fhs := r.MultipartForm.File["photos"]; len(fhs) > 0 {
				hasPhotos = true
				read, err := h.readUploadedPhotos(fhs)
				if err != nil {
					return req, err
				}
				rawPhotos = append(rawPhotos, read...)
			}
			if fhs := r.MultipartForm.File["photos[]"]; len(fhs) > 0 {
				hasPhotos = true
				read, err := h.readUploadedPhotos(fhs)
				if err != nil {
					return req, err
				}
				rawPhotos = append(rawPhotos, read...)
			}
			if vals, ok := r.MultipartForm.Value["photos"]; ok {
				hasPhotos = true
				for _, v := range vals {
					if trimmed := strings.TrimSpace(v); trimmed != "" {
						photos = append(photos, trimmed)
					}
				}
			}
			if vals, ok := r.MultipartForm.Value["photos[]"]; ok {
				hasPhotos = true
				for _, v := range vals {
					if trimmed := strings.TrimSpace(v); trimmed != "" {
						photos = append(photos, trimmed)
					}
				}
			}
			if hasPhotos {
				req.Photos = &photos
				req.RawPhotos = rawPhotos
			}
		}
		return req, nil
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

// GetPhoto returns raw binary photo data directly from the database.
func (h *Handler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	photoIDStr := chi.URLParam(r, "photoID")
	if photoIDStr == "" {
		photoIDStr = chi.URLParam(r, "id")
	}
	photoID, err := uuid.Parse(photoIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid photo ID")
		return
	}

	photo, err := h.service.GetPhoto(r.Context(), photoID)
	if err != nil {
		if errors.Is(err, ErrPhotoNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Photo not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get workshop photo", "photo_id", photoID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve photo")
		return
	}

	contentType := photo.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(photo.Data)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(photo.Data)
}

// Create handles workshop creation by authenticated owners.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	req, err := h.parseCreateRequest(r)
	if err != nil {
		if strings.Contains(err.Error(), "photo") || strings.Contains(err.Error(), "format") || strings.Contains(err.Error(), "empty") || strings.Contains(err.Error(), "size") {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", map[string]string{"photos": err.Error()})
			return
		}
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

	req, err := h.parseUpdateRequest(r)
	if err != nil {
		if strings.Contains(err.Error(), "photo") || strings.Contains(err.Error(), "format") || strings.Contains(err.Error(), "empty") || strings.Contains(err.Error(), "size") {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", map[string]string{"photos": err.Error()})
			return
		}
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
