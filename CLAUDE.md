# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# CWT Development Context

## Project Overview

CWT (Claude Worktree Tool) is a control plane for managing multiple Claude Code sessions with isolated git worktrees. Think of it as a project management system where you are the engineering manager and Claude Code sessions are your engineers working on isolated tasks.

## Repository Structure

```
cwt-cli/
├── cmd/cwt/main.go              # CLI entry point
├── internal/
│   ├── cli/                     # CLI command handlers
│   │   ├── root.go             # Root command with cobra setup
│   │   ├── new.go              # cwt new - create sessions
│   │   └── cleanup.go          # cwt cleanup - remove orphaned resources
│   ├── git/manager.go          # Git operations (hybrid go-git + shell)
│   └── session/manager.go      # Session lifecycle management
├── pkg/types/                  # Shared type definitions
│   ├── session.go             # Session and status types
│   └── config.go              # Configuration structures
├── .cwt/                       # Runtime data (created dynamically)
│   ├── sessions.json          # Session metadata persistence
│   └── worktrees/             # Git worktrees for each session
│       ├── session-1/         # Isolated workspace for session
│       └── session-2/         # Another isolated workspace
├── IMPLEMENTATION.md           # Detailed technical implementation guide
├── workflows.md               # User workflow documentation
└── README.md                  # User-facing documentation
```

## Architecture Overview

### Three-Layer Architecture
1. **CLI Layer** (`internal/cli/`): Cobra-based command handlers that orchestrate operations
2. **Business Logic** (`internal/session/`, `internal/git/`): Core session and git management  
3. **Types** (`pkg/types/`): Shared data structures and enums

### Session Lifecycle
Sessions flow through states managed by the SessionManager:
- **Creation**: SessionManager → GitManager (worktree) → tmux (claude-code process)
- **Persistence**: Session metadata stored in `.cwt/sessions.json`
- **Cleanup**: Comprehensive removal of tmux sessions, git worktrees, and metadata

### Git Integration Strategy
**Hybrid approach** due to go-git limitations:
- **go-git** (`github.com/go-git/go-git/v5`): Repository operations, status checks, commits
- **Shell commands**: Worktree management (`git worktree add/remove`) since go-git v5 doesn't support multiple worktrees
- **Isolation**: Each session gets dedicated `.cwt/worktrees/{name}/` directory with own branch

### Session State Management
The SessionManager maintains an in-memory map of sessions, synchronized with:
- **JSON persistence**: `.cwt/sessions.json` for metadata across restarts
- **tmux sessions**: Named `cwt-{session-name}` pattern for process management
- **Git worktrees**: Filesystem directories for isolated development

## Current Implementation Status

### Phase 1 Complete ✅
- `cwt new`: Creates isolated sessions with git worktrees + tmux
- `cwt cleanup`: Comprehensive cleanup of orphaned resources
- Session persistence via `.cwt/sessions.json`
- Error handling with rollback on failures

### Available Commands
```bash
cwt new [session-name]                     # Create new session  
cwt new                                     # Interactive session creation
cwt cleanup                                 # Remove orphaned sessions/worktrees
cwt --help                                  # Show available commands
```

### Planned Features
- **Phase 2**: TUI dashboard with Bubble Tea
- **Phase 3**: Claude state detection via JSONL monitoring  
- **Phase 4**: Commit/push from TUI, enhanced diff preview

## Key Implementation Patterns

### Error Handling Pattern
All operations follow consistent error wrapping:
```go
if err := operation(); err != nil {
    return fmt.Errorf("context description: %w", err)
}
```

### Resource Cleanup Pattern
Session creation uses rollback on failure:
```go
// Create resources in order
worktreePath, err := createWorktree()
if err != nil { return err }

tmuxName, err := createTmux()
if err != nil {
    removeWorktree(worktreePath)  // Cleanup on failure
    return err
}

// Save metadata last
if err := saveSession(); err != nil {
    killTmux(tmuxName)
    removeWorktree(worktreePath)
    return err
}
```

### Session Manager Pattern
The SessionManager centralizes all session operations:
- `CreateSession()`: Orchestrates git + tmux + metadata
- `LoadSessions()`: Restores state from JSON
- `SaveSessions()`: Persists state to JSON  
- `FindStaleSessions()`: Identifies cleanup candidates

## Development Commands

### Build and Run
```bash
# Run directly from source
go run cmd/cwt/main.go new "session-name"
go run cmd/cwt/main.go cleanup

# Build binary
go build -o cwt cmd/cwt/main.go

# Update dependencies
go mod tidy
```

### Testing
```bash
# Test basic functionality
go run cmd/cwt/main.go new test-session
tmux list-sessions | grep cwt    # Verify tmux session created
git worktree list               # Verify git worktree created

# Test cleanup functionality
go run cmd/cwt/main.go cleanup

# Test error scenarios
go run cmd/cwt/main.go new test-session  # Should fail if duplicate
```

### Debugging Session State
```bash
# Check session metadata
cat .cwt/sessions.json

# Check tmux sessions
tmux list-sessions

# Check git worktrees
git worktree list

# Reset everything
go run cmd/cwt/main.go cleanup
```

## Troubleshooting

### Common Issues
1. **"git repository not found"**: Ensure you're in a git repository with at least one commit
2. **"tmux not found"**: Install tmux (`brew install tmux` on macOS)
3. **"claude-code not found"**: Ensure Claude Code CLI is installed and in PATH
4. **C-q binding warnings**: Non-critical, tmux session still works normally

### Debugging Tips
- Check `.cwt/sessions.json` for session metadata
- Use `git worktree list` to see git worktrees
- Use `tmux list-sessions` to see tmux sessions
- Run cleanup command to reset state: `cwt cleanup`

## Key Design Decisions

**Hybrid Git Approach**: go-git v5 lacks worktree support, so we use shell commands for `git worktree` operations while using go-git for other repository operations.

**tmux for Process Management**: Provides robust session isolation and process management that Claude Code users are familiar with.

**JSON for Persistence**: Simple, debuggable format for session metadata without external dependencies.

## Prerequisites
- Must be run in a git repository with at least one commit
- Requires tmux >= 3.2 and claude-code CLI in PATH
- Creates `.cwt/` directory for runtime data (gitignored)