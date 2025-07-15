package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// startEventChannelListener creates a command that listens for file events
func (m Model) startEventChannelListener() tea.Cmd {
	return func() tea.Msg {
		// This will block until an event is received
		return <-m.eventChan
	}
}

// File watching setup
func (m Model) setupFileWatching() tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to create file watcher: %w", err)}
		}

		// Watch session state directory for hook events
		sessionStateDir := filepath.Join(m.stateManager.GetDataDir(), "session-state")
		if err := os.MkdirAll(sessionStateDir, 0755); err == nil {
			if err := watcher.Add(sessionStateDir); err != nil {
				return errorMsg{err: fmt.Errorf("failed to watch session state directory: %w", err)}
			}
			if debugLogger != nil {
				debugLogger.Printf("Watching session state directory: %s", sessionStateDir)
			}
		}

		// Watch sessions.json for session CRUD
		sessionsFile := filepath.Join(m.stateManager.GetDataDir(), "sessions.json")
		if _, err := os.Stat(sessionsFile); err == nil {
			if err := watcher.Add(sessionsFile); err != nil {
				return errorMsg{err: fmt.Errorf("failed to watch sessions file: %w", err)}
			}
			if debugLogger != nil {
				debugLogger.Printf("Watching sessions file: %s", sessionsFile)
			}
		}

		// Watch git index files for each session
		for _, session := range m.sessions {
			m.addSessionWatches(watcher, session)
			if debugLogger != nil {
				gitIndexPath := filepath.Join(session.Core.WorktreePath, ".git", "index")
				debugLogger.Printf("Watching git index for session %s: %s", session.Core.Name, gitIndexPath)
			}
		}

		// Store the eventChan in the watcher context
		eventChan := m.eventChan

		// Start listening for file events
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}

					// Debug logging for file events
					if debugLogger != nil {
						debugLogger.Printf("File event: %s %s", event.Op, event.Name)
					}

					// Determine event type based on file path and send appropriate message
					if filepath.Base(filepath.Dir(event.Name)) == "session-state" {
						// Session state change (hook event)
						go func() {
							time.Sleep(100 * time.Millisecond) // Debounce
							if debugLogger != nil {
								debugLogger.Printf("Sending sessionStateChangedMsg for: %s", event.Name)
							}
							select {
							case eventChan <- sessionStateChangedMsg{}:
							default: // Channel full, skip this event
								if debugLogger != nil {
									debugLogger.Printf("Event channel full, skipping sessionStateChangedMsg")
								}
							}
						}()
					} else if filepath.Base(event.Name) == "sessions.json" {
						// Session list change
						go func() {
							time.Sleep(100 * time.Millisecond) // Debounce
							if debugLogger != nil {
								debugLogger.Printf("Sending sessionListChangedMsg for: %s", event.Name)
							}
							select {
							case eventChan <- sessionListChangedMsg{}:
							default: // Channel full, skip this event
								if debugLogger != nil {
									debugLogger.Printf("Event channel full, skipping sessionListChangedMsg")
								}
							}
						}()
					} else if filepath.Base(event.Name) == "index" {
						// Git index change
						sessionID := m.getSessionIDFromPath(event.Name)
						if sessionID != "" {
							go func(sID string) {
								time.Sleep(100 * time.Millisecond) // Debounce
								if debugLogger != nil {
									debugLogger.Printf("Sending gitIndexChangedMsg for session: %s", sID)
								}
								select {
								case eventChan <- gitIndexChangedMsg{sessionID: sID}:
								default: // Channel full, skip this event
									if debugLogger != nil {
										debugLogger.Printf("Event channel full, skipping gitIndexChangedMsg")
									}
								}
							}(sessionID)
						}
					}

				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					// Send error message to TUI
					select {
					case eventChan <- errorMsg{err: fmt.Errorf("file watcher error: %w", err)}:
					default: // Channel full, skip this event
					}
				}
			}
		}()

		// Return the watcher setup message so the model can store it
		return fileWatcherSetupMsg{watcher: watcher}
	}
}

