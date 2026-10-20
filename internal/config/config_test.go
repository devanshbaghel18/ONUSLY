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
			cfg:       Config{JWTSecret: ""},
			wantError: true,
		},
		{
			name:      "whitespace JWT secret fails",
			cfg:       Config{JWTSecret: "   "},
			wantError: true,
		},
		{
			name:      "short JWT secret fails",
			cfg:       Config{JWTSecret: "short-key"},
			wantError: true,
		},
		{
			name:      "valid JWT secret passes",
			cfg:       Config{JWTSecret: "super-secure-production-jwt-secret-key-32chars"},
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
