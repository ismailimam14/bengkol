package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRole        = errors.New("invalid role; must be CUSTOMER or OWNER")
	ErrValidationFailed   = errors.New("validation failed")
	ErrTokenExpired       = errors.New("refresh token has expired")
	ErrTokenRevoked       = errors.New("refresh token has been revoked")
	ErrNoWorkshopAccess   = errors.New("no workshop access")
	ErrForbidden          = errors.New("forbidden")
	ErrEmployeeInactive   = errors.New("employee membership is inactive")
	ErrWorkshopNotFound   = errors.New("workshop not found")
)

// RegisterRequest DTO
type RegisterRequest struct {
	Name        string          `json:"name"`
	Email       string          `json:"email,omitempty"`
	Password    string          `json:"password"`
	Phone       string          `json:"phone,omitempty"`
	PhoneNumber string          `json:"phone_number,omitempty"`
	Role        domain.UserRole `json:"role"`
}

// LoginRequest DTO
type LoginRequest struct {
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Password    string `json:"password"`
}

// RefreshTokenRequest DTO
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// WorkshopSummary represents workshop membership summary
type WorkshopSummary struct {
	ID      uuid.UUID       `json:"id"`
	Name    string          `json:"name"`
	Address string          `json:"address,omitempty"`
	Phone   string          `json:"phone,omitempty"`
	Role    domain.UserRole `json:"role,omitempty"`
	Status  string          `json:"status,omitempty"`
}

// AuthResponse returns authenticated user, active token pair, and workshop context
type AuthResponse struct {
	User                      *domain.User                `json:"user"`
	Tokens                    *security.TokenPair         `json:"tokens"`
	Workshops                 []WorkshopSummary           `json:"workshops,omitempty"`
	SelectedWorkshopID        *uuid.UUID                  `json:"selected_workshop_id,omitempty"`
	Role                      domain.UserRole             `json:"role,omitempty"`
	Permissions               *domain.EmployeePermissions `json:"permissions,omitempty"`
	RequiresWorkshopSelection bool                        `json:"requires_workshop_selection,omitempty"`
}

// GetPermissionsRequest DTO
type GetPermissionsRequest struct {
	WorkshopID uuid.UUID `json:"workshop_id"`
}

// GetPermissionsResponse DTO
type GetPermissionsResponse struct {
	WorkshopID  uuid.UUID                   `json:"workshop_id"`
	Role        domain.UserRole             `json:"role"`
	Permissions *domain.EmployeePermissions `json:"permissions"`
	Tokens      *security.TokenPair         `json:"tokens"`
	Workshop    *domain.Workshop            `json:"workshop,omitempty"`
}

// UserDetailResponse DTO for /me (userDetail)
type UserDetailResponse struct {
	ID                 uuid.UUID                   `json:"id"`
	Email              string                      `json:"email"`
	Name               string                      `json:"name"`
	Phone              string                      `json:"phone"`
	Role               domain.UserRole             `json:"role"`
	Workshops          []WorkshopSummary           `json:"workshops,omitempty"`
	SelectedWorkshopID *uuid.UUID                  `json:"selected_workshop_id,omitempty"`
	Permissions        *domain.EmployeePermissions `json:"permissions,omitempty"`
	CreatedAt          time.Time                   `json:"created_at"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

// ChangePasswordRequest DTO
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password,omitempty"`
}

// Service defines auth business logic methods.
type Service interface {
	Register(ctx context.Context, req RegisterRequest) (*AuthResponse, map[string]string, error)
	Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
	RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	GetMe(ctx context.Context, userID uuid.UUID) (*UserDetailResponse, error)
	GetPermissions(ctx context.Context, userID uuid.UUID, req GetPermissionsRequest) (*GetPermissionsResponse, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) (map[string]string, error)
}

type authService struct {
	repo   Repository
	jwt    *security.JWTManager
	logger *logger.Logger
}

// NewService creates a new Auth Service instance.
func NewService(repo Repository, jwtMgr *security.JWTManager, log *logger.Logger) Service {
	return &authService{
		repo:   repo,
		jwt:    jwtMgr,
		logger: log,
	}
}

