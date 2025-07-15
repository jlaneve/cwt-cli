package tmux

import (
	"os/exec"
	"testing"
)

func TestRealChecker_IsSessionAlive(t *testing.T) {
	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}

	checker := NewRealChecker()

	// Test with non-existent session (should return false)
	alive := checker.IsSessionAlive("non-existent-session-12345")
	if alive {
		t.Error("IsSessionAlive(non-existent-session) = true, want false")
	}
}

func TestMockChecker(t *testing.T) {
	mock := NewMockChecker()

	// Test default behavior (no sessions alive)
	if mock.IsSessionAlive("test-session") {
		t.Error("IsSessionAlive() = true, want false (default)")
	}

	// Test setting session alive
	mock.SetSessionAlive("test-session", true)
	if !mock.IsSessionAlive("test-session") {
		t.Error("IsSessionAlive() = false, want true after SetSessionAlive")
	}

	// Test with dead sessions
	mock.SetSessionAlive("dead-session", false)
	if mock.IsSessionAlive("dead-session") {
		t.Error("IsSessionAlive(dead-session) = true, want false")
	}

	// Test ListSessions
	mock.SetSessionAlive("session-1", true)
	mock.SetSessionAlive("session-2", true)
	sessions, err := mock.ListSessions()
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListSessions() returned %d sessions, want 3", len(sessions))
	}

	// Test CreateSession
	err = mock.CreateSession("new-session", "/tmp", "test-command")
	if err != nil {
		t.Errorf("CreateSession() error = %v", err)
	}
	if len(mock.CreatedSessions) != 1 {
		t.Errorf("CreateSession() should track created sessions, got %d", len(mock.CreatedSessions))
	}
	if !mock.IsSessionAlive("new-session") {
		t.Error("CreateSession() should mark session as alive")
	}

	// Test KillSession
	err = mock.KillSession("session-1")
	if err != nil {
		t.Errorf("KillSession() error = %v", err)
	}
	if mock.IsSessionAlive("session-1") {
		t.Error("KillSession() should mark session as dead")
	}
	if len(mock.KilledSessions) != 1 {
		t.Errorf("KillSession() should track killed sessions, got %d", len(mock.KilledSessions))
	}

	// Test CaptureOutput
	mock.Output["session-with-output"] = "test output"
	output, err := mock.CaptureOutput("session-with-output")
	if err != nil {
		t.Errorf("CaptureOutput() error = %v", err)
	}
	if output != "test output" {
		t.Errorf("CaptureOutput() = %q, want 'test output'", output)
	}

	// Test CaptureOutput with no configured output
	_, err = mock.CaptureOutput("session-no-output")
	if err == nil {
		t.Error("CaptureOutput() with no configured output should return error")
	}

	// Test failure modes
	mock.ShouldFailCreate = true
	err = mock.CreateSession("fail-session", "/tmp", "command")
	if err == nil {
		t.Error("CreateSession() with ShouldFailCreate = true should return error")
	}
}
