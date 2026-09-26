package middleware

import (
	"context"
	"net/http"

	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

// RequestIDHeader is the HTTP header key for request tracking.
const RequestIDHeader = "X-Request-ID"

// RequestID middleware extracts or generates a UUID request ID and attaches it to request context and response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(RequestIDHeader)
		if reqID == "" {
			reqID = uuid.New().String()
		}

		w.Header().Set(RequestIDHeader, reqID)
		ctx := context.WithValue(r.Context(), logger.RequestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
