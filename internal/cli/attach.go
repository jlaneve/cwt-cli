package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

func newAttachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach [session-name]",
		Short: "Attach to a session's tmux session",
		Long: `Attach to the tmux session for a CWT session.

This is a convenience command that replaces the need to remember
tmux session names (cwt-{session-name}). 

If session-name is not provided, you will be prompted to select
from available sessions.`,
		Aliases: []string{"a"},
		Args:    cobra.MaximumNArgs(1),
		RunE:    runAttachCmd,
	}

	return cmd
}

func runAttachCmd(cmd *cobra.Command, args []string) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	// Get sessions
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		fmt.Println("Create a new session with: cwt new [session-name]")
		return fmt.Errorf("no sessions available to attach to")
	}

	// Determine which session to attach to
	var sessionToAttach *types.Session

	if len(args) > 0 {
		// Session name provided
		sessionName := args[0]
		for i := range sessions {
			if sessions[i].Core.Name == sessionName {
				sessionToAttach = &sessions[i]
				break
			}
		}

		if sessionToAttach == nil {
			return fmt.Errorf("session '%s' not found", sessionName)
		}
	} else {
		// Interactive selection
		selected, err := promptForAttachSelection(sessions)
		if err != nil {
			return err
		}
		sessionToAttach = selected
	}

	// Check if tmux session is alive
	if !sessionToAttach.IsAlive {
		fmt.Printf("⚠️  Tmux session for '%s' is not running.\n", sessionToAttach.Core.Name)
		fmt.Printf("This might happen if:\n")
		fmt.Printf("  • The Claude Code process exited\n")
		fmt.Printf("  • The tmux session was manually terminated\n")
		fmt.Printf("  • There was a system restart\n\n")
		
		// Ask user if they want to recreate the session
		fmt.Printf("Do you want to recreate the tmux session? (y/N): ")
		var response string
		fmt.Scanln(&response)
		
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Session not recreated.")
			return fmt.Errorf("cannot attach to dead tmux session")
		}
		
		// Recreate the tmux session with Claude resumption
		if err := recreateSessionWithClaudeResume(sm, sessionToAttach); err != nil {
			return fmt.Errorf("failed to recreate session: %w", err)
		}
		
		fmt.Printf("✅ Session '%s' recreated successfully\n", sessionToAttach.Core.Name)
	}

	// Attach to tmux session
	fmt.Printf("🔗 Attaching to session '%s' (tmux: %s)...\n", 
		sessionToAttach.Core.Name, sessionToAttach.Core.TmuxSession)
	
	// Use exec to replace current process with tmux attach
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	args = []string{"tmux", "attach-session", "-t", sessionToAttach.Core.TmuxSession}
	err = syscall.Exec(tmuxPath, args, os.Environ())
	if err != nil {
		return fmt.Errorf("failed to exec tmux: %w", err)
	}

	// This point should never be reached if exec succeeds
	return nil
}

func promptForAttachSelection(sessions []types.Session) (*types.Session, error) {
	fmt.Println("Multiple sessions found. Select one to attach to:")
	
	// Filter to only show alive sessions
	aliveSessions := make([]types.Session, 0)
	deadSessions := make([]types.Session, 0)
	
	for _, session := range sessions {
		if session.IsAlive {
			aliveSessions = append(aliveSessions, session)
		} else {
			deadSessions = append(deadSessions, session)
		}
	}

	if len(aliveSessions) == 0 {
		fmt.Println("❌ No active tmux sessions found.")
		if len(deadSessions) > 0 {
			fmt.Printf("Found %d stale session(s). Run 'cwt cleanup' to remove them.\n", len(deadSessions))
		}
		return nil, fmt.Errorf("no active sessions to attach to")
	}

	if len(deadSessions) > 0 {
		fmt.Printf("Found %d stale session(s). Run 'cwt cleanup' to remove them.\n", len(deadSessions))
	}

	// Use interactive selector for alive sessions
	selectedSession, err := SelectSession(aliveSessions, WithTitle("Select a session to attach to:"))
	if err != nil {
		return nil, fmt.Errorf("failed to select session: %w", err)
	}

	if selectedSession == nil {
		fmt.Println("Cancelled")
		return nil, nil
	}

	return selectedSession, nil
}

// recreateSessionWithClaudeResume recreates a dead tmux session and resumes Claude if possible
func recreateSessionWithClaudeResume(sm *state.Manager, session *types.Session) error {
	// Find Claude executable
	claudeExec := findClaudeExecutable()
	if claudeExec == "" {
		return fmt.Errorf("claude executable not found")
	}
	
	// Check if there's an existing Claude session to resume for this worktree
	var command string
	if existingSessionID, err := sm.GetClaudeChecker().FindSessionID(session.Core.WorktreePath); err == nil && existingSessionID != "" {
		command = fmt.Sprintf("%s -r %s", claudeExec, existingSessionID)
		fmt.Printf("📋 Resuming Claude session %s\n", existingSessionID)
	} else {
		command = claudeExec
		fmt.Printf("🆕 Starting new Claude session\n")
	}
	
	// Recreate the tmux session
	tmuxChecker := sm.GetTmuxChecker()
	if err := tmuxChecker.CreateSession(session.Core.TmuxSession, session.Core.WorktreePath, command); err != nil {
		return fmt.Errorf("failed to recreate tmux session: %w", err)
	}
	
	return nil
}

// findClaudeExecutable searches for claude in common installation paths
func findClaudeExecutable() string {
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