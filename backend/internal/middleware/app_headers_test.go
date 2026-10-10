package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/response"
)

func TestValidateAppHeaders_Strict_Valid(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		wantName    string
		wantDevice  string
		wantVersion string
	}{
		{
			name: "bengkol customer app on mobile app",
			headers: map[string]string{
				"appName":    "bengkol",
				"appDevice":  "mobile app",
				"appVersion": "1.0",
			},
			wantName:    "bengkol",
			wantDevice:  "mobile app",
			wantVersion: "1.0",
		},
		{
			name: "bengkolAdmin app on web",
			headers: map[string]string{
				"appName":    "bengkolAdmin",
				"appDevice":  "web",
				"appVersion": "2.0.1",
			},
			wantName:    "bengkolAdmin",
			wantDevice:  "web",
			wantVersion: "2.0.1",
		},
		{
			name: "headers with X- prefix and case insensitivity",
			headers: map[string]string{
				"X-App-Name":    "bengkoladmin",
				"X-App-Device":  "WEB",
				"X-App-Version": "1.2",
			},
			wantName:    "bengkolAdmin",
			wantDevice:  "web",
			wantVersion: "1.2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handlerCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				appName, ok := middleware.GetAppName(r.Context())
				if !ok || appName != tc.wantName {
					t.Errorf("GetAppName() = %q, %v; want %q, true", appName, ok, tc.wantName)
				}
				appDevice, ok := middleware.GetAppDevice(r.Context())
				if !ok || appDevice != tc.wantDevice {
					t.Errorf("GetAppDevice() = %q, %v; want %q, true", appDevice, ok, tc.wantDevice)
				}
				appVersion, ok := middleware.GetAppVersion(r.Context())
				if !ok || appVersion != tc.wantVersion {
					t.Errorf("GetAppVersion() = %q, %v; want %q, true", appVersion, ok, tc.wantVersion)
				}
				w.WriteHeader(http.StatusOK)
			})

			mw := middleware.ValidateAppHeaders(true)(next)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			mw.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if !handlerCalled {
				t.Fatal("expected next handler to be called")
			}
		})
	}
}

func TestValidateAppHeaders_Strict_Rejections(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		wantFields []string
	}{
		{
			name:       "missing all headers",
			headers:    map[string]string{},
			wantFields: []string{"appName", "appDevice", "appVersion"},
		},
		{
			name: "invalid appName",
			headers: map[string]string{
				"appName":    "unknownApp",
				"appDevice":  "web",
				"appVersion": "1.0",
			},
			wantFields: []string{"appName"},
		},
		{
			name: "invalid appDevice",
			headers: map[string]string{
				"appName":    "bengkol",
				"appDevice":  "smartTv",
				"appVersion": "1.0",
			},
			wantFields: []string{"appDevice"},
		},
		{
			name: "empty appVersion",
			headers: map[string]string{
				"appName":    "bengkol",
				"appDevice":  "web",
				"appVersion": "  ",
			},
			wantFields: []string{"appVersion"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("next handler should not have been called on invalid headers")
			})

			mw := middleware.ValidateAppHeaders(true)(next)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			mw.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d", rec.Code)
			}

			var resp response.Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}

			if resp.Success {
				t.Fatal("expected response.Success to be false")
			}
			if resp.Error == nil || resp.Error.Code != response.ErrCodeBadRequest {
				t.Fatalf("expected error code BAD_REQUEST, got %+v", resp.Error)
			}

			for _, f := range tc.wantFields {
				if _, ok := resp.Error.Details[f]; !ok {
					t.Errorf("expected details to contain error for field %q, got %+v", f, resp.Error.Details)
				}
			}
		})
	}
}

func TestValidateAppHeaders_NonStrict_Fallback(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		appName, _ := middleware.GetAppName(r.Context())
		if appName != "bengkol" {
			t.Errorf("expected default appName 'bengkol', got %q", appName)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.ValidateAppHeaders(false)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !handlerCalled {
		t.Fatal("expected next handler to be called in non-strict mode")
	}
}
