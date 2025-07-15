package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// Checker defines the interface for git operations
type Checker interface {
	GetStatus(worktreePath string) types.GitStatus
	CreateWorktree(branchName, worktreePath string) error
	RemoveWorktree(worktreePath string) error
	IsValidRepository(repoPath string) error
	ListWorktrees() ([]WorktreeInfo, error)
	BranchExists(branchName string) bool
	CommitChanges(worktreePath, message string) error
	CheckoutBranch(branchName string) error
}

// WorktreeInfo represents information about a git worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	Bare   bool
}

// RealChecker implements Checker using actual git commands
type RealChecker struct {
	BaseBranch string // Default branch to create worktrees from
}

// NewRealChecker creates a new RealChecker
func NewRealChecker(baseBranch string) *RealChecker {
	if baseBranch == "" {
		baseBranch = "main"
	}
	return &RealChecker{BaseBranch: baseBranch}
}

// GetStatus checks the git status of a worktree
func (r *RealChecker) GetStatus(worktreePath string) types.GitStatus {
	status := types.GitStatus{}

	if !r.pathExists(worktreePath) {
		return status
	}

	// Get porcelain status
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return status
	}

	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		// No changes
		return status
	}

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		filename := line[3:]

		// Ignore Claude-related files and directories
		if strings.HasPrefix(filename, ".claude/") || filename == ".claude" {
			continue
		}

		// We have a non-Claude change
		status.HasChanges = true

		switch {
		case strings.HasPrefix(statusCode, "M") || strings.HasPrefix(statusCode, " M"):
			status.ModifiedFiles = append(status.ModifiedFiles, filename)
		case strings.HasPrefix(statusCode, "A"):
			status.AddedFiles = append(status.AddedFiles, filename)
		case strings.HasPrefix(statusCode, "??"):
			status.UntrackedFiles = append(status.UntrackedFiles, filename)
		case strings.HasPrefix(statusCode, "D") || strings.HasPrefix(statusCode, " D"):
			status.DeletedFiles = append(status.DeletedFiles, filename)
		}
	}

	// Count commits ahead of base branch
	cmd = exec.Command("git", "rev-list", "--count", fmt.Sprintf("%s..HEAD", r.BaseBranch))
	cmd.Dir = worktreePath
	output, err = cmd.Output()
	if err == nil {
		fmt.Sscanf(string(output), "%d", &status.CommitCount)
	}

	return status
}

// CreateWorktree creates a new git worktree with a new branch
func (r *RealChecker) CreateWorktree(branchName, worktreePath string) error {
	// Check if worktree directory already exists
	if r.pathExists(worktreePath) {
		return fmt.Errorf("worktree directory already exists: %s", worktreePath)
	}

	// Check if branch already exists
	if r.BranchExists(branchName) {
		return fmt.Errorf("branch '%s' already exists. Please use a different session name or delete the existing branch with: git branch -d %s", branchName, branchName)
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(worktreePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory %s: %w", parentDir, err)
	}

	// Create worktree with new branch
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, r.BaseBranch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree %s: %w\nOutput: %s", worktreePath, err, string(output))
	}

	return nil
}

// RemoveWorktree removes a git worktree
func (r *RealChecker) RemoveWorktree(worktreePath string) error {
	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree %s: %w\nOutput: %s", worktreePath, err, string(output))
	}

	return nil
}

// IsValidRepository checks if the current directory is a valid git repository
func (r *RealChecker) IsValidRepository(repoPath string) error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	// Check if repository has commits
	cmd = exec.Command("git", "rev-parse", "HEAD")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("repository has no commits: %w", err)
	}

	return nil
}

// ListWorktrees returns all git worktrees
func (r *RealChecker) ListWorktrees() ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var worktrees []WorktreeInfo
	var current WorktreeInfo

	for _, line := range lines {
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch ")
		} else if line == "bare" {
			current.Bare = true
		}
	}

	// Add final worktree if exists
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

func (r *RealChecker) pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// BranchExists checks if a git branch exists (local or remote)
func (r *RealChecker) BranchExists(branchName string) bool {
	// Check local branches first
	cmd := exec.Command("git", "branch", "--list", branchName)
	output, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return true
	}

	// Check remote branches
	cmd = exec.Command("git", "branch", "-r", "--list", "*"+branchName)
	output, err = cmd.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return true
	}

	return false
}

