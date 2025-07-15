package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionState represents real-time state for a session
// This is updated by hooks and other external events
type SessionState struct {
	SessionID       string                 `json:"session_id"`
	ClaudeState     string                 `json:"claude_state"`     // "working", "waiting_for_input", "complete", "idle"
	LastEvent       string                 `json:"last_event"`       // "notification", "stop", "preToolUse", etc.
	LastEventTime   time.Time              `json:"last_event_time"`
	LastEventData   map[string]interface{} `json:"last_event_data,omitempty"`
	LastMessage     string                 `json:"last_message,omitempty"`     // Human-readable message from Claude
	LastUpdated     time.Time              `json:"last_updated"`
}

// LoadSessionState loads session state from the dedicated state file
func LoadSessionState(dataDir, sessionID string) (*SessionState, error) {
	stateFile := filepath.Join(dataDir, "session-state", sessionID+".json")
	
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No state file yet
		}
		return nil, fmt.Errorf("failed to read session state file: %w", err)
	}
	
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse session state: %w", err)
	}
	
	return &state, nil
}

// SaveSessionState saves session state to the dedicated state file
func SaveSessionState(dataDir string, state *SessionState) error {
	stateDir := filepath.Join(dataDir, "session-state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create session state directory: %w", err)
	}
	
	stateFile := filepath.Join(stateDir, state.SessionID+".json")
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}
	
	// Atomic write using temporary file
	tempFile := stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	
	if err := os.Rename(tempFile, stateFile); err != nil {
		os.Remove(tempFile) // Cleanup temp file
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}
	
	return nil
}

// RemoveSessionState removes the session state file
func RemoveSessionState(dataDir, sessionID string) error {
	stateFile := filepath.Join(dataDir, "session-state", sessionID+".json")
	err := os.Remove(stateFile)
	if os.IsNotExist(err) {
		return nil // Already removed
	}
	return err
}

// ParseClaudeStateFromEvent determines Claude state from hook event data
func ParseClaudeStateFromEvent(eventType string, eventData map[string]interface{}) string {
	switch eventType {
	case "notification":
		// Check if this is a "waiting for input" notification
		if reason, ok := eventData["reason"].(string); ok {
			if reason == "idle" || reason == "waiting_for_permission" {
				return "waiting_for_input"
			}
		}
		// Check message content for permission requests
		if message, ok := eventData["message"].(string); ok {
			if strings.Contains(strings.ToLower(message), "permission") || 
			   strings.Contains(strings.ToLower(message), "needs your") {
				return "waiting_for_input"
			}
		}
		return "idle"
	case "preToolUse":
		return "working"
	case "postToolUse":
		return "idle"
	case "stop":
		return "complete"
	default:
		return "idle"
	}
}

// GetClaudeStatusFromState converts session state to ClaudeStatus
func GetClaudeStatusFromState(state *SessionState) ClaudeStatus {
	if state == nil {
		return ClaudeStatus{
			State: ClaudeUnknown,
		}
	}
	
	claudeState := ClaudeUnknown
	switch state.ClaudeState {
	case "working":
		claudeState = ClaudeWorking
	case "waiting_for_input":
		claudeState = ClaudeWaiting
	case "complete":
		claudeState = ClaudeComplete
	case "idle":
		claudeState = ClaudeIdle
	}
	
	return ClaudeStatus{
		State:         claudeState,
		LastMessage:   state.LastEventTime,
		SessionID:     state.SessionID,
		StatusMessage: state.LastMessage,
	}
}