package types

import (
	"time"
)

// CoreSession represents the persistent data stored in JSON.
// Only contains core information - all derived state (tmux, git, claude status)
// is computed fresh from external systems.
type CoreSession struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WorktreePath string    `json:"worktree_path"`
	TmuxSession  string    `json:"tmux_session"`
	CreatedAt    time.Time `json:"created_at"`
}

// Session represents the complete session state with both persistent
// and derived information.
type Session struct {
	Core         CoreSession  `json:"core"`
	IsAlive      bool         `json:"is_alive"`
	ClaudeStatus ClaudeStatus `json:"claude_status"`
	GitStatus    GitStatus    `json:"git_status"`
	LastActivity time.Time    `json:"last_activity"`
}

// ClaudeState represents the current activity state of Claude
type ClaudeState string

const (
	ClaudeWorking  ClaudeState = "working"  // Tool usage or active processing
	ClaudeWaiting  ClaudeState = "waiting"  // Waiting for user input
	ClaudeComplete ClaudeState = "complete" // Task completed
	ClaudeIdle     ClaudeState = "idle"     // Idle but ready
	ClaudeUnknown  ClaudeState = "unknown"  // Cannot determine state
)

// Availability represents how recent the Claude activity is
type Availability string

const (
	AvailCurrent   Availability = "current"    // <5 minutes
	AvailRecent    Availability = "recent"     // <1 hour
	AvailStale     Availability = "stale"      // <24 hours
	AvailVeryStale Availability = "very_stale" // >24 hours
)

// ClaudeStatus combines state and time-based availability
type ClaudeStatus struct {
	State         ClaudeState  `json:"state"`
	Availability  Availability `json:"availability"`
	LastMessage   time.Time    `json:"last_message"`
	SessionID     string       `json:"session_id,omitempty"`
	StatusMessage string       `json:"status_message,omitempty"` // Human-readable status from Claude
}

// GitStatus represents the git working tree status
type GitStatus struct {
	HasChanges     bool     `json:"has_changes"`
	ModifiedFiles  []string `json:"modified_files"`
	AddedFiles     []string `json:"added_files"`
	DeletedFiles   []string `json:"deleted_files"`
	UntrackedFiles []string `json:"untracked_files"`
	CommitCount    int      `json:"commit_count"`
}

// ClaudeMessage represents a parsed JSONL message from Claude
type ClaudeMessage struct {
	Role      string    `json:"role"`
	Content   []Content `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Content represents message content (text or tool use)
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"` // For tool_use
}

// SessionData represents the JSON structure for persistence
type SessionData struct {
	Sessions []CoreSession `json:"sessions"`
}