// CommitChanges stages all changes and commits them with the given message
func (r *RealChecker) CommitChanges(worktreePath, message string) error {
	// Stage all changes
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = worktreePath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stage changes: %w\nOutput: %s", err, string(output))
	}

	// Get git user configuration
	name, email := r.getGitUserConfig()

	// Create commit
	cmd = exec.Command("git", "commit", "-m", message)
	if name != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_AUTHOR_NAME=%s", name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", name))
	}
	if email != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", email))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email))
	}
	cmd.Dir = worktreePath
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// CheckoutBranch switches to the specified branch
func (r *RealChecker) CheckoutBranch(branchName string) error {
	cmd := exec.Command("git", "checkout", branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w\nOutput: %s", branchName, err, string(output))
	}
	return nil
}

// getGitUserConfig gets the git user name and email from config
func (r *RealChecker) getGitUserConfig() (string, string) {
	var name, email string

	// Get user name
	cmd := exec.Command("git", "config", "user.name")
	if output, err := cmd.Output(); err == nil {
		name = strings.TrimSpace(string(output))
	}

	// Get user email
	cmd = exec.Command("git", "config", "user.email")
	if output, err := cmd.Output(); err == nil {
		email = strings.TrimSpace(string(output))
	}

	// Fallback values if not configured
	if name == "" {
		name = "CWT User"
	}
	if email == "" {
		email = "user@example.com"
	}

	return name, email
}

// MockChecker implements Checker for testing
type MockChecker struct {
	Statuses    map[string]types.GitStatus
	Worktrees   map[string]bool
	ShouldFail  map[string]bool
	Delay       time.Duration
	ValidRepo   bool
}

// NewMockChecker creates a new MockChecker
func NewMockChecker() *MockChecker {
	return &MockChecker{
		Statuses:   make(map[string]types.GitStatus),
		Worktrees:  make(map[string]bool),
		ShouldFail: make(map[string]bool),
		ValidRepo:  true,
	}
}

// GetStatus returns the mocked status
func (m *MockChecker) GetStatus(worktreePath string) types.GitStatus {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	status, exists := m.Statuses[worktreePath]
	if !exists {
		return types.GitStatus{} // Empty status
	}
	return status
}

// CreateWorktree mocks worktree creation
func (m *MockChecker) CreateWorktree(branchName, worktreePath string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	if m.ShouldFail[worktreePath] {
		return fmt.Errorf("mock create failure for worktree %s", worktreePath)
	}
	m.Worktrees[worktreePath] = true
	return nil
}

// RemoveWorktree mocks worktree removal
func (m *MockChecker) RemoveWorktree(worktreePath string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	if m.ShouldFail[worktreePath] {
		return fmt.Errorf("mock remove failure for worktree %s", worktreePath)
	}
	delete(m.Worktrees, worktreePath)
	return nil
}

// IsValidRepository returns the mocked validity
func (m *MockChecker) IsValidRepository(repoPath string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	if !m.ValidRepo {
		return fmt.Errorf("mock repository validation failure")
	}
	return nil
}

// ListWorktrees returns mocked worktree list
func (m *MockChecker) ListWorktrees() ([]WorktreeInfo, error) {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	var worktrees []WorktreeInfo
	for path := range m.Worktrees {
		worktrees = append(worktrees, WorktreeInfo{
			Path:   path,
			Branch: filepath.Base(path),
		})
	}
	return worktrees, nil
}

// SetStatus sets the git status for testing
func (m *MockChecker) SetStatus(worktreePath string, status types.GitStatus) {
	m.Statuses[worktreePath] = status
}

// SetWorktreeExists sets whether a worktree exists
func (m *MockChecker) SetWorktreeExists(worktreePath string, exists bool) {
	if exists {
		m.Worktrees[worktreePath] = true
	} else {
		delete(m.Worktrees, worktreePath)
	}
}

// SetShouldFail sets whether operations should fail
func (m *MockChecker) SetShouldFail(worktreePath string, shouldFail bool) {
	m.ShouldFail[worktreePath] = shouldFail
}

// SetDelay sets a delay for all operations
func (m *MockChecker) SetDelay(delay time.Duration) {
	m.Delay = delay
}

// BranchExists returns whether a branch exists (always false for mock)
func (m *MockChecker) BranchExists(branchName string) bool {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	// Mock implementation - can be configured if needed
	return false
}

// CommitChanges mocks committing changes
func (m *MockChecker) CommitChanges(worktreePath, message string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	if m.ShouldFail[worktreePath] {
		return fmt.Errorf("mock commit failure for worktree %s", worktreePath)
	}
	return nil
}

// CheckoutBranch mocks checking out a branch
func (m *MockChecker) CheckoutBranch(branchName string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	// Mock implementation - always succeeds unless configured otherwise
	return nil
}