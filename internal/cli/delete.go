package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/types"
)

func newDeleteCmd() *cobra.Command {
	var force bool
	
	cmd := &cobra.Command{
		Use:   "delete [session-name]",
		Short: "Delete a session and clean up its resources",
		Long: `Delete a CWT session, removing:
- Tmux session
- Git worktree
- Session metadata

This operation cannot be undone.`,
		Aliases: []string{"del", "rm"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteCmd(args, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runDeleteCmd(args []string, force bool) error {
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
		fmt.Println("No sessions found to delete.")
		return fmt.Errorf("no sessions available to delete")
	}

	// Determine which session to delete
	var sessionToDelete *string
	var sessionID string

	if len(args) > 0 {
		// Session name provided
		sessionName := args[0]
		for _, session := range sessions {
			if session.Core.Name == sessionName {
				sessionToDelete = &sessionName
				sessionID = session.Core.ID
				break
			}
		}

		if sessionToDelete == nil {
			return fmt.Errorf("session '%s' not found", sessionName)
		}
	} else {
		// Interactive selection
		sessionName, id, err := promptForSessionSelection(sessions)
		if err != nil {
			return err
		}
		sessionToDelete = &sessionName
		sessionID = id
	}

	// Confirm deletion unless forced
	if !force {
		if !confirmDeletion(*sessionToDelete) {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	// Delete session
	fmt.Printf("Deleting session '%s'...\n", *sessionToDelete)
	
	if err := sm.DeleteSession(sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	fmt.Printf("✅ Session '%s' deleted successfully!\n", *sessionToDelete)

	return nil
}

func promptForSessionSelection(sessions []types.Session) (string, string, error) {
	if len(sessions) == 1 {
		return sessions[0].Core.Name, sessions[0].Core.ID, nil
	}

	fmt.Println("Multiple sessions found. Select one to delete:")
	for i, session := range sessions {
		status := "🔴 dead"
		if session.IsAlive {
			status = "🟢 alive"
		}
		fmt.Printf("  %d. %s (%s)\n", i+1, session.Core.Name, status)
	}

	reader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Print("Enter selection (1-" + fmt.Sprintf("%d", len(sessions)) + "): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}

		var selection int
		if _, err := fmt.Sscanf(strings.TrimSpace(input), "%d", &selection); err != nil {
			fmt.Println("Invalid input. Please enter a number.")
			continue
		}

		if selection < 1 || selection > len(sessions) {
			fmt.Printf("Invalid selection. Please enter a number between 1 and %d.\n", len(sessions))
			continue
		}

		selectedSession := sessions[selection-1]
		return selectedSession.Core.Name, selectedSession.Core.ID, nil
	}
}

func confirmDeletion(sessionName string) bool {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Printf("Are you sure you want to delete session '%s'? This cannot be undone. (y/N): ", sessionName)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response := strings.ToLower(strings.TrimSpace(input))
	return response == "y" || response == "yes"
}