func (s *authService) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, map[string]string, error) {
	if req.Phone == "" && req.PhoneNumber != "" {
		req.Phone = req.PhoneNumber
	}

	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)

	v.Required("name", req.Name)
	if req.Email != "" {
		v.Email("email", req.Email)
	}
	v.Required("password", req.Password)
	v.MinLength("password", req.Password, 8)
	v.Required("phone", req.Phone)

	if req.Role == "" {
		req.Role = domain.RoleCustomer
	}

	if req.Role != domain.RoleCustomer && req.Role != domain.RoleOwner {
		v.AddError("role", "role must be either CUSTOMER or OWNER")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	hasDuplicate := false

	// Check if user phone already exists
	existingPhone, err := s.repo.GetUserByPhone(ctx, req.Phone)
	if err == nil && existingPhone != nil {
		v.AddError("phone", "phone number is already registered")
		hasDuplicate = true
	} else if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, nil, err
	}

	// Check if user email already exists (if email is provided)
	if req.Email != "" {
		existingEmail, err := s.repo.GetUserByEmail(ctx, req.Email)
		if err == nil && existingEmail != nil {
			v.AddError("email", "email is already registered")
			hasDuplicate = true
		} else if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, nil, err
		}
	}

	if hasDuplicate {
		return nil, v.Errors, ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	user := &domain.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		Phone:        req.Phone,
		Role:         req.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, nil, err
	}

	// Generate tokens
	tokens, tokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, nil, err
	}

	// Store refresh token
	rfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
		return nil, nil, err
	}

	s.logger.Info("user registered successfully", "user_id", user.ID, "role", user.Role)

	return &AuthResponse{
		User:   user,
		Tokens: tokens,
	}, nil, nil
}

func (s *authService) resolveMembership(ctx context.Context, ws domain.Workshop, userID uuid.UUID) (domain.UserRole, *domain.EmployeePermissions, error) {
	if ws.OwnerID == userID {
		perms := domain.DefaultPermissionsForRole(domain.EmployeeRoleOwner)
		return domain.RoleOwner, &perms, nil
	}

	emp, err := s.repo.GetWorkshopEmployeeMembership(ctx, ws.ID, userID)
	if err != nil {
		return "", nil, err
	}
	if emp == nil {
		return "", nil, ErrForbidden
	}
	if emp.Status != domain.EmployeeStatusActive {
		return "", nil, ErrEmployeeInactive
	}

	role := emp.Role.ToUserRole()
	perms := emp.CalculatePermissions()
	return role, &perms, nil
}

