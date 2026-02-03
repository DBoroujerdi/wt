package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/todoengineering/wt/internal/config"
	"github.com/todoengineering/wt/internal/registry"
)

type Worktree struct {
	Name    string
	Path    string
	Branch  string
	Missing bool // true if registry entry but path doesn't exist
}

func ListWorktrees(repoName string) ([]Worktree, error) {
	worktreeDir := GetWorktreeDir(repoName)

	// Use registry.Sync to get merged data from registry and filesystem
	syncedWorktrees, err := registry.Sync(repoName, worktreeDir)
	if err != nil {
		// Fall back to filesystem-only scan if registry fails
		return listWorktreesFromFilesystem(repoName, worktreeDir)
	}

	var worktrees []Worktree
	for _, sw := range syncedWorktrees {
		worktrees = append(worktrees, Worktree{
			Name:    filepath.Base(sw.Entry.Path),
			Path:    sw.Entry.Path,
			Branch:  sw.Entry.Branch,
			Missing: sw.Status == registry.StatusMissing,
		})
	}

	sort.Slice(worktrees, func(i, j int) bool {
		return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
	})

	return worktrees, nil
}

// listWorktreesFromFilesystem is a fallback when registry is unavailable
func listWorktreesFromFilesystem(repoName, worktreeDir string) ([]Worktree, error) {
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		return []Worktree{}, nil
	}

	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read worktree directory: %w", err)
	}

	var worktrees []Worktree
	for _, entry := range entries {
		if entry.IsDir() {
			worktreePath := filepath.Join(worktreeDir, entry.Name())
			branch := GetWorktreeBranch(worktreePath)
			worktrees = append(worktrees, Worktree{
				Name:   entry.Name(),
				Path:   worktreePath,
				Branch: branch,
			})
		}
	}

	sort.Slice(worktrees, func(i, j int) bool {
		return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
	})

	return worktrees, nil
}

func WorktreeExistsForBranch(repoName, branchName string) (bool, *Worktree) {
	// First check the registry
	if entry := registry.Lookup(repoName, branchName); entry != nil {
		// Verify the path still exists
		if _, err := os.Stat(entry.Path); err == nil {
			return true, &Worktree{
				Name:   filepath.Base(entry.Path),
				Path:   entry.Path,
				Branch: entry.Branch,
			}
		}
		// Path doesn't exist, clean up stale registry entry
		registry.Unregister(repoName, branchName)
	}

	// Fall back to file system scan
	worktrees, err := ListWorktrees(repoName)
	if err != nil {
		return false, nil
	}

	// Sanitize branch name to match how it would be used as a worktree directory
	sanitizedBranch := SanitizeBranchName(branchName)

	for _, wt := range worktrees {
		if wt.Name == sanitizedBranch || wt.Name == branchName || wt.Branch == branchName {
			return true, &wt
		}
	}

	return false, nil
}

func SanitizeBranchName(branchName string) string {
	// Replace problematic characters with underscores
	replacer := strings.NewReplacer(
		"/", "_",
		":", "_",
		" ", "_",
		"\\", "_",
		"*", "_",
		"?", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\"", "_",
	)
	return replacer.Replace(branchName)
}

func GitWorktreeExistsForBranch(branchName string) (bool, string) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	lines := strings.Split(string(output), "\n")
	var currentPath string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			// Remove refs/heads/ prefix if present
			branch = strings.TrimPrefix(branch, "refs/heads/")
			if branch == branchName {
				return true, currentPath
			}
		}
	}

	return false, ""
}

type Project struct {
	Name      string
	Path      string
	Worktrees []Worktree
}

func ListAllProjects() ([]Project, error) {
	baseDir := GetWorktreeBaseDir()

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return []Project{}, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read worktree base directory: %w", err)
	}

	var projects []Project
	for _, entry := range entries {
		if entry.IsDir() {
			projectName := entry.Name()
			projectPath := filepath.Join(baseDir, projectName)

			// List worktrees for this project
			worktrees, err := ListWorktrees(projectName)
			if err != nil {
				// Skip projects we can't read worktrees for
				continue
			}

			// Only include projects that have at least one worktree
			if len(worktrees) > 0 {
				projects = append(projects, Project{
					Name:      projectName,
					Path:      projectPath,
					Worktrees: worktrees,
				})
			}
		}
	}

	// Sort projects by name
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, nil
}

