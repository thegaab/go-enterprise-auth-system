package services

import (
	"context"
	"fmt"
	"time"

	"auth/internal/auth"
	"auth/internal/config"
	"auth/internal/logger"
	"auth/internal/models"
	"auth/internal/repository"
	"github.com/google/uuid"
)

type AuthService struct {
	repo   *repository.Repository
	config *config.Config
	logger *logger.Logger
}

type AuthTokenResponse struct {
	Token     string                 `json:"token"`
	ExpiresAt time.Time              `json:"expires_at"`
	User      *models.UserResponse   `json:"user"`
}

func NewAuthService(repo *repository.Repository, cfg *config.Config, logger *logger.Logger) *AuthService {
	return &AuthService{
		repo:   repo,
		config: cfg,
		logger: logger,
	}
}

func (s *AuthService) SignUp(ctx context.Context, req *models.SignUpRequest) (*models.UserResponse, error) {
	// Validate input
	if err := req.Validate(); err != nil {
		s.logger.Warn("validation failed", "error", err)
		return nil, err
	}

	// Check if user already exists
	existingUser, _ := s.repo.User.GetByUsername(ctx, req.Username)
	if existingUser != nil {
		return nil, fmt.Errorf("user already exists")
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("failed to hash password", "error", err)
		return nil, fmt.Errorf("internal server error")
	}

	// Create user
	user := &models.User{
		ID:       uuid.New().String(),
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
	}

	if err := s.repo.User.Create(ctx, user); err != nil {
		s.logger.Error("failed to create user", "error", err, "username", req.Username)
		return nil, fmt.Errorf("failed to create user")
	}

	s.logger.Info("user created successfully", "user_id", user.ID, "username", user.Username)
	return user.ToResponse(), nil
}

func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*AuthTokenResponse, error) {
	// Validate input
	if err := req.Validate(); err != nil {
		s.logger.Warn("validation failed", "error", err)
		return nil, err
	}

	// Get user
	user, err := s.repo.User.GetByUsername(ctx, req.Username)
	if err != nil {
		s.logger.Warn("user not found", "username", req.Username)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check password
	if !auth.CheckPasswordHash(req.Password, user.Password) {
		s.logger.Warn("invalid password", "username", req.Username)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Generate token
	token, err := auth.GenerateJWT(user.Username, s.config.JWT.Secret, s.config.JWT.Expiration)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err, "user_id", user.ID)
		return nil, fmt.Errorf("internal server error")
	}

	s.logger.Info("user logged in successfully", "user_id", user.ID, "username", user.Username)
	
	return &AuthTokenResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(s.config.JWT.Expiration),
		User:      user.ToResponse(),
	}, nil
}

func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*models.UserResponse, error) {
	user, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		s.logger.Warn("user not found", "user_id", userID)
		return nil, fmt.Errorf("user not found")
	}

	return user.ToResponse(), nil
}