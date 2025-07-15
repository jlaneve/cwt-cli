package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// newDiffCmd creates the 'cwt diff' command
func newDiffCmd() *cobra.Command {
	var against string
	var web bool
	var stat bool
	var name bool
	var cached bool

	cmd := &cobra.Command{
		Use:   "diff [session-name]",
		Short: "Show detailed diff for session changes",
		Long: `Show comprehensive diff view of changes in a session with rich formatting.

Examples:
  cwt diff my-session               # Show full diff for session
  cwt diff my-session --stat        # Show diff statistics only
  cwt diff my-session --against main # Compare against specific branch
  cwt diff my-session --web          # Open diff in external viewer
  cwt diff my-session --cached       # Show staged changes only
  cwt diff                          # Interactive session selector`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sm, err := createStateManager()
			if err != nil {
				return err
			}
			defer sm.Close()

			if len(args) == 0 {
				return interactiveDiff(sm, against, web, stat, name, cached)
			}

			sessionName := args[0]
			return showSessionDiff(sm, sessionName, against, web, stat, name, cached)
		},
	}

	cmd.Flags().StringVar(&against, "against", "", "Compare against specific branch (default: base branch)")
	cmd.Flags().BoolVar(&web, "web", false, "Open diff in external viewer")
	cmd.Flags().BoolVar(&stat, "stat", false, "Show diff statistics only")
	cmd.Flags().BoolVar(&name, "name-only", false, "Show only file names")
	cmd.Flags().BoolVar(&cached, "cached", false, "Show staged changes only")

	return cmd
}

// showSessionDiff displays the diff for a specific session
func showSessionDiff(sm *state.Manager, sessionName, against string, web, stat, nameOnly, cached bool) error {
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	// Find the session
	var targetSession *types.Session
	for _, session := range sessions {
		if session.Core.Name == sessionName {
			targetSession = &session
			break
		}
	}

	if targetSession == nil {
		return fmt.Errorf("session '%s' not found", sessionName)
	}

	return renderSessionDiff(*targetSession, against, web, stat, nameOnly, cached)
}

// interactiveDiff provides an interactive session selector for diff
func interactiveDiff(sm *state.Manager, against string, web, stat, nameOnly, cached bool) error {
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions available for diff")
		return nil
	}

	// Use selector with filter for sessions with changes
	selectedSession, err := SelectSession(sessions, 
		WithTitle("Select a session to view diff:"),
		WithSessionFilter(func(session types.Session) bool {
			return session.GitStatus.HasChanges
		}))
	
	if err != nil {
		return fmt.Errorf("failed to select session: %w", err)
	}

	if selectedSession == nil {
		fmt.Println("Cancelled")
		return nil
	}

	return renderSessionDiff(*selectedSession, against, web, stat, nameOnly, cached)
}

// renderSessionDiff renders the diff for a session
func renderSessionDiff(session types.Session, against string, web, stat, nameOnly, cached bool) error {
	// Change to session worktree directory
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(session.Core.WorktreePath); err != nil {
		return fmt.Errorf("failed to change to worktree directory: %w", err)
	}

	// Determine comparison target
	target := against
	if target == "" {
		target = "main" // Default base branch
	}

	// Open in external viewer if requested
	if web {
		return openDiffInExternalViewer(target, cached)
	}

	// Show diff header
	fmt.Printf("📋 Diff for session: %s\n", session.Core.Name)
	fmt.Printf("📂 Path: %s\n", session.Core.WorktreePath)
	
	if cached {
		fmt.Printf("🔍 Comparing: staged changes\n")
	} else {
		fmt.Printf("🔍 Comparing: working tree vs %s\n", target)
	}
	
	fmt.Println(strings.Repeat("=", 70))

	// Show summary stats first
	if err := showDiffStats(target, cached); err != nil {
		fmt.Printf("Warning: failed to show diff stats: %v\n", err)
	}

	if stat {
		return nil // Only show stats
	}

	fmt.Println(strings.Repeat("-", 70))

	// Show file names only if requested
	if nameOnly {
		return showDiffFileNames(target, cached)
	}

	// Show full diff with syntax highlighting
	return showFullDiff(target, cached)
}

// showDiffStats shows diff statistics
func showDiffStats(target string, cached bool) error {
	var cmd *exec.Cmd
	
	if cached {
		cmd = exec.Command("git", "diff", "--cached", "--stat")
	} else {
		cmd = exec.Command("git", "diff", target, "--stat")
	}

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	if len(output) > 0 {
		fmt.Printf("📊 Change Statistics:\n")
		fmt.Print(string(output))
	} else {
		fmt.Println("📊 No changes found")
	}

	return nil
}

