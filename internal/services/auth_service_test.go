package services_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"auth/internal/config"
	"auth/internal/logger"
	"auth/internal/models"
	"auth/internal/repository"
	"auth/internal/services"
)

type mockUserRepository struct {
	users map[string]*models.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users: make(map[string]*models.User),
	}
}

func (m *mockUserRepository) Create(ctx context.Context, user *models.User) error {
	if _, exists := m.users[user.Username]; exists {
		return fmt.Errorf("user already exists")
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.Username] = user
	return nil
}

func (m *mockUserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	if user, exists := m.users[username]; exists {
		return user, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (m *mockUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	for _, user := range m.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

func (m *mockUserRepository) Update(ctx context.Context, user *models.User) error {
	if _, exists := m.users[user.Username]; !exists {
		return fmt.Errorf("user not found")
	}
	user.UpdatedAt = time.Now()
	m.users[user.Username] = user
	return nil
}

func (m *mockUserRepository) Delete(ctx context.Context, id string) error {
	for username, user := range m.users {
		if user.ID == id {
			delete(m.users, username)
			return nil
		}
	}
	return fmt.Errorf("user not found")
}

func setupAuthService() *services.AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			Expiration: time.Hour,
		},
	}
	log := logger.New("error") // Suppress logs during tests
	mockRepo := newMockUserRepository()
	repo := repository.New(mockRepo)
	
	return services.NewAuthService(repo, cfg, log)
}

func TestAuthService_SignUp(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	tests := []struct {
		name    string
		request *models.SignUpRequest
		wantErr bool
	}{
		{
			name: "valid signup",
			request: &models.SignUpRequest{
				Username: "testuser",
				Email:    "test@example.com",
				Password: "password123",
			},
			wantErr: false,
		},
		{
			name: "invalid email",
			request: &models.SignUpRequest{
				Username: "testuser2",
				Email:    "invalid-email",
				Password: "password123",
			},
			wantErr: true,
		},
		{
			name: "short password",
			request: &models.SignUpRequest{
				Username: "testuser3",
				Email:    "test3@example.com",
				Password: "123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := authService.SignUp(ctx, tt.request)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("SignUp() expected error, got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("SignUp() unexpected error: %v", err)
				return
			}
			
			if user.Username != tt.request.Username {
				t.Errorf("SignUp() username = %v, want %v", user.Username, tt.request.Username)
			}
			
			if user.Email != tt.request.Email {
				t.Errorf("SignUp() email = %v, want %v", user.Email, tt.request.Email)
			}
		})
	}
}

func TestAuthService_Login(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	// First create a user
	signupReq := &models.SignUpRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}
	_, err := authService.SignUp(ctx, signupReq)
	if err != nil {
		t.Fatalf("Failed to create user for login test: %v", err)
	}

	tests := []struct {
		name    string
		request *models.LoginRequest
		wantErr bool
	}{
		{
			name: "valid login",
			request: &models.LoginRequest{
				Username: "testuser",
				Password: "password123",
			},
			wantErr: false,
		},
		{
			name: "invalid password",
			request: &models.LoginRequest{
				Username: "testuser",
				Password: "wrongpassword",
			},
			wantErr: true,
		},
		{
			name: "nonexistent user",
			request: &models.LoginRequest{
				Username: "nonexistent",
				Password: "password123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := authService.Login(ctx, tt.request)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Login() expected error, got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Login() unexpected error: %v", err)
				return
			}
			
			if response.Token == "" {
				t.Errorf("Login() token is empty")
			}
			
			if response.User.Username != tt.request.Username {
				t.Errorf("Login() username = %v, want %v", response.User.Username, tt.request.Username)
			}
		})
	}
}