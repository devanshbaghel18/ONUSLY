package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	JWTSecret          string
}

func Load() Config {
	// Load .env for local development.
	// On EC2 this will simply be ignored if .env doesn't exist.
	_ = godotenv.Load()

	cfg := Config{
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
	}

	if cfg.GoogleClientID == "" {
		log.Println("warning: GOOGLE_CLIENT_ID not set")
	}

	if cfg.JWTSecret == "" {
		log.Println("warning: JWT_SECRET not set")
	}

	return cfg
}