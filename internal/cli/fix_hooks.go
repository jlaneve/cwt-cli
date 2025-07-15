package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newFixHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix-hooks",
		Short: "Fix broken hook paths in existing sessions",
		Long: `Auto-detect and fix broken hook paths in Claude settings.json files.

This command scans all existing sessions and updates any invalid executable
paths in their Claude hook configurations. This is useful when:
- Sessions were created with 'go run' and have temp executable paths
- The cwt binary was moved or renamed
- Hook paths are pointing to non-existent executables`,
		RunE: runFixHooksCmd,
	}

	return cmd
}

func runFixHooksCmd(cmd *cobra.Command, args []string) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	// Get the correct cwt executable path
	correctPath := getCwtExecutablePath()

	fixed := 0
	for _, session := range sessions {
		settingsPath := filepath.Join(session.Core.WorktreePath, "settings.json")

		if updated, err := fixSettingsFile(settingsPath, session.Core.ID, correctPath); err != nil {
			fmt.Printf("⚠️  Failed to fix hooks for session '%s': %v\n", session.Core.Name, err)
		} else if updated {
			fmt.Printf("✅ Fixed hooks for session '%s'\n", session.Core.Name)
			fixed++
		}
	}

	if fixed == 0 {
		fmt.Println("All session hooks are already correctly configured.")
	} else {
		fmt.Printf("\n🎉 Fixed hooks for %d session(s)\n", fixed)
	}

	return nil
}

// fixSettingsFile updates the settings.json file with correct hook paths
func fixSettingsFile(settingsPath, sessionID, correctPath string) (bool, error) {
	// Check if settings file exists
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return false, fmt.Errorf("settings.json not found")
	}

	// Read current settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, fmt.Errorf("failed to read settings file: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("failed to parse settings JSON: %w", err)
	}

	// Check if hooks exist
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		// No hooks section, create it
		hooks = make(map[string]interface{})
		settings["hooks"] = hooks
	}

	// Check if any hooks need updating
	needsUpdate := false
	expectedHooks := map[string]interface{}{
		"Notification": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s notification", correctPath, sessionID),
					},
				},
			},
		},
		"Stop": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s stop", correctPath, sessionID),
					},
				},
			},
		},
		"PreToolUse": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s pre_tool_use", correctPath, sessionID),
					},
				},
			},
		},
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s post_tool_use", correctPath, sessionID),
					},
				},
			},
		},
		"SubagentStop": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s subagent_stop", correctPath, sessionID),
					},
				},
			},
		},
		"PreCompact": []map[string]interface{}{
			{
				"matcher": "",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": fmt.Sprintf("%s __hook %s pre_compact", correctPath, sessionID),
					},
				},
			},
		},
	}

	for hookName, expectedHook := range expectedHooks {
		currentHook, exists := hooks[hookName]
		if !exists {
			needsUpdate = true
			hooks[hookName] = expectedHook
		} else {
			// Check if current hook matches expected structure
			expectedJSON, _ := json.Marshal(expectedHook)
			currentJSON, _ := json.Marshal(currentHook)
			if string(expectedJSON) != string(currentJSON) {
				needsUpdate = true
				hooks[hookName] = expectedHook
			}
		}
	}

	if !needsUpdate {
		return false, nil // No changes needed
	}

	// Write updated settings
	updatedData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to marshal updated settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, updatedData, 0644); err != nil {
		return false, fmt.Errorf("failed to write updated settings: %w", err)
	}

	return true, nil
}

// getCwtExecutablePath duplicates the logic from state manager for consistency
func getCwtExecutablePath() string {
	// First, try to find cwt in PATH (most reliable for installed binaries)
	if path, err := exec.LookPath("cwt"); err == nil {
		return path
	}

	// Check if we're running from go run (has temp executable path)
	if execPath, err := os.Executable(); err == nil {
		// If it's a temp path from go run, use "go run cmd/cwt/main.go" instead
		if strings.Contains(execPath, "go-build") || strings.Contains(execPath, "/tmp/") {
			// Check if we're in the cwt project directory
			if _, err := os.Stat("cmd/cwt/main.go"); err == nil {
				return "go run cmd/cwt/main.go"
			}
		} else {
			// It's a real executable path
			return execPath
		}
	}

	// Final fallback to "cwt" in PATH
	return "cwt"
}
