package employee

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

// Handler handles workshop employee HTTP requests.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Employee Handler instance.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// List returns a paginated list of employees for a workshop.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	callerRole, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	}

	filter := EmployeeFilter{
		Search: r.URL.Query().Get("search"),
	}

	if roleStr := r.URL.Query().Get("role"); roleStr != "" {
		if rEnum, ok := domain.NormalizeEmployeeRole(roleStr); ok {
			filter.Role = &rEnum
		}
	}

	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		st := domain.EmployeeStatus(statusStr)
		filter.Status = &st
	}

	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	list, meta, err := h.service.ListEmployees(r.Context(), workshopID, callerUserID, callerRole, pagination, filter)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "Access denied")
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to list employees", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve employees")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// GetByID returns an employee's details.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	employeeIDStr := chi.URLParam(r, "employeeId")
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid employee ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	callerRole, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	emp, err := h.service.GetEmployeeByID(r.Context(), workshopID, employeeID, callerUserID, callerRole)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "Access denied")
			return
		}
		if errors.Is(err, ErrEmployeeNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Employee not found")
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get employee", "employee_id", employeeID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve employee")
		return
	}

	response.Success(w, http.StatusOK, emp)
}

// GetMe returns the authenticated caller's employee profile & permissions in the workshop.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	emp, err := h.service.GetCurrentEmployee(r.Context(), workshopID, callerUserID)
	if err != nil {
		if errors.Is(err, ErrEmployeeNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "You are not an employee of this workshop")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get caller employee record", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve employee profile")
		return
	}

	response.Success(w, http.StatusOK, emp)
}

// Create adds a new employee to the workshop.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	callerRole, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req CreateEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	emp, valErrors, err := h.service.CreateEmployee(r.Context(), workshopID, callerUserID, callerRole, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to manage employees in this workshop")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create employee", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create employee")
		return
	}

	response.Success(w, http.StatusCreated, emp)
}

// Update modifies an existing employee.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	employeeIDStr := chi.URLParam(r, "employeeId")
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid employee ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	callerRole, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req UpdateEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	emp, valErrors, err := h.service.UpdateEmployee(r.Context(), workshopID, employeeID, callerUserID, callerRole, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to manage employees in this workshop")
			return
		}
		if errors.Is(err, ErrCannotModifyOwner) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "Managers cannot modify workshop owners")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		if errors.Is(err, ErrEmployeeNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Employee not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update employee", "employee_id", employeeID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update employee")
		return
	}

	response.Success(w, http.StatusOK, emp)
}

// Delete removes an employee from the workshop.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	employeeIDStr := chi.URLParam(r, "employeeId")
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid employee ID")
		return
	}

	callerUserID, ok := middleware.GetUserID(r.Context())
	callerRole, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	if err := h.service.DeleteEmployee(r.Context(), workshopID, employeeID, callerUserID, callerRole); err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to delete employees in this workshop")
			return
		}
		if errors.Is(err, ErrCannotModifyOwner) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "Managers cannot delete workshop owners")
			return
		}
		if errors.Is(err, ErrCannotDeleteSelf) {
			response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "You cannot delete your own employee record")
			return
		}
		if errors.Is(err, ErrEmployeeNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Employee not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to delete employee", "employee_id", employeeID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to delete employee")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Employee successfully deleted",
	})
}
