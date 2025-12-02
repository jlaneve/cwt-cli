package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/operations"
)

var dangerousMode bool

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

	cmd.Flags().BoolVar(&dangerousMode, "dangerous", false, "Run Claude with --dangerously-skip-permissions (no confirmation prompts)")

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
	if dangerousMode {
		fmt.Printf("Creating session '%s' (dangerous mode)...\n", sessionName)
	} else {
		fmt.Printf("Creating session '%s'...\n", sessionName)
	}

	sessionOps := operations.NewSessionOperations(sm)
	if err := sessionOps.CreateSession(sessionName, dangerousMode); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Success message
	if dangerousMode {
		fmt.Printf("✅ Session '%s' created successfully! (dangerous mode enabled)\n", sessionName)
	} else {
		fmt.Printf("✅ Session '%s' created successfully!\n", sessionName)
	}

	// Attach to the newly created session
	tmuxSessionName := fmt.Sprintf("cwt-%s", sessionName)
	return operations.AttachToTmuxSession(sessionName, tmuxSessionName)
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
