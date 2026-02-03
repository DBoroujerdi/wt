package registry

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// WorktreeEntry represents a single worktree registration
type WorktreeEntry struct {
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Repo   string `json:"repo"`
}

// Registry holds all registered worktrees
type Registry struct {
	Worktrees []WorktreeEntry `json:"worktrees"`
}

var (
	registryMutex sync.Mutex
	registryCache *Registry
)

// getRegistryPath returns the path to the registry file
func getRegistryPath() string {
	xdgDataHome := os.Getenv("XDG_DATA_HOME")
	if xdgDataHome == "" {
		xdgDataHome = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(xdgDataHome, "wt", "worktrees.json")
}

// Load reads the registry from disk
func Load() (*Registry, error) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if registryCache != nil {
		return registryCache, nil
	}

	registryPath := getRegistryPath()
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			registryCache = &Registry{Worktrees: []WorktreeEntry{}}
			return registryCache, nil
		}
		return nil, err
	}

	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}

	registryCache = &registry
	return registryCache, nil
}

// Save writes the registry to disk
func Save(registry *Registry) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	registryPath := getRegistryPath()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}

	registryCache = registry
	return os.WriteFile(registryPath, data, 0644)
}

// Register adds a new worktree entry to the registry
func Register(repo, branch, path string) error {
	registry, err := Load()
	if err != nil {
		return err
	}

	// Check if entry already exists, update it if so
	for i, entry := range registry.Worktrees {
		if entry.Repo == repo && entry.Branch == branch {
			registry.Worktrees[i].Path = path
			return Save(registry)
		}
	}

	// Add new entry
	registry.Worktrees = append(registry.Worktrees, WorktreeEntry{
		Branch: branch,
		Path:   path,
		Repo:   repo,
	})

	return Save(registry)
}

// Unregister removes a worktree entry from the registry
func Unregister(repo, branch string) error {
	registry, err := Load()
	if err != nil {
		return err
	}

	// Find and remove the entry
	for i, entry := range registry.Worktrees {
		if entry.Repo == repo && entry.Branch == branch {
			registry.Worktrees = append(registry.Worktrees[:i], registry.Worktrees[i+1:]...)
			return Save(registry)
		}
	}

	return nil // Entry not found, nothing to do
}

// UnregisterByPath removes a worktree entry by its path
func UnregisterByPath(path string) error {
	registry, err := Load()
	if err != nil {
		return err
	}

	// Find and remove the entry
	for i, entry := range registry.Worktrees {
		if entry.Path == path {
			registry.Worktrees = append(registry.Worktrees[:i], registry.Worktrees[i+1:]...)
			return Save(registry)
		}
	}

	return nil // Entry not found, nothing to do
}

// Lookup finds a worktree entry by repo and branch
func Lookup(repo, branch string) *WorktreeEntry {
	registry, err := Load()
	if err != nil {
		return nil
	}

	for _, entry := range registry.Worktrees {
		if entry.Repo == repo && entry.Branch == branch {
			return &entry
		}
	}

	return nil
}

// LookupByPath finds a worktree entry by its path
func LookupByPath(path string) *WorktreeEntry {
	registry, err := Load()
	if err != nil {
		return nil
	}

	for _, entry := range registry.Worktrees {
		if entry.Path == path {
			return &entry
		}
	}

	return nil
}

// ListByRepo returns all worktree entries for a given repo
func ListByRepo(repo string) []WorktreeEntry {
	registry, err := Load()
	if err != nil {
		return nil
	}

	var entries []WorktreeEntry
	for _, entry := range registry.Worktrees {
		if entry.Repo == repo {
			entries = append(entries, entry)
		}
	}

	return entries
}

// ClearCache clears the in-memory cache (useful for testing)
func ClearCache() {
	registryMutex.Lock()
	defer registryMutex.Unlock()
	registryCache = nil
}

// SyncStatus represents the sync state of a worktree
type SyncStatus string

const (
	StatusOK       SyncStatus = "ok"       // Worktree exists and is in registry
	StatusMissing  SyncStatus = "missing"  // In registry but path doesn't exist
	StatusOrphaned SyncStatus = "orphaned" // On disk but not in registry (just discovered)
)

// SyncedWorktree represents a worktree with its sync status
type SyncedWorktree struct {
	Entry  WorktreeEntry
	Status SyncStatus
}

// Sync reconciles the registry with the filesystem for a given repo
// It returns the merged list of worktrees with their sync status
func Sync(repo, worktreeDir string) ([]SyncedWorktree, error) {
	registry, err := Load()
	if err != nil {
		return nil, err
	}

	var result []SyncedWorktree
	seenPaths := make(map[string]bool)

	// First, check all registry entries for this repo
	for _, entry := range registry.Worktrees {
		if entry.Repo != repo {
			continue
		}

		status := StatusOK
		if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
			status = StatusMissing
		} else {
			seenPaths[entry.Path] = true
		}

		result = append(result, SyncedWorktree{
			Entry:  entry,
			Status: status,
		})
	}

	// Now scan the filesystem for worktrees not in registry
	if _, err := os.Stat(worktreeDir); err == nil {
		entries, err := os.ReadDir(worktreeDir)
		if err == nil {
			for _, dirEntry := range entries {
				if !dirEntry.IsDir() {
					continue
				}

				worktreePath := filepath.Join(worktreeDir, dirEntry.Name())

				// Skip if we already saw this path in the registry
				if seenPaths[worktreePath] {
					continue
				}

				// Get the branch name from git
				branch := getWorktreeBranchFromGit(worktreePath)

				// Add to registry for future lookups
				newEntry := WorktreeEntry{
					Branch: branch,
					Path:   worktreePath,
					Repo:   repo,
				}

				result = append(result, SyncedWorktree{
					Entry:  newEntry,
					Status: StatusOrphaned,
				})

				// Register the orphaned worktree
				registry.Worktrees = append(registry.Worktrees, newEntry)
			}
		}
	}

	// Save any newly discovered worktrees
	if err := Save(registry); err != nil {
		// Non-fatal: continue with what we have
	}

	return result, nil
}

// getWorktreeBranchFromGit gets the current branch of a worktree directory
func getWorktreeBranchFromGit(worktreePath string) string {
	cmd := exec.Command("git", "-C", worktreePath, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try rev-parse
		cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			// Final fallback: use the directory name
			return filepath.Base(worktreePath)
		}
	}
	return strings.TrimSpace(string(output))
}