func (s *authService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	if req.Phone == "" && req.PhoneNumber != "" {
		req.Phone = req.PhoneNumber
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)

	if (req.Email == "" && req.Phone == "") || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	var user *domain.User
	var err error

	if req.Email != "" && req.Phone != "" {
		user, err = s.repo.GetUserByEmail(ctx, req.Email)
		if err != nil || user.Phone != req.Phone {
			return nil, ErrInvalidCredentials
		}
	} else if req.Email != "" {
		user, err = s.repo.GetUserByEmail(ctx, req.Email)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	} else {
		user, err = s.repo.GetUserByPhone(ctx, req.Phone)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	if !security.CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	now := time.Now().UTC()
	appName, _ := middleware.GetAppName(ctx)

	// App Name: bengkol (Customer App)
	if appName == middleware.AppNameBengkol {
		userCopy := *user
		userCopy.Role = domain.RoleCustomer

		tokens, tokenHash, err := s.jwt.GenerateTokenPair(&userCopy)
		if err != nil {
			return nil, err
		}

		rfToken := &domain.RefreshToken{
			ID:        uuid.New(),
			UserID:    user.ID,
			TokenHash: tokenHash,
			Revoked:   false,
			ExpiresAt: tokens.RefreshExpiresAt,
			CreatedAt: now,
		}
		if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
			return nil, err
		}

		s.logger.Info("user logged in via customer app", "user_id", user.ID, "role", userCopy.Role)
		return &AuthResponse{
			User:                      &userCopy,
			Tokens:                    tokens,
			Role:                      domain.RoleCustomer,
			Workshops:                 nil,
			SelectedWorkshopID:        nil,
			Permissions:               nil,
			RequiresWorkshopSelection: false,
		}, nil
	}

	// App Name: bengkolAdmin (Admin / Workshop App)
	if appName == middleware.AppNameBengkolAdmin {
		workshops, err := s.repo.GetWorkshopsByUserID(ctx, user.ID)
		if err != nil {
			return nil, err
		}

		if len(workshops) == 0 {
			s.logger.Warn("user login rejected: no workshop access", "user_id", user.ID)
			return nil, ErrNoWorkshopAccess
		}

		summaries := make([]WorkshopSummary, 0, len(workshops))
		for _, ws := range workshops {
			wsRole := domain.RoleCustomer
			if ws.OwnerID == user.ID {
				wsRole = domain.RoleOwner
			} else {
				emp, _ := s.repo.GetWorkshopEmployeeMembership(ctx, ws.ID, user.ID)
				if emp != nil {
					wsRole = emp.Role.ToUserRole()
				}
			}
			summaries = append(summaries, WorkshopSummary{
				ID:      ws.ID,
				Name:    ws.Name,
				Address: ws.Address,
				Phone:   ws.Phone,
				Role:    wsRole,
				Status:  string(ws.Status),
			})
		}

		// Single workshop: auto-select and issue workshop-scoped token pair
		if len(workshops) == 1 {
			ws := workshops[0]
			role, perms, err := s.resolveMembership(ctx, ws, user.ID)
			if err != nil {
				return nil, err
			}

			userCopy := *user
			userCopy.Role = role

			tokens, tokenHash, err := s.jwt.GenerateScopedTokenPair(&userCopy, &ws.ID, role, perms)
			if err != nil {
				return nil, err
			}

			rfToken := &domain.RefreshToken{
				ID:        uuid.New(),
				UserID:    user.ID,
				TokenHash: tokenHash,
				Revoked:   false,
				ExpiresAt: tokens.RefreshExpiresAt,
				CreatedAt: now,
			}
			if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
				return nil, err
			}

			s.logger.Info("user logged in via admin app (single workshop auto-selected)", "user_id", user.ID, "workshop_id", ws.ID, "role", role)
			return &AuthResponse{
				User:                      &userCopy,
				Tokens:                    tokens,
				Workshops:                 summaries,
				SelectedWorkshopID:        &ws.ID,
				Role:                      role,
				Permissions:               perms,
				RequiresWorkshopSelection: false,
			}, nil
		}

		// Multiple workshops: requires selection
		tokens, tokenHash, err := s.jwt.GenerateTokenPair(user)
		if err != nil {
			return nil, err
		}

		rfToken := &domain.RefreshToken{
			ID:        uuid.New(),
			UserID:    user.ID,
			TokenHash: tokenHash,
			Revoked:   false,
			ExpiresAt: tokens.RefreshExpiresAt,
			CreatedAt: now,
		}
		if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
			return nil, err
		}

		s.logger.Info("user logged in via admin app (multiple workshops, requires selection)", "user_id", user.ID, "workshops_count", len(workshops))
		return &AuthResponse{
			User:                      user,
			Tokens:                    tokens,
			Workshops:                 summaries,
			SelectedWorkshopID:        nil,
			Role:                      "",
			Permissions:               nil,
			RequiresWorkshopSelection: true,
		}, nil
	}

	// Fallback / legacy login (when appName is unspecified or in test environments)
	tokens, tokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	rfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
		return nil, err
	}

	s.logger.Info("user logged in", "user_id", user.ID, "role", user.Role)

	return &AuthResponse{
		User:                      user,
		Tokens:                    tokens,
		Role:                      user.Role,
		RequiresWorkshopSelection: false,
	}, nil
}

