package types

// Event represents all possible events in the system
type Event interface {
	EventType() string
}

// Immediate Events - triggered by user actions for instant UI feedback

// SessionCreationStarted is emitted when session creation begins
type SessionCreationStarted struct {
	Name string `json:"name"`
}

func (e SessionCreationStarted) EventType() string { return "session_creation_started" }

// SessionCreated is emitted when session creation completes successfully
type SessionCreated struct {
	Session Session `json:"session"`
}

func (e SessionCreated) EventType() string { return "session_created" }

// SessionCreationFailed is emitted when session creation fails
type SessionCreationFailed struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

func (e SessionCreationFailed) EventType() string { return "session_creation_failed" }

// SessionDeleted is emitted when session deletion completes
type SessionDeleted struct {
	SessionID string `json:"session_id"`
}

func (e SessionDeleted) EventType() string { return "session_deleted" }

// SessionDeletionFailed is emitted when session deletion fails
type SessionDeletionFailed struct {
	SessionID string `json:"session_id"`
	Error     string `json:"error"`
}

func (e SessionDeletionFailed) EventType() string { return "session_deletion_failed" }

// Periodic Events - triggered by external state changes

// ClaudeStatusChanged is emitted when Claude status changes
type ClaudeStatusChanged struct {
	SessionID string       `json:"session_id"`
	OldStatus ClaudeStatus `json:"old_status"`
	NewStatus ClaudeStatus `json:"new_status"`
}

func (e ClaudeStatusChanged) EventType() string { return "claude_status_changed" }

// TmuxSessionDied is emitted when a tmux session dies
type TmuxSessionDied struct {
	SessionID   string `json:"session_id"`
	TmuxSession string `json:"tmux_session"`
}

func (e TmuxSessionDied) EventType() string { return "tmux_session_died" }

// GitChangesDetected is emitted when git changes are detected
type GitChangesDetected struct {
	SessionID string    `json:"session_id"`
	NewStatus GitStatus `json:"new_status"`
}

func (e GitChangesDetected) EventType() string { return "git_changes_detected" }

// RefreshCompleted is emitted when external state refresh completes
type RefreshCompleted struct {
	Sessions []Session `json:"sessions"`
	Error    string    `json:"error,omitempty"`
}

func (e RefreshCompleted) EventType() string { return "refresh_completed" }