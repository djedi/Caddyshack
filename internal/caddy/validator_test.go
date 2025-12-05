package caddy

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestParseValidationErrors tests the error parsing logic.
func TestParseValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantLine int
		wantMsg  string
	}{
		{
			name:     "caddyfile with line number",
			input:    "Caddyfile:42 - Error: invalid directive",
			wantLen:  1,
			wantLine: 42,
			wantMsg:  "Error: invalid directive",
		},
		{
			name:     "line number format",
			input:    "line 10: unrecognized directive",
			wantLen:  1,
			wantLine: 10,
			wantMsg:  "unrecognized directive",
		},
		{
			name:     "error keyword without line",
			input:    "Error: something went wrong",
			wantLen:  1,
			wantLine: 0,
			wantMsg:  "Error: something went wrong",
		},
		{
			name:     "invalid keyword",
			input:    "invalid syntax near bracket",
			wantLen:  1,
			wantLine: 0,
			wantMsg:  "invalid syntax near bracket",
		},
		{
			name:     "empty input",
			input:    "",
			wantLen:  0,
			wantLine: 0,
			wantMsg:  "",
		},
		{
			name:     "multiple errors",
			input:    "Caddyfile:10 - Error: first error\nCaddyfile:20 - Error: second error",
			wantLen:  2,
			wantLine: 10,
			wantMsg:  "Error: first error",
		},
		{
			name:     "unrecognized keyword",
			input:    "unrecognized directive: foobar",
			wantLen:  1,
			wantLine: 0,
			wantMsg:  "unrecognized directive: foobar",
		},
		{
			name:     "expected keyword",
			input:    "expected token but got eof",
			wantLen:  1,
			wantLine: 0,
			wantMsg:  "expected token but got eof",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := parseValidationErrors(tt.input)

			if len(errors) != tt.wantLen {
				t.Errorf("parseValidationErrors() got %d errors, want %d", len(errors), tt.wantLen)
				return
			}

			if tt.wantLen > 0 {
				if errors[0].Line != tt.wantLine {
					t.Errorf("parseValidationErrors() line = %d, want %d", errors[0].Line, tt.wantLine)
				}
				if errors[0].Message != tt.wantMsg {
					t.Errorf("parseValidationErrors() message = %q, want %q", errors[0].Message, tt.wantMsg)
				}
			}
		})
	}
}

