package state

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// validateSessionName validates a session name according to git branch naming rules
// Based on the validation logic from archive/internal/cli/new.go
func validateSessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	if len(name) > 50 {
		return fmt.Errorf("session name too long (max 50 characters)")
	}

	// Check for invalid characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "..", "@{"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("invalid characters in session name: '%s' not allowed", char)
		}
	}

	// Cannot start or end with certain characters
	invalidStartEnd := []string{"-", "/", "."}
	for _, char := range invalidStartEnd {
		if strings.HasPrefix(name, char) || strings.HasSuffix(name, char) {
			return fmt.Errorf("session name cannot start or end with '%s'", char)
		}
	}

	// Cannot be just numbers
	if isNumericOnly(name) {
		return fmt.Errorf("session name cannot be just numbers")
	}

	// Check for reserved names
	reservedNames := []string{"main", "master", "HEAD", "refs"}
	for _, reserved := range reservedNames {
		if strings.EqualFold(name, reserved) {
			return fmt.Errorf("'%s' is a reserved name and cannot be used", name)
		}
	}

	// Additional git ref name validation
	if !isValidGitRefName(name) {
		return fmt.Errorf("session name must be a valid git branch name")
	}

	return nil
}

func isNumericOnly(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func isValidGitRefName(name string) bool {
	// Git ref name rules (simplified):
	// - ASCII control characters (< 32 or 127) are not allowed
	// - Space, ~, ^, :, ?, *, [, \ are not allowed (already checked above)
	// - Cannot be empty or start with /
	// - Cannot end with .lock
	// - Cannot contain .. or @{
	// - Cannot start or end with /
	// - No zero-width or format characters

	if strings.HasSuffix(name, ".lock") {
		return false
	}

	// Check for ASCII control characters and problematic Unicode characters
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
		// Only reject zero-width and format characters that cause issues
		// Cf = Format characters (includes zero-width space, etc.)
		if unicode.In(r, unicode.Cf) {
			return false
		}
	}

	// Must contain at least one valid character that's not a special character
	validCharRegex := regexp.MustCompile(`[a-zA-Z0-9_-]`)
	if !validCharRegex.MatchString(name) {
		return false
	}

	return true
}