func (s *authService) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthResponse, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return nil, security.ErrInvalidToken
	}

	tokenHash := security.HashToken(req.RefreshToken)
	storedToken, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, security.ErrInvalidToken
	}

	if storedToken.Revoked {
		// Possible token reuse attack; revoke all tokens for this user for security
		_ = s.repo.RevokeAllUserRefreshTokens(ctx, storedToken.UserID)
		s.logger.Warn("detected reuse of revoked refresh token; revoked all tokens", "user_id", storedToken.UserID)
		return nil, ErrTokenRevoked
	}

	if time.Now().UTC().After(storedToken.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Rotate refresh token by revoking old one
	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	now := time.Now().UTC()
	appName, _ := middleware.GetAppName(ctx)

	if appName == middleware.AppNameBengkol {
		userCopy := *user
		userCopy.Role = domain.RoleCustomer

		tokens, newTokenHash, err := s.jwt.GenerateTokenPair(&userCopy)
		if err != nil {
			return nil, err
		}

		newRfToken := &domain.RefreshToken{
			ID:        uuid.New(),
			UserID:    user.ID,
			TokenHash: newTokenHash,
			Revoked:   false,
			ExpiresAt: tokens.RefreshExpiresAt,
			CreatedAt: now,
		}
		if err := s.repo.SaveRefreshToken(ctx, newRfToken); err != nil {
			return nil, err
		}

		return &AuthResponse{
			User:   &userCopy,
			Tokens: tokens,
			Role:   domain.RoleCustomer,
		}, nil
	}

	if appName == middleware.AppNameBengkolAdmin {
		workshops, err := s.repo.GetWorkshopsByUserID(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		if len(workshops) == 0 {
			return nil, ErrNoWorkshopAccess
		}

		if len(workshops) == 1 {
			ws := workshops[0]
			role, perms, err := s.resolveMembership(ctx, ws, user.ID)
			if err != nil {
				return nil, err
			}

			userCopy := *user
			userCopy.Role = role

			tokens, newTokenHash, err := s.jwt.GenerateScopedTokenPair(&userCopy, &ws.ID, role, perms)
			if err != nil {
				return nil, err
			}

			newRfToken := &domain.RefreshToken{
				ID:        uuid.New(),
				UserID:    user.ID,
				TokenHash: newTokenHash,
				Revoked:   false,
				ExpiresAt: tokens.RefreshExpiresAt,
				CreatedAt: now,
			}
			if err := s.repo.SaveRefreshToken(ctx, newRfToken); err != nil {
				return nil, err
			}

			return &AuthResponse{
				User:                      &userCopy,
				Tokens:                    tokens,
				SelectedWorkshopID:        &ws.ID,
				Role:                      role,
				Permissions:               perms,
				RequiresWorkshopSelection: false,
			}, nil
		}
	}

	// Issue standard token pair
	tokens, newTokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	newRfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: newTokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, newRfToken); err != nil {
		return nil, err
	}

	return &AuthResponse{
		User:   user,
		Tokens: tokens,
		Role:   user.Role,
	}, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	tokenHash := security.HashToken(refreshToken)
	return s.repo.RevokeRefreshToken(ctx, tokenHash)
}

func (s *authService) GetPermissions(ctx context.Context, userID uuid.UUID, req GetPermissionsRequest) (*GetPermissionsResponse, error) {
	if req.WorkshopID == uuid.Nil {
		return nil, ErrValidationFailed
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	ws, err := s.repo.GetWorkshopByID(ctx, req.WorkshopID)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, ErrWorkshopNotFound
	}

	role, perms, err := s.resolveMembership(ctx, *ws, userID)
	if err != nil {
		return nil, err
	}

	userCopy := *user
	userCopy.Role = role

	tokens, tokenHash, err := s.jwt.GenerateScopedTokenPair(&userCopy, &ws.ID, role, perms)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
		return nil, err
	}

	s.logger.Info("issued workshop-scoped permissions and token", "user_id", userID, "workshop_id", req.WorkshopID, "role", role)

	return &GetPermissionsResponse{
		WorkshopID:  req.WorkshopID,
		Role:        role,
		Permissions: perms,
		Tokens:      tokens,
		Workshop:    ws,
	}, nil
}