// TestValidationResultString tests the String() method.
func TestValidationResultString(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   string
	}{
		{
			name:   "valid result",
			result: &ValidationResult{Valid: true},
			want:   "Configuration is valid",
		},
		{
			name: "invalid with line number",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Line: 5, Message: "invalid directive"},
				},
			},
			want: "Configuration is invalid:\n  Line 5: invalid directive\n",
		},
		{
			name: "invalid without line number",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Line: 0, Message: "something went wrong"},
				},
			},
			want: "Configuration is invalid:\n  something went wrong\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.String()
			if got != tt.want {
				t.Errorf("ValidationResult.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestValidationResultError tests the Error() method.
func TestValidationResultError(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   string
	}{
		{
			name:   "valid result",
			result: &ValidationResult{Valid: true},
			want:   "",
		},
		{
			name: "invalid with errors",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Message: "first error"},
					{Message: "second error"},
				},
			},
			want: "first error",
		},
		{
			name:   "invalid without errors",
			result: &ValidationResult{Valid: false},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Error()
			if got != tt.want {
				t.Errorf("ValidationResult.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewValidator tests the Validator constructor and method chaining.
func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v.caddyBinary != "caddy" {
		t.Errorf("NewValidator() caddyBinary = %q, want %q", v.caddyBinary, "caddy")
	}
	if v.timeout != 30*time.Second {
		t.Errorf("NewValidator() timeout = %v, want %v", v.timeout, 30*time.Second)
	}

	// Test method chaining
	v = v.WithCaddyBinary("/usr/local/bin/caddy").WithTimeout(10 * time.Second)
	if v.caddyBinary != "/usr/local/bin/caddy" {
		t.Errorf("WithCaddyBinary() = %q, want %q", v.caddyBinary, "/usr/local/bin/caddy")
	}
	if v.timeout != 10*time.Second {
		t.Errorf("WithTimeout() = %v, want %v", v.timeout, 10*time.Second)
	}
}

// TestValidatorWithNonexistentBinary tests behavior with missing caddy binary.
func TestValidatorWithNonexistentBinary(t *testing.T) {
	v := NewValidator().WithCaddyBinary("/nonexistent/caddy")

	result, err := v.ValidateContent("localhost:8080 {\n}")

	// Should return an error since the binary doesn't exist
	if err == nil && result != nil && result.Valid {
		t.Error("ValidateContent() with nonexistent binary should fail")
	}
}

// Integration tests that require a real caddy binary.
// These tests are skipped if caddy is not installed.

func caddyAvailable() bool {
	_, err := exec.LookPath("caddy")
	return err == nil
}

func TestValidateContent_Integration(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("caddy binary not available, skipping integration test")
	}

	tests := []struct {
		name      string
		content   string
		wantValid bool
	}{
		{
			name:      "valid simple site",
			content:   "localhost:8080 {\n\trespond \"Hello\"\n}\n",
			wantValid: true,
		},
		{
			name:      "valid with reverse proxy",
			content:   "example.com {\n\treverse_proxy localhost:3000\n}\n",
			wantValid: true,
		},
		{
			name:      "valid multiple sites",
			content:   "a.example.com {\n\trespond \"A\"\n}\n\nb.example.com {\n\trespond \"B\"\n}\n",
			wantValid: true,
		},
		{
			name:      "invalid directive",
			content:   "localhost:8080 {\n\tnonexistent_directive foo\n}\n",
			wantValid: false,
		},
		{
			name:      "syntax error - missing brace",
			content:   "localhost:8080 {\n\trespond \"Hello\"\n",
			wantValid: false,
		},
	}

	v := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.ValidateContent(tt.content)
			if err != nil {
				// Some validation errors may cause exec errors
				if tt.wantValid {
					t.Errorf("ValidateContent() error = %v, expected valid config", err)
				}
				return
			}

			if result.Valid != tt.wantValid {
				t.Errorf("ValidateContent() valid = %v, want %v. Errors: %v", result.Valid, tt.wantValid, result.Errors)
			}

			if !result.Valid && len(result.Errors) == 0 {
				t.Error("ValidateContent() returned invalid but no errors")
			}
		})
	}
}

func TestValidateContent_WithGlobalOptions(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("caddy binary not available, skipping integration test")
	}

	content := `{
	email admin@example.com
	admin off
}

example.com {
	reverse_proxy localhost:8080
}
`

	v := NewValidator()
	result, err := v.ValidateContent(content)
	if err != nil {
		t.Fatalf("ValidateContent() error = %v", err)
	}

	if !result.Valid {
		t.Errorf("ValidateContent() valid = false, want true. Errors: %v", result.Errors)
	}
}

func TestValidateContent_WithSnippets(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("caddy binary not available, skipping integration test")
	}

	content := `(common) {
	encode gzip
	header X-Frame-Options "DENY"
}

example.com {
	import common
	respond "Hello"
}
`

	v := NewValidator()
	result, err := v.ValidateContent(content)
	if err != nil {
		t.Fatalf("ValidateContent() error = %v", err)
	}

	if !result.Valid {
		t.Errorf("ValidateContent() valid = false, want true. Errors: %v", result.Errors)
	}
}

func TestValidateContent_ErrorDetails(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("caddy binary not available, skipping integration test")
	}

	// Invalid config with unknown directive
	content := `localhost:8080 {
	fake_directive_that_does_not_exist
}
`

	v := NewValidator()
	result, err := v.ValidateContent(content)
	if err != nil {
		t.Fatalf("ValidateContent() error = %v", err)
	}

	if result.Valid {
		t.Error("ValidateContent() valid = true, want false for invalid directive")
		return
	}

	if len(result.Errors) == 0 {
		t.Error("ValidateContent() returned no errors for invalid config")
		return
	}

	// Check that we have some error message
	hasMessage := false
	for _, e := range result.Errors {
		if e.Message != "" {
			hasMessage = true
			// The error should mention the unknown directive
			if strings.Contains(strings.ToLower(e.Message), "unrecognized") ||
				strings.Contains(strings.ToLower(e.Message), "unknown") ||
				strings.Contains(strings.ToLower(e.Message), "fake_directive") {
				return // Success - found the expected error
			}
		}
	}

	if !hasMessage {
		t.Errorf("ValidateContent() error messages are empty. Got: %+v", result.Errors)
	}
}
