# CWT Future Ideas - Worktree Lifecycle Management

This document outlines future enhancements to CWT that focus on managing the lifecycle of changes from worktrees back to the main branch. These features would transform CWT from a session manager into a complete branch lifecycle orchestrator.

## Overview

Currently, CWT excels at creating isolated Claude Code sessions with git worktrees. The next evolution is managing how those changes flow back into the main development workflow. This involves four key scenarios:

1. **Testing worktree changes** - Switch to worktree branch for testing
2. **Integrating completed work** - Merge changes back to main branch
3. **Publishing branches** - Commit and push worktree branches
4. **Monitoring change status** - Track what's changed across all sessions

## Core Lifecycle Commands

### 1. Branch Switching (`cwt switch`)

**Purpose**: Temporarily switch your main workspace to a session's branch for testing or manual work.

```bash
# Switch main branch to session's branch
cwt switch session-name

# Return to previous branch
cwt switch --back

# Interactive branch switcher
cwt switch
```

**Implementation Details:**
- Track previous branch state for easy reversal
- Validate no uncommitted changes before switching
- Update session metadata to mark branch as "checked out"
- Integrate with TUI dashboard to show current branch state

**Acceptance Criteria:**
- Can switch to any session branch from main workspace
- Previous branch is tracked and restorable
- Shows clear warnings if uncommitted changes exist
- TUI reflects current branch state visually

### 2. Merge Workflows (`cwt merge`)

**Purpose**: Safely integrate session changes back to target branches with conflict resolution.

```bash
# Interactive merge to current branch
cwt merge session-name

# Merge to specific target branch
cwt merge session-name --target main

# Squash merge for clean history
cwt merge session-name --squash

# Preview merge without executing
cwt merge session-name --dry-run
```

**Implementation Details:**
- Pre-merge validation (no conflicts, target branch clean)
- Interactive conflict resolution workflow
- Rollback capability if merge fails
- Integration with TUI for visual merge status
- Support for different merge strategies (merge commit, squash, rebase)

**Acceptance Criteria:**
- Handles merge conflicts gracefully with clear resolution steps
- Supports multiple merge strategies
- Provides rollback if merge fails
- Shows preview of changes before merging
- Updates session status after successful merge

### 3. Publishing Workflows (`cwt publish`)

**Purpose**: Commit all session changes and publish the branch for collaboration or backup.

```bash
# Commit all changes + push branch
cwt publish session-name

# Push as draft PR (if GitHub CLI available)
cwt publish session-name --draft

# Create PR automatically
cwt publish session-name --pr

# Commit only, no push
cwt publish session-name --local
```

**Implementation Details:**
- Auto-generate meaningful commit messages based on Claude's work
- Integration with `gh` CLI for PR creation
- Support for conventional commit formats
- Pre-commit hook integration
- Automatic branch pushing with upstream tracking

**Acceptance Criteria:**
- Generates descriptive commit messages automatically
- Handles authentication for pushing
- Creates PRs when requested
- Respects pre-commit hooks and linting
- Updates session status to "published"

### 4. Enhanced Status & Diff (`cwt status`, `cwt diff`)

**Purpose**: Comprehensive view of changes across all sessions with rich diff capabilities.

```bash
# Show all sessions with change status
cwt status

# Detailed diff for specific session
cwt diff session-name

# Open diff in external viewer
cwt diff session-name --web

# Compare session against specific branch
cwt diff session-name --against main

# Summary of all changes across sessions
cwt status --summary
```

**Implementation Details:**
- Rich diff rendering with syntax highlighting
- Integration with external diff tools (VSCode, GitKraken, etc.)
- Change statistics (files modified, lines added/removed)
- Dependency analysis between sessions
- Export diff to various formats (patch, markdown, HTML)

**Acceptance Criteria:**
- Shows comprehensive change overview across all sessions
- Renders diffs with proper syntax highlighting
- Integrates with external diff viewers
- Provides change statistics and summaries
- Shows relationships between session changes

## TUI Dashboard Enhancements

### Session List Improvements

**Additional Status Indicators:**
- 📤 **Published**: Branch has been pushed to remote
- 🔀 **Merged**: Changes have been merged to target branch
- 🔍 **Checked Out**: Branch is currently checked out in main workspace
- ⚡ **Dirty**: Worktree has uncommitted changes
- 🔗 **Dependencies**: Session depends on or blocks other sessions

