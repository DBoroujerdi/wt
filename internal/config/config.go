package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type TmuxWindow struct {
	Name    string `toml:"name"`
	Command string `toml:"command"`
}

type Config struct {
	WorktreesLocation string       `toml:"worktrees_location"`
	CopyFiles         []string     `toml:"copy_files"`
	TmuxWindows       []TmuxWindow `toml:"tmux_windows"`
}

var defaultConfig = Config{
	WorktreesLocation: filepath.Join(os.Getenv("HOME"), "projects", "worktrees"),
	CopyFiles:         []string{},
	TmuxWindows:       []TmuxWindow{},
}

var currentConfig *Config
var verboseMode bool

// SetVerbose enables or disables verbose logging
func SetVerbose(v bool) {
	verboseMode = v
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verboseMode
}

func logVerbose(format string, args ...interface{}) {
	if verboseMode {
		fmt.Printf("[config] "+format+"\n", args...)
	}
}

func Load() (*Config, error) {
	if currentConfig != nil {
		logVerbose("Using cached config")
		return currentConfig, nil
	}

	config := defaultConfig
	logVerbose("Starting config load")

	// Load global config
	var globalConfig Config
	globalConfigPath := getGlobalConfigPath()
	logVerbose("Looking for global config at: %s", globalConfigPath)
	if err := loadConfigFile(globalConfigPath, &globalConfig); err == nil {
		logVerbose("Loaded global config")
		// Merge global config
		if globalConfig.WorktreesLocation != "" {
			logVerbose("  worktrees_location: %s", globalConfig.WorktreesLocation)
			config.WorktreesLocation = globalConfig.WorktreesLocation
		}
		if len(globalConfig.CopyFiles) > 0 {
			logVerbose("  copy_files: %v", globalConfig.CopyFiles)
		}
		if len(globalConfig.TmuxWindows) > 0 {
			logVerbose("  tmux_windows: %d windows", len(globalConfig.TmuxWindows))
			for _, w := range globalConfig.TmuxWindows {
				logVerbose("    - %s: %s", w.Name, w.Command)
			}
		}
		config.CopyFiles = append(config.CopyFiles, globalConfig.CopyFiles...)
		config.TmuxWindows = append(config.TmuxWindows, globalConfig.TmuxWindows...)
	} else if os.IsNotExist(err) {
		logVerbose("Global config not found")
	} else {
		return nil, fmt.Errorf("error loading global config: %w", err)
	}

	// Load local config
	var localConfig Config
	localConfigPath := getLocalConfigPath()
	logVerbose("Looking for local config at: %s", localConfigPath)
	if err := loadConfigFile(localConfigPath, &localConfig); err == nil {
		logVerbose("Loaded local config")
		// Local config overrides
		if localConfig.WorktreesLocation != "" {
			logVerbose("  worktrees_location: %s", localConfig.WorktreesLocation)
			config.WorktreesLocation = localConfig.WorktreesLocation
		}
		if len(localConfig.CopyFiles) > 0 {
			logVerbose("  copy_files: %v", localConfig.CopyFiles)
		}
		if len(localConfig.TmuxWindows) > 0 {
			logVerbose("  tmux_windows: %d windows", len(localConfig.TmuxWindows))
			for _, w := range localConfig.TmuxWindows {
				logVerbose("    - %s: %s", w.Name, w.Command)
			}
		}
		// Merge copy_files arrays (local adds to global)
		config.CopyFiles = append(config.CopyFiles, localConfig.CopyFiles...)
		// Merge tmux_windows arrays (local adds to global)
		config.TmuxWindows = append(config.TmuxWindows, localConfig.TmuxWindows...)
	} else if os.IsNotExist(err) {
		logVerbose("Local config not found")
	} else {
		// Show parse errors as warnings even without verbose mode
		fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", localConfigPath, err)
		fmt.Fprintf(os.Stderr, "  Using default config instead. Run with -v for details.\n")
	}

	// Remove duplicates from CopyFiles
	config.CopyFiles = removeDuplicates(config.CopyFiles)

	config.WorktreesLocation = expandPath(config.WorktreesLocation)

	logVerbose("Final config:")
	logVerbose("  worktrees_location: %s", config.WorktreesLocation)
	logVerbose("  copy_files: %v", config.CopyFiles)
	logVerbose("  tmux_windows: %d windows", len(config.TmuxWindows))

	currentConfig = &config
	return currentConfig, nil
}

func removeDuplicates(files []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, file := range files {
		if !seen[file] {
			seen[file] = true
			result = append(result, file)
		}
	}
	return result
}

func loadConfigFile(path string, config *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if _, err := toml.Decode(string(data), config); err != nil {
		logVerbose("TOML parse error in %s: %v", path, err)
		return fmt.Errorf("error parsing config file %s: %w", path, err)
	}

	return nil
}

func getGlobalConfigPath() string {
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(xdgConfigHome, "wt", "config.toml")
}

func getLocalConfigPath() string {
	// Find the git repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		// Not in a git repo, fall back to current directory
		return ".wt.toml"
	}
	repoRoot := strings.TrimSpace(string(output))
	return filepath.Join(repoRoot, ".wt.toml")
}

func expandPath(path string) string {
	if path == "" {
		return path
	}

	if path[:2] == "~/" {
		home := os.Getenv("HOME")
		path = filepath.Join(home, path[2:])
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	return absPath
}

func GetWorktreesLocation() string {
	config, err := Load()
	if err != nil {
		return defaultConfig.WorktreesLocation
	}
	return config.WorktreesLocation
}

func GetCopyFiles() []string {
	config, err := Load()
	if err != nil {
		return defaultConfig.CopyFiles
	}
	return config.CopyFiles
}

func GetTmuxWindows() []TmuxWindow {
	config, err := Load()
	if err != nil {
		return defaultConfig.TmuxWindows
	}
	return config.TmuxWindows
}

func CreateGlobalConfigDir() error {
	configPath := getGlobalConfigPath()
	configDir := filepath.Dir(configPath)
	return os.MkdirAll(configDir, 0755)
}
