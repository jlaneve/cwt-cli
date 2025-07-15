package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// newHookCmd creates the hidden hook command for Claude Code integration
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "__hook [session-id] [event-type]",
		Aliases: []string{"hook"}, // Add alias for troubleshooting
		Hidden:  true,             // Don't show in help output
		Short:   "Internal hook handler for Claude Code events",
		Long: `This is an internal command used by Claude Code hooks.
It receives session events and updates session state files.

This command is automatically configured when creating sessions
and should not be called manually.`,
		Args: cobra.MinimumNArgs(2),
		RunE: runHookCmd,
	}

	return cmd
}

func runHookCmd(cmd *cobra.Command, args []string) error {
	sessionID := args[0]
	eventType := args[1]

	// Debug: log what we received (comment out in production)
	// fmt.Fprintf(os.Stderr, "Hook called with args: %v\n", args)

	// Read hook data from stdin (Claude passes JSON data)
	var eventData map[string]interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&eventData); err != nil {
		// If no JSON data, use empty map
		eventData = make(map[string]interface{})
	}

	// Extract message if present
	var lastMessage string
	if msg, ok := eventData["message"].(string); ok {
		lastMessage = msg
	}

	// Create session state update
	state := &types.SessionState{
		SessionID:     sessionID,
		ClaudeState:   types.ParseClaudeStateFromEvent(eventType, eventData),
		LastEvent:     eventType,
		LastEventTime: time.Now(),
		LastEventData: eventData,
		LastMessage:   lastMessage,
		LastUpdated:   time.Now(),
	}

	// Save session state (using .cwt as default data directory)
	dataDir := ".cwt"
	if err := types.SaveSessionState(dataDir, state); err != nil {
		return fmt.Errorf("failed to save session state: %w", err)
	}

	return nil
}
