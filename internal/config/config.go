package config

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	JWTSecret          string
	Env                string
	AllowedOrigins     []string
	DynamoTable        string
}

func Load() Config {
	// Load .env for local development.
	// On EC2 this will simply be ignored if .env doesn't exist.
	_ = godotenv.Load()

	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	}
	if env == "" {
		env = "development"
	}

	// Default allowed origins for frontend local dev and testing
	defaultOrigins := []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:5173",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
	}

	allowedMap := make(map[string]bool)
	if env != "production" {
		for _, o := range defaultOrigins {
			allowedMap[o] = true
		}
	}

	envOrigins := os.Getenv("ALLOWED_ORIGINS")
	if envOrigins != "" {
		for _, o := range strings.Split(envOrigins, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				allowedMap[trimmed] = true
			}
		}
	}

	var allowedOrigins []string
	for o := range allowedMap {
		allowedOrigins = append(allowedOrigins, o)
	}

	dynamoTable := strings.TrimSpace(os.Getenv("DYNAMODB_TABLE"))
	if dynamoTable == "" {
		dynamoTable = "Onusly"
	}

	cfg := Config{
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		Env:                env,
		AllowedOrigins:     allowedOrigins,
		DynamoTable:        dynamoTable,
	}

	if cfg.GoogleClientID == "" {
		log.Println("warning: GOOGLE_CLIENT_ID not set")
	}

	if cfg.JWTSecret == "" {
		log.Println("warning: JWT_SECRET not set")
	}

	return cfg
}

// Validate checks that required security secrets and environment settings are configured.
func (c Config) Validate() error {
	jwtSec := strings.TrimSpace(c.JWTSecret)
	if jwtSec == "" {
		return errors.New("critical security error: JWT_SECRET is not set")
	}
	if len(jwtSec) < 16 {
		return errors.New("critical security error: JWT_SECRET must be at least 16 characters")
	}

	envLower := strings.ToLower(c.Env)

	if envLower == "production" {
		if len(jwtSec) < 32 {
			return errors.New("critical security error: production JWT_SECRET must be at least 32 characters")
		}
		insecureTokens := []string{"secret", "password", "development", "staging", "1234567890123456"}
		lower := strings.ToLower(jwtSec)
		for _, bad := range insecureTokens {
			if strings.Contains(lower, bad) {
				return errors.New("critical security error: production JWT_SECRET contains weak/insecure dictionary token: " + bad)
			}
		}
		tableLower := strings.ToLower(c.DynamoTable)
		if strings.Contains(tableLower, "staging") || strings.Contains(tableLower, "test") {
			return errors.New("critical configuration error: production environment cannot use staging/test DynamoDB table: " + c.DynamoTable)
		}
		for _, o := range c.AllowedOrigins {
			if strings.Contains(o, "localhost") || strings.Contains(o, "127.0.0.1") {
				return errors.New("critical configuration error: production environment cannot allow localhost origin: " + o)
			}
		}
	} else if envLower == "staging" {
		if len(jwtSec) < 32 {
			return errors.New("critical security error: staging JWT_SECRET must be at least 32 characters")
		}
		insecureTokens := []string{"secret", "password", "development", "1234567890123456"}
		lower := strings.ToLower(jwtSec)
		for _, bad := range insecureTokens {
			if strings.Contains(lower, bad) {
				return errors.New("critical security error: staging JWT_SECRET contains weak/insecure dictionary token: " + bad)
			}
		}
		if c.DynamoTable == "Onusly" || c.DynamoTable == "" {
			return errors.New("critical configuration error: staging environment must use an isolated DynamoDB table (e.g. Onusly-Staging), cannot target production table: " + c.DynamoTable)
		}
	}

	return nil
}
