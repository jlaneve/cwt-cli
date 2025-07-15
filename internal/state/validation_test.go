package state

import (
	"strings"
	"testing"
)

func TestValidateSessionName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		errorMsg  string
	}{
		// Valid names
		{"valid simple name", "session-one", false, ""},
		{"valid with numbers", "session123", false, ""},
		{"valid with underscores", "session_name", false, ""},
		{"valid with dots", "session.name", false, ""},
		{"valid mixed case", "SessionName", false, ""},
		{"valid single char", "s", false, ""},
		{"valid 50 chars", strings.Repeat("a", 50), false, ""},
		{"valid unicode", "session-café", false, ""},
		{"valid emoji", "session-🚀", false, ""},
		
		// Invalid names
		{"empty name", "", true, "session name cannot be empty"},
		{"too long", strings.Repeat("a", 51), true, "session name too long"},
		{"spaces", "session name", true, "invalid characters in session name: ' '"},
		{"tilde", "session~name", true, "invalid characters in session name: '~'"},
		{"caret", "session^name", true, "invalid characters in session name: '^'"},
		{"colon", "session:name", true, "invalid characters in session name: ':'"},
		{"question", "session?name", true, "invalid characters in session name: '?'"},
		{"asterisk", "session*name", true, "invalid characters in session name: '*'"},
		{"bracket", "session[name", true, "invalid characters in session name: '['"},
		{"backslash", "session\\name", true, "invalid characters in session name: '\\'"},
		{"double dot", "session..name", true, "invalid characters in session name: '..'"},
		{"at brace", "session@{name", true, "invalid characters in session name: '@{'"},
		{"starts with dash", "-session", true, "session name cannot start or end with '-'"},
		{"ends with dash", "session-", true, "session name cannot start or end with '-'"},
		{"starts with dot", ".session", true, "session name cannot start or end with '.'"},
		{"ends with dot", "session.", true, "session name cannot start or end with '.'"},
		{"starts with slash", "/session", true, "session name cannot start or end with '/'"},
		{"just numbers", "123", true, "session name cannot be just numbers"},
		{"reserved main", "main", true, "'main' is a reserved name"},
		{"reserved master", "master", true, "'master' is a reserved name"},
		{"reserved HEAD", "HEAD", true, "'HEAD' is a reserved name"},
		{"ends with .lock", "session.lock", true, "session name must be a valid git branch name"},
		{"zero width char", "session\u200Bname", true, "session name must be a valid git branch name"},
		{"control char", "session\x01name", true, "session name must be a valid git branch name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionName(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateSessionName(%q) = nil, want error", tt.input)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateSessionName(%q) = %v, want error containing %q", tt.input, err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateSessionName(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}

func TestIsNumericOnly(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123", true},
		{"0", true},
		{"999999", true},
		{"123abc", false},
		{"abc123", false},
		{"", false},
		{"1.23", false},
		{"-123", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isNumericOnly(tt.input); got != tt.want {
				t.Errorf("isNumericOnly(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}