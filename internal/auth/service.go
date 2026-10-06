package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/idtoken"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/devanshbaghel18/ONUSLY/internal/config"
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

func (s *Service) HandleCallback(ctx context.Context, code string) (string, error) {
	conf := &oauth2.Config{
		ClientID:     s.cfg.GoogleClientID,
		ClientSecret: s.cfg.GoogleClientSecret,
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		Scopes:       []string{"email", "profile"},
		Endpoint:     google.Endpoint,
	}

	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", errors.New("failed to exchange code: " + err.Error())
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", errors.New("no id_token in response")
	}

	resp, err := s.GoogleLogin(ctx, idToken)
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// sanitizeBaseHandle derives a clean, alphanumeric handle base from name or email.
func sanitizeBaseHandle(name, email string) string {
	base := strings.TrimSpace(strings.ToLower(name))
	if base == "" && email != "" {
		parts := strings.Split(email, "@")
		base = strings.ToLower(parts[0])
	}

	var b strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '-' || r == '.' {
			b.WriteRune('_')
		}
	}
	res := strings.Trim(b.String(), "_")
	if len(res) < 3 {
		if res == "" {
			res = "user"
		} else {
			res = "user_" + res
		}
	}
	if len(res) > 15 {
		res = res[:15]
	}
	return strings.Trim(res, "_")
}

// generateUniqueHandle creates a unique handle candidate for a new or existing user.
func (s *Service) generateUniqueHandle(ctx context.Context, name, email string) (string, error) {
	base := sanitizeBaseHandle(name, email)
	candidate := base

	user, err := s.repo.GetUserByHandle(ctx, candidate)
	if err == nil && user == nil {
		return candidate, nil
	}

	for i := 1; i <= 30; i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
		user, err := s.repo.GetUserByHandle(ctx, candidate)
		if err == nil && user == nil {
			return candidate, nil
		}
	}

	// Suffix with current seconds if collisions persist
	return fmt.Sprintf("%s_%d", base, time.Now().Unix()%10000), nil
}

func (s *Service) GoogleLogin(ctx context.Context, idTokenString string) (*GoogleLoginResponse, error) {
	payload, err := idtoken.Validate(ctx, idTokenString, s.cfg.GoogleClientID)
	if err != nil {
		return nil, errors.New("invalid Google token")
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	existingUser, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	var user User

	if existingUser != nil {
		user = *existingUser
		// Auto-generate handle for existing users that don't have one yet
		if user.Handle == "" {
			handle, hErr := s.generateUniqueHandle(ctx, user.Name, user.Email)
			if hErr == nil {
				user.Handle = handle
				_ = s.repo.UpdateUserHandle(ctx, user.ID, handle)
			}
		}
	} else {
		handle, _ := s.generateUniqueHandle(ctx, name, email)
		user = User{
			ID:        payload.Subject,
			Email:     email,
			Handle:    handle,
			Name:      name,
			Picture:   picture,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}

		if err := s.repo.CreateUser(ctx, &user); err != nil {
			return nil, err
		}
	}

	claims := jwt.MapClaims{
		"sub":    user.ID,
		"email":  user.Email,
		"handle": user.Handle,
		"name":   user.Name,
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(),
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

// UpdateUserHandle validates, checks uniqueness, and updates the public handle for a user.
func (s *Service) UpdateUserHandle(ctx context.Context, userID, newHandle string) (*PublicProfile, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(newHandle)), "@")
	if len(clean) < 3 || len(clean) > 20 {
		return nil, errors.New("handle must be between 3 and 20 characters")
	}

	for _, r := range clean {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return nil, errors.New("handle can only contain lowercase letters, numbers, and underscores")
		}
	}

	existing, err := s.repo.GetUserByHandle(ctx, clean)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.ID != userID {
		return nil, errors.New("this handle is already taken, please choose another")
	}

	if err := s.repo.UpdateUserHandle(ctx, userID, clean); err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return &PublicProfile{
			ID:     userID,
			Handle: clean,
		}, nil
	}

	return &PublicProfile{
		ID:      user.ID,
		Name:    user.Name,
		Handle:  clean,
		Picture: user.Picture,
	}, nil
}

// LookupUserByHandle returns the public profile for a handle (email is strictly hidden).
func (s *Service) LookupUserByHandle(ctx context.Context, handle string) (*PublicProfile, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if clean == "" {
		return nil, errors.New("handle cannot be empty")
	}

	user, err := s.repo.GetUserByHandle(ctx, clean)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	return &PublicProfile{
		ID:      user.ID,
		Name:    user.Name,
		Handle:  user.Handle,
		Picture: user.Picture,
	}, nil
}

// GetUserByID returns the full user object for the authenticated owner.
func (s *Service) GetUserByID(ctx context.Context, userID string) (*User, error) {
	return s.repo.GetUserByID(ctx, userID)
}