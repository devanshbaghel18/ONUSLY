package auth

import "context"

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByHandle(ctx context.Context, handle string) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUserHandle(ctx context.Context, userID, handle string) error
}