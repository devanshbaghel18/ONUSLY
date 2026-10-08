package goals_test

import (
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

func TestNormalizeAndValidateApps(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    []string
		expectError bool
		errorSubstr string
	}{
		{
			name:     "valid apps trimmed, case preserved, deduplicated, sorted",
			input:    []string{"  com.instagram.android  ", "com.twitter.android", "Com.Instagram.Android", "com.twitter.android"},
			expected: []string{"Com.Instagram.Android", "com.instagram.android", "com.twitter.android"},
		},
		{
			name:     "single segment invalid",
			input:    []string{"instagram"},
			expectError: true,
			errorSubstr: "must match format",
		},
		{
			name:     "starts with digit invalid",
			input:    []string{"1com.instagram.android"},
			expectError: true,
			errorSubstr: "must match format",
		},
		{
			name:     "contains hyphen invalid for java package",
			input:    []string{"com.instagram-app.android"},
			expectError: true,
			errorSubstr: "must match format",
		},
		{
			name:     "empty package name",
			input:    []string{"   "},
			expectError: true,
			errorSubstr: "cannot be empty",
		},
		{
			name:     "exceeds 255 chars",
			input:    []string{"com." + strings.Repeat("a", 255)},
			expectError: true,
			errorSubstr: "exceeds 255 characters",
		},
		{
			name: "exceeds 20 apps limit",
			input: []string{
				"com.app01", "com.app02", "com.app03", "com.app04", "com.app05",
				"com.app06", "com.app07", "com.app08", "com.app09", "com.app10",
				"com.app11", "com.app12", "com.app13", "com.app14", "com.app15",
				"com.app16", "com.app17", "com.app18", "com.app19", "com.app20",
				"com.app21",
			},
			expectError: true,
			errorSubstr: "maximum 20 apps allowed",
		},
		{
			name:     "empty slice returns empty slice",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := goals.NormalizeAndValidateApps(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("expected error containing %q, got %q", tt.errorSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(result) != len(tt.expected) {
					t.Fatalf("expected %d apps, got %d (%v)", len(tt.expected), len(result), result)
				}
				for i := range result {
					if result[i] != tt.expected[i] {
						t.Errorf("at index %d: expected %q, got %q", i, tt.expected[i], result[i])
					}
				}
			}
		})
	}
}

func TestNormalizeAndValidateDomains(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    []string
		expectError bool
		errorSubstr string
	}{
		{
			name: "schemes, ports, paths, query, leading www, trailing dot stripped, lowercased, deduplicated, sorted",
			input: []string{
				"  HTTPS://WWW.Instagram.COM:443/feed?user=1#header  ",
				"HTTP://twitter.com/",
				"instagram.com.",
				"https://www.twitter.com:8080/home",
			},
			expected: []string{"instagram.com", "twitter.com"},
		},
		{
			name:        "reject IP literal IPv4",
			input:       []string{"127.0.0.1"},
			expectError: true,
			errorSubstr: "IP literals are not allowed",
		},
		{
			name:        "reject IP literal IPv6",
			input:       []string{"::1"},
			expectError: true,
			errorSubstr: "IP literals are not allowed",
		},
		{
			name:        "reject wildcard",
			input:       []string{"*.instagram.com"},
			expectError: true,
			errorSubstr: "wildcards are not allowed",
		},
		{
			name:        "reject non-ASCII",
			input:       []string{"инстаграм.com"},
			expectError: true,
			errorSubstr: "non-ASCII characters are not allowed",
		},
		{
			name:        "reject domain with no dot (single label)",
			input:       []string{"localhost"},
			expectError: true,
			errorSubstr: "must contain at least one dot",
		},
		{
			name:        "reject label starting with hyphen",
			input:       []string{"-instagram.com"},
			expectError: true,
			errorSubstr: "contains invalid characters or hyphens",
		},
		{
			name:        "reject label ending with hyphen",
			input:       []string{"instagram-.com"},
			expectError: true,
			errorSubstr: "contains invalid characters or hyphens",
		},
		{
			name: "exceeds 20 domains limit",
			input: []string{
				"d01.com", "d02.com", "d03.com", "d04.com", "d05.com",
				"d06.com", "d07.com", "d08.com", "d09.com", "d10.com",
				"d11.com", "d12.com", "d13.com", "d14.com", "d15.com",
				"d16.com", "d17.com", "d18.com", "d19.com", "d20.com",
				"d21.com",
			},
			expectError: true,
			errorSubstr: "maximum 20 domains allowed",
		},
		{
			name:     "empty slice returns empty slice",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := goals.NormalizeAndValidateDomains(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("expected error containing %q, got %q", tt.errorSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(result) != len(tt.expected) {
					t.Fatalf("expected %d domains, got %d (%v)", len(tt.expected), len(result), result)
				}
				for i := range result {
					if result[i] != tt.expected[i] {
						t.Errorf("at index %d: expected %q, got %q", i, tt.expected[i], result[i])
					}
				}
			}
		})
	}
}
