package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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

	// Create session
	fmt.Printf("Creating session '%s'...\n", sessionName)

	if err := sm.CreateSession(sessionName); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Success message
	fmt.Printf("✅ Session '%s' created successfully!\n\n", sessionName)
	fmt.Printf("Next steps:\n")
	fmt.Printf("  • View all sessions: cwt list\n")
	fmt.Printf("  • Attach to session: cwt attach %s\n", sessionName)
	fmt.Printf("  • Open TUI dashboard: cwt tui\n")
	fmt.Printf("  • Work in isolated directory: cd %s/worktrees/%s\n", dataDir, sessionName)

	return nil
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