// showDiffFileNames shows only the names of changed files
func showDiffFileNames(target string, cached bool) error {
	var cmd *exec.Cmd
	
	if cached {
		cmd = exec.Command("git", "diff", "--cached", "--name-status")
	} else {
		cmd = exec.Command("git", "diff", target, "--name-status")
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get file names: %w", err)
	}

	if len(output) == 0 {
		fmt.Println("No files changed")
		return nil
	}

	fmt.Println("📁 Changed Files:")
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		
		status := parts[0]
		filename := parts[1]
		
		icon := getFileStatusIcon(status)
		fmt.Printf("  %s %s %s\n", icon, status, filename)
	}

	return nil
}

// showFullDiff shows the complete diff with syntax highlighting
func showFullDiff(target string, cached bool) error {
	var cmd *exec.Cmd
	
	if cached {
		cmd = exec.Command("git", "diff", "--cached", "--color=always")
	} else {
		cmd = exec.Command("git", "diff", target, "--color=always")
	}

	// Try to use a pager if available (less, more, etc.)
	if isInteractiveTerminal() {
		if pager := getPager(); pager != "" {
			return runDiffWithPager(cmd, pager)
		}
	}

	// Fallback to direct output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to show diff: %w", err)
	}

	return nil
}

// openDiffInExternalViewer opens the diff in an external application
func openDiffInExternalViewer(target string, cached bool) error {
	// Try different diff viewers in order of preference
	viewers := []string{
		"code --diff", // VSCode
		"subl --wait", // Sublime Text
		"mate -w",     // TextMate
		"vim -d",      // Vim
	}

	for _, viewer := range viewers {
		if cmd := strings.Fields(viewer); len(cmd) > 0 {
			if _, err := exec.LookPath(cmd[0]); err == nil {
				return openWithViewer(viewer, target, cached)
			}
		}
	}

	// Fallback to system default
	return openWithSystemDefault(target, cached)
}

// openWithViewer opens diff with a specific viewer
func openWithViewer(viewer, target string, cached bool) error {
	// For now, just show the diff in terminal with a message
	fmt.Printf("🔧 External viewer integration not yet implemented\n")
	fmt.Printf("📋 Preferred viewer: %s\n", viewer)
	fmt.Println("📋 Falling back to terminal diff:")
	fmt.Println(strings.Repeat("-", 50))
	
	return showFullDiff(target, cached)
}

// openWithSystemDefault opens diff with system default application
func openWithSystemDefault(target string, cached bool) error {
	fmt.Println("🔧 System default diff viewer not yet implemented")
	fmt.Println("📋 Falling back to terminal diff:")
	fmt.Println(strings.Repeat("-", 50))
	
	return showFullDiff(target, cached)
}

// Helper functions

func getFileStatusIcon(status string) string {
	switch status {
	case "A":
		return "➕"
	case "M":
		return "📝"
	case "D":
		return "➖"
	case "R":
		return "📋"
	case "C":
		return "📄"
	default:
		return "❓"
	}
}

func isInteractiveTerminal() bool {
	// Simple check for interactive terminal
	if os.Getenv("TERM") == "" {
		return false
	}
	
	// Check if stdout is a terminal
	if stat, err := os.Stdout.Stat(); err == nil {
		return (stat.Mode() & os.ModeCharDevice) != 0
	}
	
	return false
}

func getPager() string {
	// Check environment variables for pager preference
	if pager := os.Getenv("GIT_PAGER"); pager != "" {
		return pager
	}
	
	if pager := os.Getenv("PAGER"); pager != "" {
		return pager
	}
	
	// Try common pagers
	pagers := []string{"less", "more", "cat"}
	for _, pager := range pagers {
		if _, err := exec.LookPath(pager); err == nil {
			if pager == "less" {
				return "less -R" // Enable color support
			}
			return pager
		}
	}
	
	return ""
}

func runDiffWithPager(cmd *exec.Cmd, pager string) error {
	// Create a pipe from git diff to pager
	pagerCmd := exec.Command("sh", "-c", pager)
	
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}
	
	pagerCmd.Stdin = pipe
	pagerCmd.Stdout = os.Stdout
	pagerCmd.Stderr = os.Stderr
	
	if err := pagerCmd.Start(); err != nil {
		return fmt.Errorf("failed to start pager: %w", err)
	}
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start git diff: %w", err)
	}
	
	if err := cmd.Wait(); err != nil {
		pipe.Close()
		return fmt.Errorf("git diff failed: %w", err)
	}
	
	pipe.Close()
	
	if err := pagerCmd.Wait(); err != nil {
		return fmt.Errorf("pager failed: %w", err)
	}
	
	return nil
}