package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/tui"
)

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI dashboard",
		Long: `Launch the TUI (Terminal User Interface) dashboard for CWT.

The TUI provides:
- Real-time session status monitoring
- Interactive session management
- Visual indicators for tmux, git, and Claude status
- Quick session creation and deletion
- Session attachment capabilities`,
		Aliases: []string{"ui", "dashboard"},
		RunE:    runTuiCmd,
	}

	return cmd
}

func runTuiCmd(cmd *cobra.Command, args []string) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	// Note: StateManager will be closed by the TUI when it exits

	// Launch TUI
	if err := tui.Run(sm); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
