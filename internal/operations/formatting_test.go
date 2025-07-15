package operations

import (
	"strings"
	"testing"
	"time"

	"github.com/jlaneve/cwt-cli/internal/types"
)

func TestStatusFormat_FormatTmuxStatus(t *testing.T) {
	formatter := NewStatusFormat()

	tests := []struct {
		name     string
		isAlive  bool
		expected string
	}{
		{"alive session", true, "🟢 alive"},
		{"dead session", false, "🔴 dead"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatTmuxStatus(tt.isAlive)
			if result != tt.expected {
				t.Errorf("FormatTmuxStatus(%v) = %q, want %q", tt.isAlive, result, tt.expected)
			}
		})
	}
}

func TestStatusFormat_FormatClaudeStatus(t *testing.T) {
	formatter := NewStatusFormat()

	tests := []struct {
		name     string
		status   types.ClaudeStatus
		expected string
	}{
		{
			name:     "working status",
			status:   types.ClaudeStatus{State: types.ClaudeWorking},
			expected: "🔵 working",
		},
		{
			name:     "idle status",
			status:   types.ClaudeStatus{State: types.ClaudeIdle},
			expected: "🟡 idle",
		},
		{
			name:     "waiting status",
			status:   types.ClaudeStatus{State: types.ClaudeWaiting},
			expected: "⏸️ waiting",
		},
		{
			name:     "waiting with message",
			status:   types.ClaudeStatus{State: types.ClaudeWaiting, StatusMessage: "waiting for input"},
			expected: "⏸️ waiting for input",
		},
		{
			name:     "complete status",
			status:   types.ClaudeStatus{State: types.ClaudeComplete},
			expected: "✅ complete",
		},
		{
			name:     "unknown status",
			status:   types.ClaudeStatus{State: types.ClaudeUnknown},
			expected: "❓ unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatClaudeStatus(tt.status)
			if result != tt.expected {
				t.Errorf("FormatClaudeStatus(%+v) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestStatusFormat_FormatGitStatus(t *testing.T) {
	formatter := NewStatusFormat()

	tests := []struct {
		name     string
		status   types.GitStatus
		expected string
	}{
		{
			name:     "clean repository",
			status:   types.GitStatus{HasChanges: false},
			expected: "🟢 clean",
		},
		{
			name: "one modified file",
			status: types.GitStatus{
				HasChanges:    true,
				ModifiedFiles: []string{"test.go"},
			},
			expected: "🟡 1 file",
		},
		{
			name: "multiple modified files",
			status: types.GitStatus{
				HasChanges:    true,
				ModifiedFiles: []string{"test.go", "main.go"},
			},
			expected: "🟡 2 files",
		},
		{
			name: "one untracked file",
			status: types.GitStatus{
				HasChanges:     true,
				UntrackedFiles: []string{"new.txt"},
			},
			expected: "🟡 1 untracked",
		},
		{
			name: "mixed changes",
			status: types.GitStatus{
				HasChanges:     true,
				ModifiedFiles:  []string{"test.go"},
				UntrackedFiles: []string{"new.txt", "other.txt"},
			},
			expected: "🟡 1 file, 2 untracked",
		},
		{
			name: "changes without specific files",
			status: types.GitStatus{
				HasChanges: true,
			},
			expected: "🟡 changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatGitStatus(tt.status)
			if result != tt.expected {
				t.Errorf("FormatGitStatus(%+v) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestStatusFormat_FormatDuration(t *testing.T) {
	formatter := NewStatusFormat()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"30 seconds", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute"},
		{"5 minutes", 5 * time.Minute, "5 minutes"},
		{"1 hour", 1 * time.Hour, "1 hour"},
		{"3 hours", 3 * time.Hour, "3 hours"},
		{"1 day", 24 * time.Hour, "1 day"},
		{"3 days", 72 * time.Hour, "3 days"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestStatusFormat_FormatActivity(t *testing.T) {
	formatter := NewStatusFormat()

	now := time.Now()
	
	tests := []struct {
		name         string
		lastActivity time.Time
		expected     string
	}{
		{"never active", time.Time{}, "never"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"2 days ago", now.Add(-48 * time.Hour), "2 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatActivity(tt.lastActivity)
			if result != tt.expected {
				t.Errorf("FormatActivity(%v) = %q, want %q", tt.lastActivity, result, tt.expected)
			}
		})
	}
}

func TestStatusFormat_FormatSessionSummary(t *testing.T) {
	formatter := NewStatusFormat()

	session := types.Session{
		Core: types.CoreSession{
			Name: "test-session",
		},
		IsAlive: true,
		ClaudeStatus: types.ClaudeStatus{
			State: types.ClaudeWorking,
		},
		GitStatus: types.GitStatus{
			HasChanges: false,
		},
		LastActivity: time.Now().Add(-10 * time.Minute),
	}

	result := formatter.FormatSessionSummary(session)
	
	// Check that all components are present
	expectedParts := []string{
		"tmux: 🟢 alive",
		"claude: 🔵 working", 
		"git: 🟢 clean",
		"activity:",
		"minutes ago",
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("FormatSessionSummary() result missing expected part %q\nGot: %q", part, result)
		}
	}
}

func TestStatusFormat_FormatSessionList(t *testing.T) {
	formatter := NewStatusFormat()

	// Test empty list
	result := formatter.FormatSessionList([]types.Session{}, false)
	expected := "No sessions found."
	if result != expected {
		t.Errorf("FormatSessionList(empty) = %q, want %q", result, expected)
	}

	// Test single session
	sessions := []types.Session{
		{
			Core: types.CoreSession{
				ID:   "test-id",
				Name: "test-session",
			},
			IsAlive: true,
			ClaudeStatus: types.ClaudeStatus{
				State: types.ClaudeIdle,
			},
			GitStatus: types.GitStatus{
				HasChanges: false,
			},
			LastActivity: time.Now(),
		},
	}

	// Test simple format
	result = formatter.FormatSessionList(sessions, false)
	if !strings.Contains(result, "📂 test-session") {
		t.Errorf("FormatSessionList() missing session name, got: %q", result)
	}

	// Test detailed format
	result = formatter.FormatSessionList(sessions, true)
	if !strings.Contains(result, "test-id") {
		t.Errorf("FormatSessionList(detailed) missing session ID, got: %q", result)
	}
}