// Helper to add git index watching for a session
func (m Model) addSessionWatches(watcher *fsnotify.Watcher, session types.Session) {
	gitIndexPath := filepath.Join(session.Core.WorktreePath, ".git", "index")
	if _, err := os.Stat(gitIndexPath); err == nil {
		watcher.Add(gitIndexPath)
	}
}

// addNewSessionWatches adds file watches for a newly created session
func (m Model) addNewSessionWatches(session types.Session) {
	if m.fileWatcher != nil {
		m.addSessionWatches(m.fileWatcher, session)
	}
}

// Helper to extract session ID from git index path
func (m Model) getSessionIDFromPath(path string) string {
	// Extract session ID from path like .cwt/worktrees/session-name/.git/index
	// This is a simplified version - in practice, we'd need to map paths to session IDs
	for _, session := range m.sessions {
		if filepath.Dir(path) == filepath.Join(session.Core.WorktreePath, ".git") {
			return session.Core.ID
		}
	}
	return ""
}

// Polling commands
func (m Model) startGitPolling() tea.Cmd {
	return tea.Every(10*time.Second, func(time.Time) tea.Msg {
		return gitStatusRefreshMsg{}
	})
}

func (m Model) startTmuxPolling() tea.Cmd {
	return tea.Every(30*time.Second, func(time.Time) tea.Msg {
		return tmuxStatusRefreshMsg{}
	})
}

// Session management commands
func (m Model) refreshSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.stateManager.DeriveFreshSessions()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to refresh sessions: %w", err)}
		}
		return refreshCompleteMsg{sessions: sessions}
	}
}

func (m Model) refreshSessionGitStatus(sessionID string) tea.Cmd {
	return func() tea.Msg {
		// Refresh just git status for specific session
		// For now, refresh all sessions (optimize later)
		sessions, err := m.stateManager.DeriveFreshSessions()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to refresh git status: %w", err)}
		}
		return refreshCompleteMsg{sessions: sessions}
	}
}

func (m Model) refreshAllGitStatus() tea.Cmd {
	return m.refreshSessions() // For now, just refresh everything
}

func (m Model) refreshTmuxStatus() tea.Cmd {
	return m.refreshSessions() // For now, just refresh everything
}

