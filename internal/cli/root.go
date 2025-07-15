package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
)

var (
	dataDir    string
	baseBranch string
)

// NewRootCmd creates the root command for the CWT CLI
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "cwt",
		Short: "Claude Worktree Tool - Manage multiple Claude Code sessions with isolated git worktrees",
		Long: `CWT (Claude Worktree Tool) is a control plane for managing multiple Claude Code sessions
with isolated git worktrees. Think of it as a project management system where you are 
the engineering manager and Claude Code sessions are your engineers working on isolated tasks.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When no subcommand is provided, launch TUI
			return runTuiCmd(cmd, args)
		},
	}

	// Set custom help template
	rootCmd.SetHelpTemplate(getCustomHelpTemplate())

	// Global flags
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", ".cwt", "Directory for storing session data")
	rootCmd.PersistentFlags().StringVar(&baseBranch, "base-branch", "main", "Base branch for creating worktrees")

	// Add subcommands with annotations for grouping
	
	// Session Management
	sessionMgmt := []*cobra.Command{
		addAnnotation(newNewCmd(), "session-mgmt"),
		addAnnotation(newAttachCmd(), "session-mgmt"),
		addAnnotation(newDeleteCmd(), "session-mgmt"),
		addAnnotation(newCleanupCmd(), "session-mgmt"),
	}
	
	// Session Workflow (Branch Lifecycle)
	sessionWorkflow := []*cobra.Command{
		addAnnotation(newSwitchCmd(), "session-workflow"),
		addAnnotation(newMergeCmd(), "session-workflow"),
		addAnnotation(newPublishCmd(), "session-workflow"),
	}
	
	// Information & Monitoring
	info := []*cobra.Command{
		addAnnotation(newListCmd(), "info"),
		addAnnotation(newStatusCmd(), "info"),
		addAnnotation(newDiffCmd(), "info"),
	}
	
	// Interface & Utilities
	interface_utils := []*cobra.Command{
		addAnnotation(newTuiCmd(), "interface"),
		addAnnotation(newFixHooksCmd(), "interface"),
	}
	
	// Hidden/Internal commands (no annotation needed)
	hidden := []*cobra.Command{
		newHookCmd(), // Hidden internal command
	}
	
	// Add all commands
	for _, cmd := range sessionMgmt {
		rootCmd.AddCommand(cmd)
	}
	for _, cmd := range sessionWorkflow {
		rootCmd.AddCommand(cmd)
	}
	for _, cmd := range info {
		rootCmd.AddCommand(cmd)
	}
	for _, cmd := range interface_utils {
		rootCmd.AddCommand(cmd)
	}
	for _, cmd := range hidden {
		rootCmd.AddCommand(cmd)
	}

	return rootCmd
}

// createStateManager creates a StateManager with the current configuration
func createStateManager() (*state.Manager, error) {
	config := state.Config{
		DataDir:    dataDir,
		BaseBranch: baseBranch,
		// Use real checkers (default behavior)
	}

	sm := state.NewManager(config)

	// Validate git repository by trying to derive sessions
	_, err := sm.DeriveFreshSessions()
	if err != nil {
		// Try to provide helpful error message
		if err.Error() == "not a git repository" {
			return nil, fmt.Errorf("current directory is not a git repository. Please run 'git init' first")
		}
		if err.Error() == "repository has no commits" {
			return nil, fmt.Errorf("git repository has no commits. Please make an initial commit first")
		}
		// Return original error for other cases
		return nil, err
	}

	return sm, nil
}

// addAnnotation adds a group annotation to a command
func addAnnotation(cmd *cobra.Command, group string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["group"] = group
	return cmd
}

// getCustomHelpTemplate returns a custom help template with organized command groups
func getCustomHelpTemplate() string {
	return `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}{{if .HasAvailableSubCommands}}

Session Management:{{range .Commands}}{{if and (eq .Annotations.group "session-mgmt") .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

Session Workflow (Branch Lifecycle):{{range .Commands}}{{if and (eq .Annotations.group "session-workflow") .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

Information & Monitoring:{{range .Commands}}{{if and (eq .Annotations.group "info") .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

Interface & Utilities:{{range .Commands}}{{if and (eq .Annotations.group "interface") .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

Other Commands:{{range .Commands}}{{if and (or (not .Annotations.group) (eq .Name "help") (eq .Name "completion")) .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}

Workflow Examples:
  cwt new my-feature "Add user authentication"     # Create session
  cwt attach my-feature                           # Work on the feature
  cwt status --summary                            # Check overall progress
  cwt switch my-feature                          # Test changes locally
  cwt publish my-feature                         # Commit and push
  cwt merge my-feature                           # Merge to main branch
  cwt tui                                        # Use interactive dashboard
`
}

// Execute runs the root command
func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}