package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jlaneve/cwt-cli/internal/state"
)

// Run starts the TUI with the given state manager, creating a seamless loop
func Run(stateManager *state.Manager) error {
	for {
		// Create the TUI model
		model, err := NewModel(stateManager)
		if err != nil {
			return fmt.Errorf("failed to create TUI model: %w", err)
		}

		// Configure the program
		p := tea.NewProgram(
			model,
			tea.WithAltScreen(),       // Use alternate screen buffer
			tea.WithMouseCellMotion(), // Enable mouse support
		)

		// Run the program
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		// Check if we need to attach to a session after TUI exit
		if m, ok := finalModel.(Model); ok {
			if sessionName := m.GetAttachOnExit(); sessionName != "" {
				// Create logger for this function (reuse same log file)
				logFile, err := os.OpenFile("cwt-tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
				if err == nil {
					logger := log.New(logFile, "[TUI-DEBUG] ", log.LstdFlags|log.Lshortfile)
					logger.Printf("Run: TUI exited with attachOnExit: %s", sessionName)
					logger.Printf("Run: Calling attachToTmuxSession")
					logFile.Close()
				}

				// Attach to tmux session
				if err := attachToTmuxSession(sessionName); err != nil {
					return err
				}

				// When tmux exits, show transition message and restart TUI
				fmt.Println("\n🔄 Tmux session ended. Returning to CWT dashboard...")

				// Continue the loop to restart TUI
				continue
			}
		}

		// If we get here, user quit TUI without attaching - exit the loop
		break
	}

	return nil
}

// attachToTmuxSession attaches to a tmux session
func attachToTmuxSession(sessionName string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach to tmux session '%s': %w", sessionName, err)
	}

	return nil
}