func (s *authService) GetMe(ctx context.Context, userID uuid.UUID) (*UserDetailResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	appName, _ := middleware.GetAppName(ctx)
	if appName == middleware.AppNameBengkol {
		return &UserDetailResponse{
			ID:                 user.ID,
			Email:              user.Email,
			Name:               user.Name,
			Phone:              user.Phone,
			Role:               domain.RoleCustomer,
			Workshops:          nil,
			SelectedWorkshopID: nil,
			Permissions:        nil,
			CreatedAt:          user.CreatedAt,
			UpdatedAt:          user.UpdatedAt,
		}, nil
	}

	workshops, err := s.repo.GetWorkshopsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var summaries []WorkshopSummary
	if len(workshops) > 0 {
		summaries = make([]WorkshopSummary, 0, len(workshops))
		for _, ws := range workshops {
			wsRole := domain.RoleCustomer
			if ws.OwnerID == userID {
				wsRole = domain.RoleOwner
			} else {
				emp, _ := s.repo.GetWorkshopEmployeeMembership(ctx, ws.ID, userID)
				if emp != nil {
					wsRole = emp.Role.ToUserRole()
				}
			}
			summaries = append(summaries, WorkshopSummary{
				ID:      ws.ID,
				Name:    ws.Name,
				Address: ws.Address,
				Phone:   ws.Phone,
				Role:    wsRole,
				Status:  string(ws.Status),
			})
		}
	}

	selectedWSID, hasWS := middleware.GetWorkshopID(ctx)
	var selectedWorkshopID *uuid.UUID
	var perms *domain.EmployeePermissions
	userRole := user.Role

	if hasWS && selectedWSID != uuid.Nil {
		selectedWorkshopID = &selectedWSID
		ctxPerms, hasPerms := middleware.GetPermissions(ctx)
		if hasPerms && ctxPerms != nil {
			perms = ctxPerms
		}
		if ctxRole, hasRole := middleware.GetUserRole(ctx); hasRole {
			userRole = ctxRole
		}
		if perms == nil {
			ws, _ := s.repo.GetWorkshopByID(ctx, selectedWSID)
			if ws != nil {
				resolvedRole, resolvedPerms, err := s.resolveMembership(ctx, *ws, userID)
				if err == nil {
					userRole = resolvedRole
					perms = resolvedPerms
				}
			}
		}
	} else if len(workshops) == 1 && appName == middleware.AppNameBengkolAdmin {
		ws := workshops[0]
		resolvedRole, resolvedPerms, err := s.resolveMembership(ctx, ws, userID)
		if err == nil {
			selectedWorkshopID = &ws.ID
			userRole = resolvedRole
			perms = resolvedPerms
		}
	}

	return &UserDetailResponse{
		ID:                 user.ID,
		Email:              user.Email,
		Name:               user.Name,
		Phone:              user.Phone,
		Role:               userRole,
		Workshops:          summaries,
		SelectedWorkshopID: selectedWorkshopID,
		Permissions:        perms,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}, nil
}

func (s *authService) ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) (map[string]string, error) {
	v := validator.New()
	req.CurrentPassword = strings.TrimSpace(req.CurrentPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	req.ConfirmPassword = strings.TrimSpace(req.ConfirmPassword)

	v.Required("current_password", req.CurrentPassword)
	v.Required("new_password", req.NewPassword)
	v.MinLength("new_password", req.NewPassword, 8)

	if req.ConfirmPassword != "" && req.ConfirmPassword != req.NewPassword {
		v.AddError("confirm_password", "passwords do not match")
	}
	if req.CurrentPassword != "" && req.NewPassword != "" && req.CurrentPassword == req.NewPassword {
		v.AddError("new_password", "new password cannot be the same as current password")
	}

	if !v.IsValid() {
		return v.Errors, ErrValidationFailed
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if !security.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		v.AddError("current_password", "incorrect current password")
		return v.Errors, ErrValidationFailed
	}

	newHash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpdatePassword(ctx, userID, newHash); err != nil {
		return nil, err
	}

	// Revoke all existing refresh tokens for security on password change
	_ = s.repo.RevokeAllUserRefreshTokens(ctx, userID)

	s.logger.Info("password updated successfully", "user_id", userID)
	return nil, nil
}