// User action commands
func (m Model) handleAttach(sessionID string) tea.Cmd {
	// Get access to the logger from model.go
	return func() tea.Msg {
		if debugLogger != nil {
			debugLogger.Printf("handleAttach: Called with sessionID: %s", sessionID)
		}

		session := m.findSession(sessionID)
		if session == nil {
			if debugLogger != nil {
				debugLogger.Printf("handleAttach: Session not found for ID: %s", sessionID)
			}
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		if debugLogger != nil {
			debugLogger.Printf("handleAttach: Found session: %s, IsAlive: %v", session.Core.Name, session.IsAlive)
		}

		if !session.IsAlive {
			if debugLogger != nil {
				debugLogger.Printf("handleAttach: Session %s is dead, showing confirmation dialog", session.Core.Name)
			}
			// Return a command to show confirmation dialog
			return showConfirmDialogMsg{
				message: fmt.Sprintf("Session '%s' tmux is not running. Recreate it?", session.Core.Name),
				onYes: func() tea.Cmd {
					return m.recreateAndAttach(sessionID)
				},
				onNo: func() tea.Cmd {
					return nil
				},
			}
		}

		if debugLogger != nil {
			debugLogger.Printf("handleAttach: Session %s is alive, proceeding to attach", session.Core.Name)
		}
		// Attach to alive session
		return m.attachToSession(sessionID)
	}
}

func (m Model) recreateAndAttach(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session := m.findSession(sessionID)
		if session == nil {
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		// Recreate the tmux session directly (worktree already exists)
		// Find claude executable
		claudeExec := m.findClaudeExecutable()
		var command string
		if claudeExec != "" {
			// Check if there's an existing Claude session to resume for this worktree
			if existingSessionID, err := m.stateManager.GetClaudeChecker().FindSessionID(session.Core.WorktreePath); err == nil && existingSessionID != "" {
				command = fmt.Sprintf("%s -r %s", claudeExec, existingSessionID)
				if debugLogger != nil {
					debugLogger.Printf("Resuming Claude session %s for worktree %s", existingSessionID, session.Core.WorktreePath)
				}
			} else {
				command = claudeExec
				if debugLogger != nil {
					debugLogger.Printf("Starting new Claude session for worktree %s", session.Core.WorktreePath)
				}
			}
		}

		// Create new tmux session
		if err := m.stateManager.GetTmuxChecker().CreateSession(
			session.Core.TmuxSession,
			session.Core.WorktreePath,
			command,
		); err != nil {
			return errorMsg{err: fmt.Errorf("failed to recreate tmux session: %w", err)}
		}

		// Now request attachment
		return attachRequestMsg{sessionName: session.Core.TmuxSession}
	}
}

func (m Model) attachToSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		if debugLogger != nil {
			debugLogger.Printf("attachToSession: Called with sessionID: %s", sessionID)
		}

		session := m.findSession(sessionID)
		if session == nil {
			if debugLogger != nil {
				debugLogger.Printf("attachToSession: Session not found for ID: %s", sessionID)
			}
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		if debugLogger != nil {
			debugLogger.Printf("attachToSession: Returning attachRequestMsg for tmux session: %s", session.Core.TmuxSession)
		}

		// Return a special message that tells the TUI to exit and attach
		return attachRequestMsg{sessionName: session.Core.TmuxSession}
	}
}

// startSessionCreation is no longer needed - replaced with overlay dialog

func (m Model) confirmDelete(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session := m.findSession(sessionID)
		if session == nil {
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		return showConfirmDialogMsg{
			message: fmt.Sprintf("Delete session '%s' and all its resources?", session.Core.Name),
			onYes: func() tea.Cmd {
				return m.deleteSession(sessionID)
			},
			onNo: func() tea.Cmd {
				return nil
			},
		}
	}
}

func (m Model) deleteSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		if err := m.stateManager.DeleteSession(sessionID); err != nil {
			return errorMsg{err: fmt.Errorf("failed to delete session: %w", err)}
		}

		// Refresh session list after deletion
		sessions, err := m.stateManager.DeriveFreshSessions()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to refresh after deletion: %w", err)}
		}

		return refreshCompleteMsg{sessions: sessions}
	}
}

func (m Model) runCleanup() tea.Cmd {
	return func() tea.Msg {
		// Find and clean up stale sessions
		staleSessions, err := m.stateManager.FindStaleSessions()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to find stale sessions: %w", err)}
		}

		if len(staleSessions) == 0 {
			return errorMsg{err: fmt.Errorf("no orphaned sessions found")}
		}

		// Clean up each stale session
		for _, session := range staleSessions {
			if err := m.stateManager.DeleteSession(session.Core.ID); err != nil {
				return errorMsg{err: fmt.Errorf("failed to cleanup session %s: %w", session.Core.Name, err)}
			}
		}

		// Refresh session list after cleanup
		sessions, err := m.stateManager.DeriveFreshSessions()
		if err != nil {
			return errorMsg{err: fmt.Errorf("failed to refresh after cleanup: %w", err)}
		}

		return refreshCompleteMsg{sessions: sessions}
	}
}

// findClaudeExecutable searches for claude in common installation paths
func (m Model) findClaudeExecutable() string {
	// Check common installation paths
	claudePaths := []string{
		"claude",
		os.ExpandEnv("$HOME/.claude/local/claude"),
		os.ExpandEnv("$HOME/.claude/local/node_modules/.bin/claude"),
		"/usr/local/bin/claude",
	}

	for _, path := range claudePaths {
		cmd := exec.Command(path, "--version")
		if err := cmd.Run(); err == nil {
			return path
		}
	}

	return ""
}

