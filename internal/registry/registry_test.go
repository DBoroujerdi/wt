package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryRoundTrip(t *testing.T) {
	// Create a temp directory for the test
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	// Clear any cached registry
	ClearCache()

	// Register a worktree
	err := Register("myrepo", "feat/my-branch", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Clear cache to force reload from disk
	ClearCache()

	// Lookup the worktree
	entry := Lookup("myrepo", "feat/my-branch")
	if entry == nil {
		t.Fatal("Expected to find registered worktree")
	}

	if entry.Repo != "myrepo" {
		t.Errorf("Expected repo 'myrepo', got '%s'", entry.Repo)
	}
	if entry.Branch != "feat/my-branch" {
		t.Errorf("Expected branch 'feat/my-branch', got '%s'", entry.Branch)
	}
	if entry.Path != "/path/to/worktree" {
		t.Errorf("Expected path '/path/to/worktree', got '%s'", entry.Path)
	}
}

func TestRegistryUpdate(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register initial worktree
	err := Register("myrepo", "main", "/old/path")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Update the same worktree with new path
	err = Register("myrepo", "main", "/new/path")
	if err != nil {
		t.Fatalf("Failed to update worktree: %v", err)
	}

	ClearCache()

	// Verify the path was updated
	entry := Lookup("myrepo", "main")
	if entry == nil {
		t.Fatal("Expected to find registered worktree")
	}
	if entry.Path != "/new/path" {
		t.Errorf("Expected path '/new/path', got '%s'", entry.Path)
	}

	// Verify only one entry exists
	entries := ListByRepo("myrepo")
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}
}

func TestRegistryUnregister(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register a worktree
	err := Register("myrepo", "feat/branch", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Unregister it
	err = Unregister("myrepo", "feat/branch")
	if err != nil {
		t.Fatalf("Failed to unregister worktree: %v", err)
	}

	ClearCache()

	// Verify it's gone
	entry := Lookup("myrepo", "feat/branch")
	if entry != nil {
		t.Error("Expected worktree to be unregistered")
	}
}

func TestRegistryUnregisterByPath(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register a worktree
	err := Register("myrepo", "feat/branch", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Unregister by path
	err = UnregisterByPath("/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to unregister worktree by path: %v", err)
	}

	ClearCache()

	// Verify it's gone
	entry := Lookup("myrepo", "feat/branch")
	if entry != nil {
		t.Error("Expected worktree to be unregistered")
	}
}

func TestRegistryLookupByPath(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register a worktree
	err := Register("myrepo", "feat/branch", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Lookup by path
	entry := LookupByPath("/path/to/worktree")
	if entry == nil {
		t.Fatal("Expected to find worktree by path")
	}

	if entry.Branch != "feat/branch" {
		t.Errorf("Expected branch 'feat/branch', got '%s'", entry.Branch)
	}
}

func TestRegistryListByRepo(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register multiple worktrees for same repo
	Register("myrepo", "main", "/path/main")
	Register("myrepo", "feat/a", "/path/feat_a")
	Register("myrepo", "feat/b", "/path/feat_b")
	Register("otherrepo", "main", "/other/main")

	entries := ListByRepo("myrepo")
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries for myrepo, got %d", len(entries))
	}

	entries = ListByRepo("otherrepo")
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry for otherrepo, got %d", len(entries))
	}
}

func TestRegistryFileCreation(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register a worktree
	err := Register("myrepo", "main", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	// Check that the file was created
	expectedPath := filepath.Join(tempDir, "wt", "worktrees.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected registry file at %s", expectedPath)
	}
}

func TestEmptyRegistry(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Lookup in empty registry should return nil
	entry := Lookup("nonexistent", "branch")
	if entry != nil {
		t.Error("Expected nil for lookup in empty registry")
	}

	// Unregister from empty registry should not error
	err := Unregister("nonexistent", "branch")
	if err != nil {
		t.Errorf("Unregister from empty registry should not error: %v", err)
	}
}

func TestSyncDetectsMissingWorktrees(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Register a worktree that doesn't exist on disk
	err := Register("myrepo", "feat/missing-branch", "/nonexistent/path")
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	ClearCache()

	// Sync should detect the missing worktree
	worktreeDir := filepath.Join(tempDir, "worktrees", "myrepo")
	synced, err := Sync("myrepo", worktreeDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(synced) != 1 {
		t.Fatalf("Expected 1 synced worktree, got %d", len(synced))
	}

	if synced[0].Status != StatusMissing {
		t.Errorf("Expected status StatusMissing, got %v", synced[0].Status)
	}
}

func TestSyncDetectsOrphanedWorktrees(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Create a worktree directory on disk without registering it
	worktreeDir := filepath.Join(tempDir, "worktrees", "myrepo")
	orphanedPath := filepath.Join(worktreeDir, "orphaned-worktree")
	if err := os.MkdirAll(orphanedPath, 0755); err != nil {
		t.Fatalf("Failed to create orphaned worktree dir: %v", err)
	}

	// Sync should detect the orphaned worktree
	synced, err := Sync("myrepo", worktreeDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(synced) != 1 {
		t.Fatalf("Expected 1 synced worktree, got %d", len(synced))
	}

	if synced[0].Status != StatusOrphaned {
		t.Errorf("Expected status StatusOrphaned, got %v", synced[0].Status)
	}

	// Check that it was auto-registered
	ClearCache()
	entries := ListByRepo("myrepo")
	if len(entries) != 1 {
		t.Errorf("Expected orphaned worktree to be registered, got %d entries", len(entries))
	}
}

func TestSyncExistingWorktrees(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", tempDir)
	defer os.Unsetenv("XDG_DATA_HOME")

	ClearCache()

	// Create a worktree directory and register it
	worktreeDir := filepath.Join(tempDir, "worktrees", "myrepo")
	existingPath := filepath.Join(worktreeDir, "feat_existing")
	if err := os.MkdirAll(existingPath, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	// Register it with a branch name containing slash
	err := Register("myrepo", "feat/existing", existingPath)
	if err != nil {
		t.Fatalf("Failed to register worktree: %v", err)
	}

	ClearCache()

	// Sync should show it as OK
	synced, err := Sync("myrepo", worktreeDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(synced) != 1 {
		t.Fatalf("Expected 1 synced worktree, got %d", len(synced))
	}

	if synced[0].Status != StatusOK {
		t.Errorf("Expected status StatusOK, got %v", synced[0].Status)
	}

	if synced[0].Entry.Branch != "feat/existing" {
		t.Errorf("Expected branch 'feat/existing', got '%s'", synced[0].Entry.Branch)
	}
}
