package worktree

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/todoengineering/wt/internal/config"
	"github.com/todoengineering/wt/internal/editor"
	"github.com/todoengineering/wt/internal/git"
	"github.com/todoengineering/wt/internal/tmux"
	"github.com/todoengineering/wt/internal/ui"
)

var newFromBranch string

var newCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create new worktree",
	Long: `Two modes:
1) Default: Creates a new Git branch named <name> and a worktree for it.
2) With --from <branch>: Creates a worktree for an existing branch, optionally named <name>.
In both modes, opens the worktree in the configured editor and creates/switches to a tmux session.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Enable verbose logging if flag is set
		config.SetVerbose(verbose)

		// Check if we're in a git repository
		if !git.IsGitRepository() {
			fmt.Fprintf(os.Stderr, "Error: not in a git repository\n")
			os.Exit(1)
		}

		// Get repository name
		repoName, err := git.GetRepositoryName()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Mode selection: from existing branch vs new branch
		var worktreeName string
		var worktreePath string
		if newFromBranch != "" {
			// Create worktree from an existing branch
			// Always fetch remote branches for up-to-date list
			fmt.Println("Fetching remote branches...")
			if err := git.FetchRemoteBranches(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to fetch remote branches: %v\n", err)
			}

			sourceBranch := newFromBranch
			if sourceBranch == ":pick" {
				// Interactive selection
				branches, err := git.ListAllBranches()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error listing branches: %v\n", err)
					os.Exit(1)
				}

				// Filter out current branch
				currentBranch, _ := git.GetCurrentBranch()
				var selectableBranches []git.Branch
				for _, b := range branches {
					if b.Name != currentBranch {
						selectableBranches = append(selectableBranches, b)
					}
				}
				if len(selectableBranches) == 0 {
					fmt.Println("No other branches available")
					os.Exit(1)
				}

				picked, err := selectBranchInteractive(selectableBranches)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error selecting branch: %v\n", err)
					os.Exit(1)
				}
				sourceBranch = picked
			}

			// Determine worktree name
			if len(args) > 0 {
				worktreeName = args[0]
			} else {
				// Default to sanitized branch name
				worktreeName = git.SanitizeBranchName(sourceBranch)
			}

			// If a worktree already exists for this branch, automatically switch to it
			if gitWorktreeExists, gitWorktreePath := git.GitWorktreeExistsForBranch(sourceBranch); gitWorktreeExists {
				fmt.Printf("A worktree already exists for branch '%s' at:\n  %s\n", sourceBranch, gitWorktreePath)
				fmt.Printf("Switching to existing worktree...\n")
				// Convert to Worktree struct for openWorktree function
				existing := &git.Worktree{
					Name:   sourceBranch, // Use branch name for better session naming
					Path:   gitWorktreePath,
					Branch: sourceBranch,
				}
				openWorktree(repoName, *existing)
				return
			}

			fmt.Printf("Creating worktree '%s' for branch '%s'...\n", worktreeName, sourceBranch)
			p, err := git.CreateWorktree(repoName, worktreeName, sourceBranch)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			worktreePath = p
		} else {
			// Default behavior: create a new branch, then a worktree for it
			// Get or prompt for worktree/branch name
			if len(args) > 0 {
				worktreeName = args[0]
			} else {
				// Interactive mode - prompt for name
				fmt.Print("Enter worktree name: ")
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
					os.Exit(1)
				}
				worktreeName = strings.TrimSpace(input)
				if worktreeName == "" {
					fmt.Fprintf(os.Stderr, "Error: worktree name cannot be empty\n")
					os.Exit(1)
				}
			}

			// Check if branch already exists
			branchExists, err := git.BranchExists(worktreeName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error checking branch existence: %v\n", err)
				os.Exit(1)
			}

			if branchExists {
				fmt.Printf("Branch '%s' already exists, using it...\n", worktreeName)
				// Check if ANY worktree exists for this branch (not just ones managed by this tool)
				if gitWorktreeExists, gitWorktreePath := git.GitWorktreeExistsForBranch(worktreeName); gitWorktreeExists {
					fmt.Printf("A worktree already exists for branch '%s' at:\n  %s\n", worktreeName, gitWorktreePath)
					fmt.Printf("Switching to existing worktree...\n")
					// Convert to Worktree struct for openWorktree function
					existing := &git.Worktree{
						Name:   worktreeName, // Use branch name for better session naming
						Path:   gitWorktreePath,
						Branch: worktreeName,
					}
					openWorktree(repoName, *existing)
					return
				}
			} else {
				// Create new branch
				fmt.Printf("Creating branch '%s'...\n", worktreeName)
				if err := git.CreateBranch(worktreeName); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}

			// Create worktree for the branch (existing or newly created)
			fmt.Printf("Creating worktree for '%s'...\n", worktreeName)
			p, err := git.CreateWorktree(repoName, worktreeName, worktreeName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			worktreePath = p
		}

		fmt.Printf("Worktree created at: %s\n", worktreePath)

		// Create/switch tmux session and/or open editor according to flags
		// Standardize on session name: <repo>-<worktree>
		sessionName := tmux.SanitizeSessionName(fmt.Sprintf("%s-%s", repoName, worktreeName))
		if noTmux {
			if !noEditor {
				if err := editor.OpenInEditor(worktreePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					fmt.Printf("Opened in editor\n")
				}
			}
			return
		}
		if tmux.IsInstalled() {
			if verbose {
				fmt.Println("[tmux] tmux is installed")
			}
			// Check if tmux_windows is configured
			tmuxWindows := config.GetTmuxWindows()
			if verbose {
				fmt.Printf("[tmux] Found %d configured windows\n", len(tmuxWindows))
				for i, w := range tmuxWindows {
					fmt.Printf("[tmux]   %d: name=%q command=%q\n", i+1, w.Name, w.Command)
				}
			}
			if len(tmuxWindows) > 0 {
				// Convert config.TmuxWindow to tmux.TmuxWindow
				windows := make([]tmux.TmuxWindow, len(tmuxWindows))
				for i, w := range tmuxWindows {
					windows[i] = tmux.TmuxWindow{
						Name:    w.Name,
						Command: w.Command,
					}
				}

				if err := tmux.CreateSessionWithNamedWindows(sessionName, worktreePath, windows); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					fmt.Printf("Tmux session '%s' created with configured windows\n", sessionName)
				}
			} else if noEditor {
				if err := tmux.CreateSession(sessionName, worktreePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					fmt.Printf("Tmux session '%s' created\n", sessionName)
				}
			} else {
				if err := tmux.CreateSessionWithCommand(sessionName, worktreePath, editor.GetEditorCommand(worktreePath)); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					fmt.Printf("Tmux session '%s' created with editor\n", sessionName)
				}
			}
		} else {
			if !noEditor {
				if err := editor.OpenInEditor(worktreePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					fmt.Printf("Opened in editor\n")
				}
			}
		}
	},
}

func init() {
	newCmd.Flags().StringVar(&newFromBranch, "from", "", "create a worktree from an existing branch (optionally provide <name> for worktree)")
}

// Branch selection helpers
func selectBranchInteractive(branches []git.Branch) (string, error) {
	// Sort branches: local first, then by name
	sort.Slice(branches, func(i, j int) bool {
		if branches[i].IsLocal != branches[j].IsLocal {
			return branches[i].IsLocal
		}
		return branches[i].Name < branches[j].Name
	})

	var items []ui.Item
	for _, branch := range branches {
		status := ""
		if branch.IsLocal && branch.IsRemote {
			status = "[local+remote]"
		} else if branch.IsLocal {
			status = "[local]"
		} else {
			status = "[remote]"
		}

		items = append(items, ui.Item{
			TitleStr:       branch.Name,
			DescriptionStr: status,
			FilterStr:      branch.Name,
			Value:          branch.Name,
		})
	}

	selected, err := ui.Select(items, "Select a branch to create a worktree from")
	if err != nil {
		return "", err
	}

	return selected.Value.(string), nil
}
