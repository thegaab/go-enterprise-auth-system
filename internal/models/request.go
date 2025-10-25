package models

import (
	"errors"
	"regexp"
	"strings"
)

// SignUpRequest defines the structure for a sign up request
type SignUpRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8"`
	Email    string `json:"email" validate:"required,email"`
}

// LoginRequest defines the structure for a login request
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// APIError represents an API error response
type APIError struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Details map[string]string `json:"details,omitempty"`
}

// ValidationErrors represents validation errors
type ValidationErrors map[string]string

func (v ValidationErrors) Error() string {
	var errs []string
	for field, msg := range v {
		errs = append(errs, field+": "+msg)
	}
	return strings.Join(errs, ", ")
}

// Validate validates the SignUpRequest
func (r *SignUpRequest) Validate() error {
	errors := make(ValidationErrors)

	if r.Username == "" {
		errors["username"] = "username is required"
	} else if len(r.Username) < 3 {
		errors["username"] = "username must be at least 3 characters"
	} else if len(r.Username) > 50 {
		errors["username"] = "username must be less than 50 characters"
	}

	if r.Password == "" {
		errors["password"] = "password is required"
	} else if len(r.Password) < 8 {
		errors["password"] = "password must be at least 8 characters"
	}

	if r.Email == "" {
		errors["email"] = "email is required"
	} else if !isValidEmail(r.Email) {
		errors["email"] = "invalid email format"
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// Validate validates the LoginRequest
func (r *LoginRequest) Validate() error {
	errors := make(ValidationErrors)

	if r.Username == "" {
		errors["username"] = "username is required"
	}

	if r.Password == "" {
		errors["password"] = "password is required"
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

func isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
