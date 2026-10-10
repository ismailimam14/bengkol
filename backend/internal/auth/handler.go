package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles authentication HTTP requests.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Auth Handler instance.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// Routes returns the sub-router for /auth endpoints.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.RefreshToken)
	r.Post("/logout", h.Logout)
	r.With(middleware.RequireAuthenticated).Post("/get-permissions", h.GetPermissions)
	r.With(middleware.RequireAuthenticated).Post("/select-workshop", h.GetPermissions)
	r.With(middleware.RequireAuthenticated).Get("/me", h.GetMe)
	r.With(middleware.RequireAuthenticated).Put("/password", h.ChangePassword)
	r.With(middleware.RequireAuthenticated).Post("/change-password", h.ChangePassword)

	return r
}

// Register creates a new customer or workshop owner account.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	authResp, validationErrors, err := h.service.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", validationErrors)
			return
		}
		if errors.Is(err, ErrUserAlreadyExists) {
			msg := "Email or phone number is already in use"
			if _, emailExists := validationErrors["email"]; emailExists && len(validationErrors) == 1 {
				msg = "Email is already in use"
			} else if _, phoneExists := validationErrors["phone"]; phoneExists && len(validationErrors) == 1 {
				msg = "Phone number is already in use"
			}
			response.ErrorWithDetails(w, http.StatusConflict, response.ErrCodeConflict, msg, validationErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to register user", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to complete registration")
		return
	}

	response.Success(w, http.StatusCreated, authResp)
}

// Login authenticates a user and returns a token pair.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	authResp, err := h.service.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Invalid credentials")
			return
		}
		if errors.Is(err, ErrNoWorkshopAccess) {
			response.Error(w, http.StatusForbidden, response.ErrCodeNoWorkshopAccess, "User has no associated workshops")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to login user", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Login request failed")
		return
	}

	response.Success(w, http.StatusOK, authResp)
}

// GetPermissions selects a workshop and returns a workshop-scoped token pair and permissions.
func (h *Handler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req GetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.WorkshopID == uuid.Nil {
		response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", map[string]string{
			"workshop_id": "workshop_id is required",
		})
		return
	}

	resp, err := h.service.GetPermissions(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrNoWorkshopAccess) || errors.Is(err, ErrEmployeeInactive) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have active access to this workshop")
			return
		}
		if errors.Is(err, ErrWorkshopNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Workshop not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get workshop permissions", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to get workshop permissions")
		return
	}

	response.Success(w, http.StatusOK, resp)
}

// RefreshToken exchanges an active refresh token for a new token pair (rotation).
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	authResp, err := h.service.RefreshToken(r.Context(), req)
	if err != nil {
		if errors.Is(err, security.ErrInvalidToken) || errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrTokenRevoked) {
			response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Invalid or expired refresh token")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to refresh token", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Token refresh failed")
		return
	}

	response.Success(w, http.StatusOK, authResp)
}

// Logout revokes the provided refresh token.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.service.Logout(r.Context(), req.RefreshToken); err != nil {
		h.logger.WithContext(r.Context()).Error("failed to logout user", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to logout")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Successfully logged out",
	})
}

// GetMe returns the profile of the authenticated user.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	user, err := h.service.GetMe(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "User not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get current user", "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to fetch profile")
		return
	}

	response.Success(w, http.StatusOK, user)
}

// ChangePassword updates the authenticated user's password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	valErrors, err := h.service.ChangePassword(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		if errors.Is(err, ErrUserNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "User not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to change password", "user_id", userID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to change password")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Password changed successfully",
	})
}

