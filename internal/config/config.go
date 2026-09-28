package config

import "os"

type Config struct {
    GoogleClientID string
    JWTSecret       string
}

func Load() Config {
    return Config{
        GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
        JWTSecret:      os.Getenv("JWT_SECRET"),
    }
}