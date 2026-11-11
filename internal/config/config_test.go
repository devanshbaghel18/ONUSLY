package config

import (
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantError bool
	}{
		{
			name:      "empty JWT secret fails",
			cfg:       Config{JWTSecret: "", Env: "development"},
			wantError: true,
		},
		{
			name:      "whitespace JWT secret fails",
			cfg:       Config{JWTSecret: "   ", Env: "development"},
			wantError: true,
		},
		{
			name:      "short JWT secret fails",
			cfg:       Config{JWTSecret: "short-key", Env: "development"},
			wantError: true,
		},
		{
			name:      "valid development JWT secret passes",
			cfg:       Config{JWTSecret: "super-secure-dev-jwt-secret-16chars", Env: "development"},
			wantError: false,
		},
		{
			name:      "production secret under 32 chars fails",
			cfg:       Config{JWTSecret: "valid-16char-secret-not-32", Env: "production"},
			wantError: true,
		},
		{
			name:      "production secret with weak dictionary token fails",
			cfg:       Config{JWTSecret: "my-super-secret-production-key-which-has-32-chars-long", Env: "production"},
			wantError: true,
		},
		{
			name:      "valid production JWT secret passes",
			cfg:       Config{JWTSecret: "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f", Env: "production", DynamoTable: "Onusly", AllowedOrigins: []string{"https://app.onusly.com"}},
			wantError: false,
		},
		{
			name:      "production with staging DynamoDB table fails",
			cfg:       Config{JWTSecret: "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f", Env: "production", DynamoTable: "Onusly-Staging"},
			wantError: true,
		},
		{
			name:      "production with localhost origin fails",
			cfg:       Config{JWTSecret: "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f", Env: "production", DynamoTable: "Onusly", AllowedOrigins: []string{"http://localhost:3000"}},
			wantError: true,
		},
		{
			name:      "staging with production DynamoDB table fails",
			cfg:       Config{JWTSecret: "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f", Env: "staging", DynamoTable: "Onusly"},
			wantError: true,
		},
		{
			name:      "staging with short secret fails",
			cfg:       Config{JWTSecret: "short-staging-secret-16", Env: "staging", DynamoTable: "Onusly-Staging"},
			wantError: true,
		},
		{
			name:      "valid staging config passes",
			cfg:       Config{JWTSecret: "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f", Env: "staging", DynamoTable: "Onusly-Staging", AllowedOrigins: []string{"https://staging.onusly.internal"}},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantError && err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}
