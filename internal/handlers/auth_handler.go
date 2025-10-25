package handlers

import (
	"encoding/json"
	"net/http"

	"auth/internal/middleware"
	"auth/internal/models"
	"auth/internal/services"
	"auth/internal/logger"
)

type AuthHandler struct {
	authService *services.AuthService
	logger      *logger.Logger
}

func NewAuthHandler(authService *services.AuthService, logger *logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		logger:      logger,
	}
}

// SignUp handles user registration
// @Summary Register a new user
// @Description Register a new user with username, email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param user body models.SignUpRequest true "User registration data"
// @Success 201 {object} models.UserResponse
// @Failure 400 {object} models.APIError
// @Failure 409 {object} models.APIError
// @Failure 500 {object} models.APIError
// @Router /signup [post]
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req models.SignUpRequest
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, "Invalid JSON format", "INVALID_JSON", http.StatusBadRequest, nil)
		return
	}

	user, err := h.authService.SignUp(r.Context(), &req)
	if err != nil {
		if validationErr, ok := err.(models.ValidationErrors); ok {
			h.writeErrorResponse(w, "Validation failed", "VALIDATION_ERROR", http.StatusBadRequest, map[string]string(validationErr))
			return
		}
		
		if err.Error() == "user already exists" {
			h.writeErrorResponse(w, "User already exists", "USER_EXISTS", http.StatusConflict, nil)
			return
		}
		
		h.logger.Error("signup failed", "error", err)
		h.writeErrorResponse(w, "Internal server error", "INTERNAL_ERROR", http.StatusInternalServerError, nil)
		return
	}

	h.writeJSONResponse(w, user, http.StatusCreated)
}

// Login handles user login
// @Summary Login a user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param user body models.LoginRequest true "User login data"
// @Success 200 {object} services.AuthTokenResponse
// @Failure 400 {object} models.APIError
// @Failure 401 {object} models.APIError
// @Failure 500 {object} models.APIError
// @Router /login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, "Invalid JSON format", "INVALID_JSON", http.StatusBadRequest, nil)
		return
	}

	response, err := h.authService.Login(r.Context(), &req)
	if err != nil {
		if validationErr, ok := err.(models.ValidationErrors); ok {
			h.writeErrorResponse(w, "Validation failed", "VALIDATION_ERROR", http.StatusBadRequest, map[string]string(validationErr))
			return
		}
		
		if err.Error() == "invalid credentials" {
			h.writeErrorResponse(w, "Invalid credentials", "INVALID_CREDENTIALS", http.StatusUnauthorized, nil)
			return
		}
		
		h.logger.Error("login failed", "error", err)
		h.writeErrorResponse(w, "Internal server error", "INTERNAL_ERROR", http.StatusInternalServerError, nil)
		return
	}

	h.writeJSONResponse(w, response, http.StatusOK)
}

// GetProfile returns the current user's profile
// @Summary Get user profile
// @Description Get the authenticated user's profile information
// @Tags user
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} models.UserResponse
// @Failure 401 {object} models.APIError
// @Failure 404 {object} models.APIError
// @Failure 500 {object} models.APIError
// @Router /profile [get]
func (h *AuthHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		h.writeErrorResponse(w, "User not found in context", "NO_USER_CONTEXT", http.StatusUnauthorized, nil)
		return
	}

	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil {
		if err.Error() == "user not found" {
			h.writeErrorResponse(w, "User not found", "USER_NOT_FOUND", http.StatusNotFound, nil)
			return
		}
		
		h.logger.Error("get profile failed", "error", err, "user_id", userID)
		h.writeErrorResponse(w, "Internal server error", "INTERNAL_ERROR", http.StatusInternalServerError, nil)
		return
	}

	h.writeJSONResponse(w, user, http.StatusOK)
}

func (h *AuthHandler) writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode JSON response", "error", err)
	}
}

func (h *AuthHandler) writeErrorResponse(w http.ResponseWriter, message, code string, statusCode int, details map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	errorResponse := models.APIError{
		Message: message,
		Code:    code,
		Details: details,
	}
	
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}
