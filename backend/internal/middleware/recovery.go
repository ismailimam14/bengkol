package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
)

// Recoverer catches panics and returns standard 500 error response.
func Recoverer(log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rvr := recover(); rvr != nil {
					if rvr == http.ErrAbortHandler {
						panic(rvr)
					}

					stack := string(debug.Stack())
					log.WithContext(r.Context()).Error("panic_recovered",
						"error", fmt.Sprintf("%v", rvr),
						"stack", stack,
					)

					response.Error(w, http.StatusInternalServerError,
						response.ErrCodeInternalServerError,
						"An unexpected server error occurred",
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