func CreateWorktree(repoName, worktreeName, branchName string) (string, error) {
	// First check if we already have this worktree registered
	if entry := registry.Lookup(repoName, branchName); entry != nil {
		// Verify the path still exists
		if _, err := os.Stat(entry.Path); err == nil {
			return "", fmt.Errorf("worktree for branch '%s' already exists at %s", branchName, entry.Path)
		}
		// Path doesn't exist, clean up stale registry entry
		registry.Unregister(repoName, branchName)
	}

	// Sanitize the worktree name for use as a folder name
	sanitizedName := SanitizeBranchName(worktreeName)

	worktreeDir := GetWorktreeDir(repoName)
	worktreePath := filepath.Join(worktreeDir, sanitizedName)

	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("worktree '%s' already exists at %s", sanitizedName, worktreePath)
	}

	// Create the worktree
	cmd := exec.Command("git", "worktree", "add", worktreePath, branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %s", string(output))
	}

	// Register the worktree in the registry
	if err := registry.Register(repoName, branchName, worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register worktree: %v\n", err)
	}

	// Copy configured files from main repository to new worktree
	if err := copyConfiguredFiles(worktreePath); err != nil {
		// Log warning but don't fail the worktree creation
		fmt.Fprintf(os.Stderr, "Warning: failed to copy some files: %v\n", err)
	}

	return worktreePath, nil
}

func copyConfiguredFiles(worktreePath string) error {
	// Get the main repository path (current directory)
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get repository root: %w", err)
	}
	mainRepoPath := strings.TrimSpace(string(output))

	if config.IsVerbose() {
		fmt.Printf("[copy] Main repo path: %s\n", mainRepoPath)
	}

	// Get files to copy from config
	filesToCopy := config.GetCopyFiles()
	if config.IsVerbose() {
		fmt.Printf("[copy] Files to copy: %v\n", filesToCopy)
	}
	if len(filesToCopy) == 0 {
		if config.IsVerbose() {
			fmt.Println("[copy] No files configured to copy")
		}
		return nil
	}

	var copyErrors []string
	for _, pattern := range filesToCopy {
		// Expand glob patterns
		matches, err := filepath.Glob(filepath.Join(mainRepoPath, pattern))
		if err != nil {
			copyErrors = append(copyErrors, fmt.Sprintf("%s: %v", pattern, err))
			continue
		}

		if len(matches) == 0 {
			// Pattern didn't match any files - this is okay, skip silently
			continue
		}

		for _, sourcePath := range matches {
			// Get relative path from main repo
			relPath, err := filepath.Rel(mainRepoPath, sourcePath)
			if err != nil {
				copyErrors = append(copyErrors, fmt.Sprintf("%s: %v", sourcePath, err))
				continue
			}

			destPath := filepath.Join(worktreePath, relPath)

			// Create destination directory if needed
			destDir := filepath.Dir(destPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				copyErrors = append(copyErrors, fmt.Sprintf("%s: %v", relPath, err))
				continue
			}

			// Copy the file
			if err := copyFile(sourcePath, destPath); err != nil {
				copyErrors = append(copyErrors, fmt.Sprintf("%s: %v", relPath, err))
				continue
			}

			fmt.Printf("Copied: %s\n", relPath)
		}
	}

	if len(copyErrors) > 0 {
		return fmt.Errorf("failed to copy some files:\n  %s", strings.Join(copyErrors, "\n  "))
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Get source file info to preserve permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	// Skip directories
	if sourceInfo.IsDir() {
		return nil
	}

	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func RemoveWorktree(worktreePath string) error {
	// Unregister from the registry first (before removing the worktree)
	if err := registry.UnregisterByPath(worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to unregister worktree: %v\n", err)
	}

	// Use git worktree remove with --force to handle uncommitted changes
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s", string(output))
	}

	return nil
}

func IsMainWorktree(worktreePath string) (bool, error) {
	// Get the main repository path
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get main repository path: %w", err)
	}

	mainRepoPath := strings.TrimSpace(string(output))

	// Compare paths
	absWorktreePath, err := filepath.Abs(worktreePath)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path: %w", err)
	}

	absMainRepoPath, err := filepath.Abs(mainRepoPath)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute main repo path: %w", err)
	}

	return absWorktreePath == absMainRepoPath, nil
}

func GetWorktreeBranch(worktreePath string) string {
	// Change to the worktree directory and get the current branch
	cmd := exec.Command("git", "-C", worktreePath, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try to get branch from HEAD
		cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			return "unknown"
		}
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		// Detached HEAD state, try to get the commit hash
		cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "--short", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			return "detached"
		}
		return fmt.Sprintf("detached@%s", strings.TrimSpace(string(output)))
	}

	return branch
}
