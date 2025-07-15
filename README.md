# CWT - Claude Worktree Tool

A control plane for managing multiple Claude Code sessions with isolated git worktrees.

## The Problem

Claude Code is incredibly powerful, so you naturally want to run multiple sessions simultaneously - one for each feature, bug fix, or experiment. But managing multiple sessions manually becomes painful:

- **Context switching hell**: Jumping between tmux sessions with `Ctrl+b n` and trying to remember which session is working on what
- **No visibility**: Can't see at a glance which sessions need input, which are stuck, or which have completed work
- **Branch conflicts**: Multiple sessions on the same branch leads to merge conflicts and lost work
- **Lost work**: Easy to forget about sessions or accidentally overwrite changes

## The Solution

CWT gives you comprehensive tooling for managing multiple Claude Code sessions with **automatic isolation** via git worktrees. You get:

- 🖥️ **TUI Dashboard**: Interactive terminal interface for session management
- 🌲 **Automatic Git Worktrees**: Each session works on its own branch in isolation
- ⚡ **Rich CLI Commands**: Create, attach, switch, merge, and publish sessions
- 📊 **Status Monitoring**: Track session progress and git changes
- 🔄 **Complete Workflow**: From creation to merge, manage entire session lifecycle

## Quick Start

```bash
# Create a new session
cwt new
# → Prompts for session name
# → Creates git worktree and tmux session
# → Starts Claude Code

# View dashboard
cwt
# → Shows all sessions with status indicators
# → Interactive session management

# List sessions
cwt list
# → Shows all sessions and their status

# Check detailed status
cwt status
# → Comprehensive status with change details
```

## Core Workflows

### Creating Sessions

```bash
# Interactive (recommended)
cwt new
Enter session name: auth-system

# Direct
cwt new auth-system
```

### Session Management Commands

```bash
# Session lifecycle
cwt new feature-name                               # Create new session
cwt attach feature-name                            # Attach to session's tmux
cwt delete feature-name                            # Delete session completely
cwt cleanup                                        # Remove orphaned resources

# Working with session changes
cwt switch feature-name                            # Switch to session's branch
cwt diff feature-name                              # Show session's changes
cwt publish feature-name                           # Commit and push changes
cwt merge feature-name                             # Merge session to main

# Monitoring and information
cwt list                                           # List all sessions
cwt status                                         # Detailed status of all sessions
cwt tui                                           # Interactive dashboard
```

### Session Status Indicators

- **Active**: tmux session is running with Claude Code
- **Clean/Modified**: Git status of session's worktree  
- **Ahead/Behind**: Branch status relative to base branch
- **Published**: Branch has been pushed to remote

## Why Use CWT?

### Before CWT

```bash
# Terminal 1
cd ~/project
git checkout -b feature-auth
tmux new-session -s auth
claude-code
# work on auth...

# Terminal 2  
cd ~/project
git stash  # oh no, conflicts with auth changes
git checkout -b feature-payments  
tmux new-session -s payments
claude-code
# work on payments...

# Later: which session was which? what's the status?
tmux list-sessions  # cryptic output
git status  # which branch am I on?
```

### With CWT

```bash
cwt new auth-system
cwt new payment-flow
cwt status  # see both sessions, their progress, and changes
cwt         # interactive dashboard view
```

### Key Benefits

- **Parallel Development**: Work on multiple features simultaneously without conflicts
- **Visual Status**: Immediately see which sessions need attention
- **Zero Context Loss**: Each session maintains its own environment and history  
- **Quick Testing**: Checkout any session's branch to test or run code locally
- **Clean Git History**: Each session produces focused commits on isolated branches
- **Session Recovery**: Crashed sessions don't affect other work

## Real-World Usage

### Feature Development

```bash
cwt new user-profiles
# → Creates isolated workspace for profile feature
# → You continue with other work

cwt status  # check progress later
# → See session status and any changes made
cwt attach user-profiles  # interact with Claude directly
# → Press Ctrl+b d to detach from session

# When ready to integrate changes:
cwt switch user-profiles   # test the feature locally
cwt publish user-profiles  # commit and push changes
```

### Bug Fixes

```bash
cwt new login-bug
cwt new search-perf

# Work on other things while Claude handles both
cwt status  # check progress
# → See login-bug has changes ready for review
# → See search-perf session is still active
```

### Experimentation

```bash
cwt new react-migration
cwt new db-optimization

# Let Claude explore both approaches in parallel
# Review results with cwt diff and choose which direction to pursue
```

## Installation

```bash
# Build from source
git clone https://github.com/jlaneve/cwt-cli
cd cwt-cli
go build -o cwt cmd/cwt/main.go

# Add to PATH (optional)
sudo mv cwt /usr/local/bin/
```

## Requirements

- Go >= 1.23 (for building)
- tmux >= 3.2
- git >= 2.25 (for worktree support)
- Claude Code CLI installed and in PATH

## How It Works

1. **Git Worktrees**: Each session gets its own directory with an isolated branch
2. **Tmux Sessions**: Claude Code runs in named tmux sessions for easy management
3. **State Tracking**: CWT monitors session status and git changes
4. **TUI Interface**: Real-time dashboard built with terminal UI components

## Development

### Setting Up Pre-commit Hooks

This project uses pre-commit hooks to catch linting and formatting issues before they reach CI:

```bash
# Install pre-commit (one-time setup)
pip install pre-commit

# Install the git hook scripts
pre-commit install

# (Optional) Run hooks on all files
pre-commit run --all-files
```

### Available Make Targets

```bash
make help      # Show available targets
make test      # Run tests
make build     # Build binary
make lint      # Run golangci-lint (requires installation)
make clean     # Clean build artifacts
```

### Pre-commit Hooks

The pre-commit configuration includes:
- **gofmt**: Code formatting
- **goimports**: Import organization 
- **go mod tidy**: Dependency management
- **go vet**: Basic linting
- **golangci-lint**: Enhanced linting (see `.golangci.yml`)
- **General checks**: trailing whitespace, file endings, YAML/JSON validation

## Contributing

See [workflows.md](workflows.md) for detailed workflow documentation.

## License

MIT