**Enhanced Metadata Display:**
- Change summary (files modified, +/- lines)
- Target branch for merging
- Last commit hash and message
- Remote tracking status
- Merge conflict indicators

### New TUI Key Bindings

```
s - Switch to session branch
m - Merge session to current branch
u - Publish (commit + push) session
t - Toggle between detailed/compact view
/ - Search/filter sessions
n - Create new session from current context
```

### Split-Panel Views

**Diff Panel Enhancements:**
- Syntax-highlighted diff rendering
- Side-by-side vs unified diff views
- File tree navigation
- Search within diffs
- Export diff options

**New Status Panel:**
- Git graph showing branch relationships
- Change timeline across sessions
- Conflict detection and warnings
- Dependency visualization

## Advanced Workflow Features

### 5. Session Dependencies

**Purpose**: Manage dependencies between sessions for complex multi-part features.

```bash
# Create session based on another session
cwt new feature-part-2 --base feature-part-1

# Show dependency graph
cwt deps

# Update dependent sessions when base changes
cwt deps --update feature-part-1
```

### 6. Batch Operations

**Purpose**: Perform operations across multiple sessions efficiently.

```bash
# Publish all completed sessions
cwt publish --all --status complete

# Merge multiple sessions with dependency resolution
cwt merge session-1 session-2 session-3 --resolve-deps

# Archive old sessions
cwt archive --older-than 7d
```

### 7. Quality Gates

**Purpose**: Automated quality checks before merging or publishing.

```bash
# Run tests in session context
cwt test session-name

# Lint and format session changes
cwt lint session-name --fix

# Generate AI code review
cwt review session-name
```

## Implementation Architecture

### Extended Session State Machine

```
Created → Working → [Testing] → [Review] → [Published] → [Merged] → Archived
                     ↑    ↓           ↑        ↓
                  Checked Out    Conflicts  Rejected
```

### New Manager Components

```go
// internal/lifecycle/manager.go
type LifecycleManager struct {
    sessionManager *session.Manager
    gitManager     *git.Manager
    publishManager *PublishManager
    mergeManager   *MergeManager
}

// internal/publish/manager.go
type PublishManager struct {
    prProvider PRProvider // GitHub, GitLab, etc.
    hooks      []PrePublishHook
}

// internal/merge/manager.go
type MergeManager struct {
    conflictResolver ConflictResolver
    strategies       []MergeStrategy
}
```

### Configuration Extensions

```yaml
# .cwt/config.yaml
lifecycle:
  default_merge_strategy: "squash"  # merge, squash, rebase
  auto_publish: false
  pr_template: ".github/pull_request_template.md"
  quality_gates:
    - "test"
    - "lint"
    - "review"

providers:
  github:
    create_draft_prs: true
    auto_assign_reviewers: true
  
keybindings:
  switch: "s"
  merge: "m"
  publish: "u"
  diff_external: "shift+d"
```

## Success Metrics

### User Experience
- **Reduced context switching**: From 5+ git commands to 1 CWT command
- **Faster iteration**: Cut test-iterate cycle from 2min to 30sec
- **Error prevention**: Catch conflicts before they reach main branch
- **Workflow consistency**: Standardized approach across team

### Technical Metrics
- **Merge conflict rate**: Reduce conflicts by 50% through early detection
- **Branch lifecycle time**: Track time from creation to merge
- **Quality gate pass rate**: Monitor automated check success rates
- **Recovery time**: How quickly failed merges can be recovered

## Rollout Strategy

### Phase 1: Core Commands (Week 1-2)
- Implement `cwt switch` and `cwt merge` with basic functionality
- Add TUI key bindings for new commands
- Basic conflict detection and warnings

### Phase 2: Publishing Integration (Week 3)
- Implement `cwt publish` with GitHub integration
- Enhanced `cwt status` and `cwt diff` commands
- External diff tool integration

### Phase 3: Advanced Features (Week 4-5)
- Session dependencies and batch operations
- Quality gates and automated testing
- Advanced TUI features and visualizations

### Phase 4: Polish & Documentation (Week 6)
- Comprehensive error handling and recovery
- User documentation and tutorials
- Performance optimization and testing

## Migration Path

**Backward Compatibility**: All existing CWT functionality remains unchanged. New commands are additive.

**Gradual Adoption**: Users can adopt new features incrementally:
1. Start with `cwt switch` for testing
2. Add `cwt merge` for integration
3. Use `cwt publish` for collaboration
4. Explore advanced features as needed

**Team Integration**: New features designed to work with existing git workflows and team practices.