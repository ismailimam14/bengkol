package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/bengkol/backend/pkg/response"
)

const (
	HeaderAppName    = "app-name"
	HeaderAppDevice  = "app-device"
	HeaderAppVersion = "app-version"

	HeaderXAppName    = "X-App-Name"
	HeaderXAppDevice  = "X-App-Device"
	HeaderXAppVersion = "X-App-Version"
)

// Supported Application Names
const (
	AppNameBengkol      = "bengkol"
	AppNameBengkolAdmin = "bengkolAdmin"
)

// Supported Application Devices
const (
	AppDeviceAndroid = "android"
	AppDeviceTablet  = "tablet"
	AppDeviceWeb     = "web"

	// Deprecated: AppDeviceMobileApp is kept for backward compatibility alias.
	AppDeviceMobileApp = "android"
)

const (
	AppNameKey    contextKey = "app_name"
	AppDeviceKey  contextKey = "app_device"
	AppVersionKey contextKey = "app_version"
)

// GetAppName retrieves normalized appName from request context.
func GetAppName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(AppNameKey).(string)
	return name, ok
}

// GetAppDevice retrieves normalized appDevice from request context.
func GetAppDevice(ctx context.Context) (string, bool) {
	dev, ok := ctx.Value(AppDeviceKey).(string)
	return dev, ok
}

// GetAppVersion retrieves normalized appVersion from request context.
func GetAppVersion(ctx context.Context) (string, bool) {
	ver, ok := ctx.Value(AppVersionKey).(string)
	return ver, ok
}

// ValidateAppHeaders validates and normalizes appName, appDevice, and appVersion headers.
// If strict is true, missing headers are rejected with 400 Bad Request.
// If strict is false, completely omitted headers fall back to defaults for test compatibility,
// but any provided invalid headers are still rejected.
func ValidateAppHeaders(strict bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawAppName := getHeader(r.Header, HeaderAppName, "appName", HeaderXAppName, "X-App-Name")
			rawAppDevice := getHeader(r.Header, HeaderAppDevice, "appDevice", HeaderXAppDevice, "X-App-Device")
			rawAppVersion := getHeader(r.Header, HeaderAppVersion, "appVersion", HeaderXAppVersion, "X-App-Version")

			// Check if headers are completely omitted
			if rawAppName == "" && rawAppDevice == "" && rawAppVersion == "" {
				if !strict {
					ctx := context.WithValue(r.Context(), AppNameKey, AppNameBengkol)
					ctx = context.WithValue(ctx, AppDeviceKey, AppDeviceAndroid)
					ctx = context.WithValue(ctx, AppVersionKey, "1.0")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			valErrors := make(map[string]string)

			// 1. Validate & Normalize appName
			var normalizedAppName string
			switch strings.TrimSpace(rawAppName) {
			case AppNameBengkol:
				normalizedAppName = AppNameBengkol
			case AppNameBengkolAdmin:
				normalizedAppName = AppNameBengkolAdmin
			default:
				if strings.EqualFold(rawAppName, AppNameBengkol) {
					normalizedAppName = AppNameBengkol
				} else if strings.EqualFold(rawAppName, AppNameBengkolAdmin) {
					normalizedAppName = AppNameBengkolAdmin
				} else {
					valErrors["app-name"] = "app-name header is required and must be either 'bengkol' or 'bengkolAdmin'"
					valErrors["appName"] = valErrors["app-name"]
				}
			}

			// 2. Validate & Normalize appDevice
			var normalizedAppDevice string
			switch strings.ToLower(strings.TrimSpace(rawAppDevice)) {
			case AppDeviceAndroid, "mobile app", "mobile", "mobileapp":
				normalizedAppDevice = AppDeviceAndroid
			case AppDeviceTablet:
				normalizedAppDevice = AppDeviceTablet
			case AppDeviceWeb:
				normalizedAppDevice = AppDeviceWeb
			default:
				valErrors["app-device"] = "app-device header is required and must be either 'android', 'tablet', or 'web'"
				valErrors["appDevice"] = valErrors["app-device"]
			}

			// 3. Validate appVersion
			normalizedAppVersion := strings.TrimSpace(rawAppVersion)
			if normalizedAppVersion == "" {
				valErrors["app-version"] = "app-version header is required (e.g. '1.0', '1.2', '2.0')"
				valErrors["appVersion"] = valErrors["app-version"]
			}

			if len(valErrors) > 0 {
				response.ErrorWithDetails(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid or missing required application headers", valErrors)
				return
			}

			ctx := context.WithValue(r.Context(), AppNameKey, normalizedAppName)
			ctx = context.WithValue(ctx, AppDeviceKey, normalizedAppDevice)
			ctx = context.WithValue(ctx, AppVersionKey, normalizedAppVersion)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getHeader(h http.Header, keys ...string) string {
	for _, key := range keys {
		if val := h.Get(key); strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
		for k, values := range h {
			if strings.EqualFold(k, key) && len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				return strings.TrimSpace(values[0])
			}
		}
	}
	return ""
}
