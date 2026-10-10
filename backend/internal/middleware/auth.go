package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
)

type contextKey string

const (
	ClaimsKey      contextKey = "user_claims"
	UserIDKey      contextKey = "user_id"
	UserRoleKey    contextKey = "user_role"
	UserEmailKey   contextKey = "user_email"
	WorkshopIDKey  contextKey = "workshop_id"
	PermissionsKey contextKey = "user_permissions"
)

// Authenticate middleware validates the Bearer JWT access token and enriches request context.
func Authenticate(jwtMgr *security.JWTManager, log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Invalid authorization header format; expected 'Bearer <token>'")
				return
			}

			tokenString := parts[1]
			claims, err := jwtMgr.ValidateAccessToken(tokenString)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Invalid or expired access token")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UserRoleKey, claims.Role)
			ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
			if claims.WorkshopID != nil {
				ctx = context.WithValue(ctx, WorkshopIDKey, *claims.WorkshopID)
			}
			if claims.Permissions != nil {
				ctx = context.WithValue(ctx, PermissionsKey, claims.Permissions)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthenticated blocks requests that have not been authenticated.
func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		if !ok || claims == nil {
			response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required to access this resource")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRoles restricts endpoint access to specified roles.
func RequireRoles(allowedRoles ...domain.UserRole) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := GetUserRole(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
				return
			}

			// ADMIN always has universal access
			if role == domain.RoleAdmin {
				next.ServeHTTP(w, r)
				return
			}

			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to perform this action")
		})
	}
}

// RequireWorkshopContext ensures the request has an active workshop-scoped token.
func RequireWorkshopContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workshopID, ok := GetWorkshopID(r.Context())
		if !ok || workshopID == uuid.Nil {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "A selected workshop context is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission checks if the authenticated user has a specific permission key.
func RequirePermission(permKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := GetUserRole(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
				return
			}

			// Workshop OWNER always has universal access across all workshop permissions
			if role == domain.RoleOwner {
				next.ServeHTTP(w, r)
				return
			}

			perms, hasPerms := GetPermissions(r.Context())
			if hasPerms && perms != nil {
				if perms.HasPermission(permKey) {
					next.ServeHTTP(w, r)
					return
				}
				response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to perform this action")
				return
			}

			defPerms := domain.DefaultPermissionsForRole(domain.EmployeeRole(role))
			if defPerms.HasPermission(permKey) {
				next.ServeHTTP(w, r)
				return
			}

			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to perform this action")
		})
	}
}

// GetClaims retrieves CustomClaims from request context.
func GetClaims(ctx context.Context) (*security.CustomClaims, bool) {
	claims, ok := ctx.Value(ClaimsKey).(*security.CustomClaims)
	return claims, ok
}

// GetUserID retrieves UserID from context.
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return id, ok
}

// GetUserRole retrieves UserRole from context.
func GetUserRole(ctx context.Context) (domain.UserRole, bool) {
	role, ok := ctx.Value(UserRoleKey).(domain.UserRole)
	return role, ok
}

// GetWorkshopID retrieves WorkshopID from context.
func GetWorkshopID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(WorkshopIDKey).(uuid.UUID)
	return id, ok
}

// GetPermissions retrieves EmployeePermissions from context.
func GetPermissions(ctx context.Context) (*domain.EmployeePermissions, bool) {
	p, ok := ctx.Value(PermissionsKey).(*domain.EmployeePermissions)
	return p, ok
}
