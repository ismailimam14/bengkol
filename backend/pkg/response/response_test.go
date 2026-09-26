package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/pkg/response"
)

func TestSuccessResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	testData := map[string]string{"message": "hello world"}

	response.Success(rec, http.StatusOK, testData)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true, got %v", resp.Success)
	}

	if resp.Error != nil {
		t.Errorf("expected error to be nil, got %+v", resp.Error)
	}
}

func TestErrorResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	response.Error(rec, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid input data")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Success {
		t.Errorf("expected success to be false, got %v", resp.Success)
	}

	if resp.Error == nil {
		t.Fatalf("expected error object, got nil")
	}

	if resp.Error.Code != response.ErrCodeBadRequest {
		t.Errorf("expected code %s, got %s", response.ErrCodeBadRequest, resp.Error.Code)
	}

	if resp.Error.Message != "Invalid input data" {
		t.Errorf("expected message 'Invalid input data', got %s", resp.Error.Message)
	}
}

func TestErrorWithDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	details := map[string]string{"email": "invalid email format"}
	response.ErrorWithDetails(rec, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Validation error", details)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, rec.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error.Details["email"] != "invalid email format" {
		t.Errorf("expected field error on email, got %v", resp.Error.Details)
	}
}
