package auth

import (
	"context"
	"errors"
)

type DynamoRepository struct{}

func NewRepository() Repository {
	return &DynamoRepository{}
}

func (r *DynamoRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return nil, errors.New("TODO: implement DynamoDB GetUserByEmail")
}

func (r *DynamoRepository) CreateUser(ctx context.Context, user *User) error {
	return errors.New("TODO: implement DynamoDB CreateUser")
}