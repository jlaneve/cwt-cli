package operations

import (
	"fmt"
	"strings"
	"time"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// StatusFormat defines how to format session status information
type StatusFormat struct {
	// Add configuration options if needed in the future
}

// NewStatusFormat creates a new StatusFormat instance
func NewStatusFormat() *StatusFormat {
	return &StatusFormat{}
}

// FormatTmuxStatus formats the tmux status with appropriate emoji and color
func (f *StatusFormat) FormatTmuxStatus(isAlive bool) string {
	if isAlive {
		return "🟢 alive"
	}
	return "🔴 dead"
}

// FormatClaudeStatus formats the Claude status with appropriate emoji and details
func (f *StatusFormat) FormatClaudeStatus(claudeStatus types.ClaudeStatus) string {
	switch claudeStatus.State {
	case types.ClaudeWorking:
		return "🔵 working"
	case types.ClaudeWaiting:
		if claudeStatus.StatusMessage != "" {
			return fmt.Sprintf("⏸️ %s", claudeStatus.StatusMessage)
		}
		return "⏸️ waiting"
	case types.ClaudeComplete:
		return "✅ complete"
	case types.ClaudeIdle:
		return "🟡 idle"
	case types.ClaudeUnknown:
		return "❓ unknown"
	default:
		return "❓ unknown"
	}
}

// FormatGitStatus formats the git status with file change information
func (f *StatusFormat) FormatGitStatus(gitStatus types.GitStatus) string {
	if !gitStatus.HasChanges {
		return "🟢 clean"
	}

	parts := []string{}

	if len(gitStatus.ModifiedFiles) > 0 {
		if len(gitStatus.ModifiedFiles) == 1 {
			parts = append(parts, "1 file")
		} else {
			parts = append(parts, fmt.Sprintf("%d files", len(gitStatus.ModifiedFiles)))
		}
	}

	if len(gitStatus.UntrackedFiles) > 0 {
		if len(gitStatus.UntrackedFiles) == 1 {
			parts = append(parts, "1 untracked")
		} else {
			parts = append(parts, fmt.Sprintf("%d untracked", len(gitStatus.UntrackedFiles)))
		}
	}

	if len(parts) == 0 {
		return "🟡 changes"
	}

	return fmt.Sprintf("🟡 %s", strings.Join(parts, ", "))
}

// FormatActivity formats the last activity time
func (f *StatusFormat) FormatActivity(lastActivity time.Time) string {
	if lastActivity.IsZero() {
		return "never"
	}

	duration := time.Since(lastActivity)
	return f.FormatDuration(duration) + " ago"
}

// FormatDuration formats a duration in a human-readable way
func (f *StatusFormat) FormatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}

// FormatSessionSummary creates a one-line summary of a session's status
func (f *StatusFormat) FormatSessionSummary(session types.Session) string {
	tmux := f.FormatTmuxStatus(session.IsAlive)
	claude := f.FormatClaudeStatus(session.ClaudeStatus)
	git := f.FormatGitStatus(session.GitStatus)
	activity := f.FormatActivity(session.LastActivity)

	return fmt.Sprintf("tmux: %s | claude: %s | git: %s | activity: %s",
		tmux, claude, git, activity)
}

// FormatSessionList formats a list of sessions for display
func (f *StatusFormat) FormatSessionList(sessions []types.Session, detailed bool) string {
	if len(sessions) == 0 {
		return "No sessions found."
	}

	var result strings.Builder

	for i, session := range sessions {
		if i > 0 {
			result.WriteString("\n")
		}

		result.WriteString(fmt.Sprintf("📂 %s", session.Core.Name))

		if detailed {
			result.WriteString(fmt.Sprintf(" (%s)", session.Core.ID))
			result.WriteString(fmt.Sprintf("\n   %s", f.FormatSessionSummary(session)))
			result.WriteString(fmt.Sprintf("\n   📁 %s", session.Core.WorktreePath))
		} else {
			result.WriteString(fmt.Sprintf(" - %s", f.FormatSessionSummary(session)))
		}
	}

	return result.String()
}
