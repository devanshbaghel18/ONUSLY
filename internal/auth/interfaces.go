package auth

import "context"

type Repository interface {
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByHandle(ctx context.Context, handle string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUserHandle(ctx context.Context, userID, oldHandle, newHandle string) error
}
