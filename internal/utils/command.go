package utils

import (
	"os"
	"os/exec"
	"strings"
)

// GetCWTCommand returns the appropriate command to run CWT based on context
func GetCWTCommand() []string {
	// Get the current executable path
	executable, err := os.Executable()
	if err == nil {
		// Check if we're running from a temporary directory (go run)
		if strings.Contains(executable, "go-build") || strings.Contains(executable, "/tmp/") {
			// We're running via 'go run'
			return []string{"go", "run", "cmd/cwt/main.go"}
		}
		
		// Check if the executable name suggests it's a built binary
		if strings.HasSuffix(executable, "/cwt") || strings.HasSuffix(executable, "\\cwt.exe") {
			// Use the actual executable path
			return []string{executable}
		}
	}
	
	// Fallback: check if running from source by examining os.Args
	if len(os.Args) >= 3 && strings.Contains(os.Args[0], "go") && 
		strings.Contains(strings.Join(os.Args[1:3], " "), "run") {
		// We're running via 'go run cmd/cwt/main.go'
		return []string{"go", "run", "cmd/cwt/main.go"}
	}
	
	// Check if there's a 'cwt' binary in the current directory
	if _, err := os.Stat("./cwt"); err == nil {
		return []string{"./cwt"}
	}
	
	// Check if 'cwt' is in PATH
	if _, err := exec.LookPath("cwt"); err == nil {
		return []string{"cwt"}
	}
	
	// Final fallback: try go run (assume we're in project directory)
	return []string{"go", "run", "cmd/cwt/main.go"}
}

// ExecuteCWTCommand executes a CWT command with proper binary detection
func ExecuteCWTCommand(subcommand string, args ...string) error {
	baseCmd := GetCWTCommand()
	fullArgs := append(baseCmd[1:], subcommand)
	fullArgs = append(fullArgs, args...)
	
	cmd := exec.Command(baseCmd[0], fullArgs...)
	return cmd.Run()
}

// GetCWTExecutablePath returns just the executable path/command for CWT
func GetCWTExecutablePath() string {
	cmd := GetCWTCommand()
	if len(cmd) == 1 {
		return cmd[0]
	}
	// For go run, return the full command as a string
	return strings.Join(cmd, " ")
}