package middleware

import (
	"net/http"
	"time"

	"github.com/bengkol/backend/pkg/logger"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// RequestLogger logs incoming HTTP requests with latency, status, path, and method.
func RequestLogger(log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			ctxLog := log.WithContext(r.Context())

			ctxLog.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_ip", r.RemoteAddr,
				"bytes", rw.bytesWritten,
			)
		})
	}
}
