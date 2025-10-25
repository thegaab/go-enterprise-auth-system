package repository

import (
	"context"
	"auth/internal/models"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id string) error
}

type Repository struct {
	User UserRepository
}

func New(userRepo UserRepository) *Repository {
	return &Repository{
		User: userRepo,
	}
}