package response

import (
	"encoding/json"
	"net/http"
)

// Response represents the standard API response structure.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// APIError represents structured error details.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Meta contains metadata such as pagination information.
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PageSize   int   `json:"page_size,omitempty"`
	TotalItems int64 `json:"total_items,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
}

// JSON sends a JSON response with status code.
func JSON(w http.ResponseWriter, statusCode int, payload Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

// Success sends a standard 200 OK or specified success status code response.
func Success(w http.ResponseWriter, statusCode int, data interface{}) {
	JSON(w, statusCode, Response{
		Success: true,
		Data:    data,
	})
}

// SuccessWithMeta sends a standard success response with metadata (e.g. pagination).
func SuccessWithMeta(w http.ResponseWriter, statusCode int, data interface{}, meta *Meta) {
	JSON(w, statusCode, Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Error sends a standardized error response.
func Error(w http.ResponseWriter, statusCode int, code string, message string) {
	JSON(w, statusCode, Response{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}

// ErrorWithDetails sends a standardized error response including field validation details.
func ErrorWithDetails(w http.ResponseWriter, statusCode int, code string, message string, details map[string]string) {
	JSON(w, statusCode, Response{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// Common error codes
const (
	ErrCodeInternalServerError = "INTERNAL_SERVER_ERROR"
	ErrCodeNotFound            = "RESOURCE_NOT_FOUND"
	ErrCodeBadRequest          = "BAD_REQUEST"
	ErrCodeUnauthorized        = "UNAUTHORIZED"
	ErrCodeForbidden           = "FORBIDDEN"
	ErrCodeConflict            = "CONFLICT"
	ErrCodeValidationFailed    = "VALIDATION_FAILED"
	ErrCodeDatabaseUnavailable = "DATABASE_UNAVAILABLE"
	ErrCodeNoWorkshopAccess    = "NO_WORKSHOP_ACCESS"
)
