package auth

import (
	"context"
	"errors"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/idtoken"
)

type Service struct {
	repo Repository
	cfg  config.Config
}

func NewService(repo Repository, cfg config.Config) *Service {
	return &Service{
		repo: repo,
		cfg:  cfg,
	}
}

func (s *Service) GoogleLogin(ctx context.Context, idTokenString string) (*GoogleLoginResponse, error) {

	if idTokenString == "" {
		return nil, errors.New("idToken is required")
	}

	// Verify the Google ID token
	payload, err := idtoken.Validate(ctx, idTokenString, s.cfg.GoogleClientID)
	if err != nil {
		return nil, errors.New("invalid Google token")
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	user := User{
		ID:        payload.Subject,
		Email:     email,
		Name:      name,
		Picture:   picture,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// TODO: Check if user exists in DynamoDB.
	// TODO: Create user if it doesn't exist.

	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	return &GoogleLoginResponse{
		User:  user,
		Token: signedToken,
	}, nil
}