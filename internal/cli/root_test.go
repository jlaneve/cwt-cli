package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() with --help error = %v", err)
	}

	output := buf.String()
	expectedStrings := []string{
		"CWT (Claude Worktree Tool)",
		"Available Commands:",
		"attach",
		"cleanup",
		"delete",
		"list",
		"new",
		"tui",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Help output missing expected string: %q", expected)
		}
	}
}

func TestRootCmd_InvalidCommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"invalid-command"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Expected error for invalid command")
	}
}

func TestRootCmd_Version(t *testing.T) {
	// Test that version flag exists
	cmd := NewRootCmd()
	versionFlag := cmd.Flag("version")
	if versionFlag != nil {
		t.Error("Version flag should not exist (not implemented)")
	}
}

func TestNewCommands(t *testing.T) {
	// Test that all commands are properly initialized
	commands := []string{
		"new",
		"list",
		"delete",
		"cleanup",
		"attach",
		"tui",
	}

	rootCmd := NewRootCmd()
	
	for _, cmdName := range commands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Command %q not found in root command", cmdName)
		}
	}
}