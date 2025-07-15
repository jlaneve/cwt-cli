package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/operations"
)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new [session-name]",
		Short: "Create a new session with isolated git worktree and tmux session",
		Long: `Create a new CWT session with:
- Isolated git worktree in .cwt/worktrees/[session-name]
- New tmux session running Claude Code
- Session metadata persistence

If session-name is not provided, you will be prompted interactively.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runNewCmd,
	}

	return cmd
}

func runNewCmd(cmd *cobra.Command, args []string) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	// Get session name
	var sessionName string
	if len(args) > 0 {
		sessionName = args[0]
	} else {
		sessionName, err = promptForSessionName()
		if err != nil {
			return err
		}
	}

	// Create session using operations layer
	fmt.Printf("Creating session '%s'...\n", sessionName)

	sessionOps := operations.NewSessionOperations(sm)
	if err := sessionOps.CreateSession(sessionName); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Success message
	fmt.Printf("✅ Session '%s' created successfully!\n", sessionName)
	fmt.Printf("🔗 Attaching to session...\n")

	// Attach to the newly created session
	tmuxSessionName := fmt.Sprintf("cwt-%s", sessionName)
	return attachToTmuxSession(tmuxSessionName)
}

func promptForSessionName() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Enter session name: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		sessionName := strings.TrimSpace(input)
		if sessionName == "" {
			fmt.Println("Session name cannot be empty. Please try again.")
			continue
		}

		return sessionName, nil
	}
}

// attachToTmuxSession attaches to the specified tmux session using exec.
// This function replaces the current process with tmux attach-session,
// so no code after the syscall.Exec call will execute.
func attachToTmuxSession(tmuxSessionName string) error {
	// Validate input
	if tmuxSessionName == "" {
		return fmt.Errorf("tmux session name cannot be empty")
	}

	// Find tmux in PATH
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	// Verify session exists before attempting attach
	if err := verifyTmuxSessionExists(tmuxSessionName); err != nil {
		return fmt.Errorf("tmux session not found: %w", err)
	}

	// Use exec to replace current process with tmux attach
	args := []string{"tmux", "attach-session", "-t", tmuxSessionName}
	err = syscall.Exec(tmuxPath, args, os.Environ())
	if err != nil {
		return fmt.Errorf("failed to exec tmux: %w", err)
	}

	// This point should never be reached if exec succeeds
	panic("syscall.Exec returned unexpectedly")
}

// verifyTmuxSessionExists checks if the specified tmux session exists
func verifyTmuxSessionExists(sessionName string) error {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("session '%s' does not exist", sessionName)
	}
	return nil
}