// loadDiffData loads diff data for the current session
func (m Model) loadDiffData() tea.Cmd {
	return func() tea.Msg {
		if m.diffMode == nil {
			return diffErrorMsg{err: fmt.Errorf("diff mode not initialized")}
		}

		// Change to session worktree directory
		originalDir, err := os.Getwd()
		if err != nil {
			return diffErrorMsg{err: fmt.Errorf("failed to get current directory: %w", err)}
		}
		defer os.Chdir(originalDir)

		// Validate that the worktree path is a git repository
		worktreePath := m.diffMode.session.Core.WorktreePath
		if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err != nil {
			return diffErrorMsg{err: fmt.Errorf("not a git repository: %s", worktreePath)}
		}

		if err := os.Chdir(worktreePath); err != nil {
			return diffErrorMsg{err: fmt.Errorf("failed to change to worktree directory: %w", err)}
		}

		// Build git diff command
		var cmd *exec.Cmd
		if m.diffMode.cached {
			cmd = exec.Command("git", "diff", "--cached", "--no-color")
		} else {
			cmd = exec.Command("git", "diff", m.diffMode.target, "--no-color")
		}

		output, err := cmd.Output()
		if err != nil {
			return diffErrorMsg{err: fmt.Errorf("failed to get diff: %w", err)}
		}

		// Parse diff output into DiffLine structures
		diffLines := parseDiffOutput(string(output))
		return diffLoadedMsg{diffLines: diffLines}
	}
}

// parseDiffOutput parses git diff output into structured diff lines
func parseDiffOutput(output string) []DiffLine {
	lines := strings.Split(output, "\n")
	var diffLines []DiffLine
	var currentHunk int
	var currentFile string
	var oldLineNum, newLineNum int

	for _, line := range lines {
		if line == "" {
			continue
		}

		var diffLine DiffLine
		diffLine.Content = line
		diffLine.HunkID = currentHunk
		diffLine.FileName = currentFile
		diffLine.OldLine = oldLineNum
		diffLine.NewLine = newLineNum

		switch {
		case strings.HasPrefix(line, "diff --git"):
			diffLine.Type = DiffLineFileHeader
			// Extract filename from diff header
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentFile = strings.TrimPrefix(parts[3], "b/")
			}

		case strings.HasPrefix(line, "index "):
			diffLine.Type = DiffLineHeader

		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
			diffLine.Type = DiffLineHeader

		case strings.HasPrefix(line, "@@"):
			diffLine.Type = DiffLineHunkHeader
			currentHunk++
			// Parse line numbers from hunk header
			if matches := regexp.MustCompile(`-(\d+)(?:,\d+)? \+(\d+)`).FindStringSubmatch(line); len(matches) >= 3 {
				if old, err := strconv.Atoi(matches[1]); err == nil {
					oldLineNum = old
				}
				if new, err := strconv.Atoi(matches[2]); err == nil {
					newLineNum = new
				}
			}

		case strings.HasPrefix(line, "+"):
			diffLine.Type = DiffLineAdded
			diffLine.NewLine = newLineNum
			newLineNum++

		case strings.HasPrefix(line, "-"):
			diffLine.Type = DiffLineRemoved
			diffLine.OldLine = oldLineNum
			oldLineNum++

		case strings.HasPrefix(line, " "):
			diffLine.Type = DiffLineContext
			diffLine.OldLine = oldLineNum
			diffLine.NewLine = newLineNum
			oldLineNum++
			newLineNum++

		case strings.HasPrefix(line, "\\"):
			diffLine.Type = DiffLineNoNewline

		default:
			diffLine.Type = DiffLineContext
		}

		diffLines = append(diffLines, diffLine)
	}

	return diffLines
}
