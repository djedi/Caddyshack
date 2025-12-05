package caddy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ValidationError represents a single validation error from Caddy.
type ValidationError struct {
	Line    int    // Line number where the error occurred (0 if unknown)
	Message string // Error message
}

// ValidationResult contains the result of validating a Caddyfile.
type ValidationResult struct {
	Valid  bool              // Whether the Caddyfile is valid
	Errors []ValidationError // List of validation errors (empty if valid)
}

// Validator handles Caddyfile validation by shelling out to the caddy binary.
type Validator struct {
	caddyBinary string        // Path to the caddy binary
	timeout     time.Duration // Timeout for validation command
}

// NewValidator creates a new Validator with default settings.
func NewValidator() *Validator {
	return &Validator{
		caddyBinary: "caddy",
		timeout:     30 * time.Second,
	}
}

// WithCaddyBinary sets a custom path to the caddy binary.
func (v *Validator) WithCaddyBinary(path string) *Validator {
	v.caddyBinary = path
	return v
}

// WithTimeout sets a custom timeout for the validation command.
func (v *Validator) WithTimeout(timeout time.Duration) *Validator {
	v.timeout = timeout
	return v
}

// ValidateFile validates a Caddyfile at the given path.
// It shells out to `caddy validate --config <path>` and parses the output.
func (v *Validator) ValidateFile(path string) (*ValidationResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), v.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.caddyBinary, "validate", "--config", path)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check if the context timed out
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("validation timed out after %v", v.timeout)
	}

	// If no error, the config is valid
	if err == nil {
		return &ValidationResult{Valid: true}, nil
	}

	// Parse validation errors from stderr
	errors := parseValidationErrors(stderr.String())
	if len(errors) == 0 {
		// If we couldn't parse specific errors, include the raw output
		combinedOutput := strings.TrimSpace(stderr.String())
		if combinedOutput == "" {
			combinedOutput = strings.TrimSpace(stdout.String())
		}
		if combinedOutput == "" {
			combinedOutput = err.Error()
		}
		errors = []ValidationError{{Message: combinedOutput}}
	}

	return &ValidationResult{
		Valid:  false,
		Errors: errors,
	}, nil
}

// ValidateContent validates Caddyfile content provided as a string.
// It writes the content to a temporary file and validates it.
func (v *Validator) ValidateContent(content string) (*ValidationResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), v.timeout)
	defer cancel()

	// Use caddy adapt to validate from stdin
	cmd := exec.CommandContext(ctx, v.caddyBinary, "adapt", "--config", "-", "--adapter", "caddyfile", "--validate")
	cmd.Stdin = strings.NewReader(content)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check if the context timed out
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("validation timed out after %v", v.timeout)
	}

	// If no error, the config is valid
	if err == nil {
		return &ValidationResult{Valid: true}, nil
	}

	// Parse validation errors from stderr
	errors := parseValidationErrors(stderr.String())
	if len(errors) == 0 {
		// If we couldn't parse specific errors, include the raw output
		combinedOutput := strings.TrimSpace(stderr.String())
		if combinedOutput == "" {
			combinedOutput = strings.TrimSpace(stdout.String())
		}
		if combinedOutput == "" {
			combinedOutput = err.Error()
		}
		errors = []ValidationError{{Message: combinedOutput}}
	}

	return &ValidationResult{
		Valid:  false,
		Errors: errors,
	}, nil
}

// parseValidationErrors parses caddy validation output to extract structured errors.
// Caddy outputs errors in various formats, this function attempts to parse common patterns.
func parseValidationErrors(output string) []ValidationError {
	var errors []ValidationError

	// Common patterns in Caddy error output
	patterns := []*regexp.Regexp{
		// Pattern: "Caddyfile:42 - Error: message"
		regexp.MustCompile(`(?i)caddyfile:(\d+)\s*[-:]\s*(.+)`),
		// Pattern: "line 42: message"
		regexp.MustCompile(`(?i)line\s+(\d+):\s*(.+)`),
		// Pattern: "Error: message at Caddyfile:42"
		regexp.MustCompile(`(?i)error:\s*(.+?)\s+at\s+.*?:(\d+)`),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matched := false
		for _, pattern := range patterns {
			matches := pattern.FindStringSubmatch(line)
			if matches != nil {
				var lineNum int
				var msg string

				// Different patterns have different capture group orders
				if pattern.String() == patterns[2].String() {
					// "Error: message at Caddyfile:42"
					msg = strings.TrimSpace(matches[1])
					fmt.Sscanf(matches[2], "%d", &lineNum)
				} else {
					// Other patterns have line number first
					fmt.Sscanf(matches[1], "%d", &lineNum)
					msg = strings.TrimSpace(matches[2])
				}

				errors = append(errors, ValidationError{
					Line:    lineNum,
					Message: msg,
				})
				matched = true
				break
			}
		}

		// If no pattern matched, check for general error messages
		if !matched {
			// Look for lines that contain error indicators
			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, "error") ||
				strings.Contains(lowerLine, "invalid") ||
				strings.Contains(lowerLine, "unknown") ||
				strings.Contains(lowerLine, "unrecognized") ||
				strings.Contains(lowerLine, "expected") {
				errors = append(errors, ValidationError{
					Line:    0,
					Message: line,
				})
			}
		}
	}

	return errors
}

// String returns a human-readable representation of the validation result.
func (r *ValidationResult) String() string {
	if r.Valid {
		return "Configuration is valid"
	}

	var sb strings.Builder
	sb.WriteString("Configuration is invalid:\n")
	for _, err := range r.Errors {
		if err.Line > 0 {
			sb.WriteString(fmt.Sprintf("  Line %d: %s\n", err.Line, err.Message))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", err.Message))
		}
	}
	return sb.String()
}

// Error returns the first error message if invalid, empty string otherwise.
func (r *ValidationResult) Error() string {
	if r.Valid || len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Message
}
