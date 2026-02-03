package worktree

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/todoengineering/wt/internal/git"
)

var (
	// Styles for list output
	repoStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	branchStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	arrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	missingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Italic(true)
	noItemsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
)

var (
	listAll      bool
	listJSON     bool
	listPathOnly bool
)

type listEntry struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Missing bool   `json:"missing,omitempty"`
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List worktrees",
	Long:  `Lists on-disk worktrees for the current repository or across all projects when --all is provided. Supports JSON and path-only output for scripting.`,
	Run: func(cmd *cobra.Command, args []string) {
		if listAll {
			projects, err := git.ListAllProjects()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing projects: %v\n", err)
				os.Exit(1)
			}

			if listJSON {
				var out []listEntry
				for _, p := range projects {
					for _, wt := range p.Worktrees {
						out = append(out, listEntry{Project: p.Name, Name: wt.Name, Path: wt.Path, Branch: wt.Branch, Missing: wt.Missing})
					}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
					os.Exit(1)
				}
				return
			}

			if listPathOnly {
				for _, p := range projects {
					for _, wt := range p.Worktrees {
						fmt.Println(wt.Path)
					}
				}
				return
			}

			if len(projects) == 0 {
				fmt.Println(noItemsStyle.Render("No projects with worktrees found"))
				fmt.Printf("Worktree base directory: %s\n", pathStyle.Render(git.GetWorktreeBaseDir()))
				return
			}

			for _, p := range projects {
				fmt.Printf("%s\n", repoStyle.Render(p.Name+":"))
				if len(p.Worktrees) == 0 {
					fmt.Printf("  %s\n", noItemsStyle.Render("(no worktrees)"))
					continue
				}
				for _, wt := range p.Worktrees {
					if wt.Missing {
						fmt.Printf("  %s %s %s %s\n",
							warningStyle.Render("⚠"),
							missingStyle.Render(wt.Branch),
							arrowStyle.Render("→"),
							missingStyle.Render(wt.Path+" (missing)"))
					} else {
						fmt.Printf("  %s %s %s\n",
							branchStyle.Render(wt.Branch),
							arrowStyle.Render("→"),
							pathStyle.Render(wt.Path))
					}
				}
			}
			return
		}

		if !git.IsGitRepository() {
			fmt.Fprintf(os.Stderr, "Error: not in a git repository\n")
			os.Exit(1)
		}

		repoName, err := git.GetRepositoryName()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		worktrees, err := git.ListWorktrees(repoName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing worktrees: %v\n", err)
			os.Exit(1)
		}

		if listJSON {
			var out []listEntry
			for _, wt := range worktrees {
				out = append(out, listEntry{Project: repoName, Name: wt.Name, Path: wt.Path, Branch: wt.Branch, Missing: wt.Missing})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if listPathOnly {
			for _, wt := range worktrees {
				fmt.Println(wt.Path)
			}
			return
		}

		if len(worktrees) == 0 {
			fmt.Printf("%s %s\n", noItemsStyle.Render("No worktrees found for repository"), repoStyle.Render("'"+repoName+"'"))
			fmt.Printf("Worktree directory: %s\n", pathStyle.Render(git.GetWorktreeDir(repoName)))
			return
		}

		fmt.Printf("Worktrees for repository %s\n", repoStyle.Render("'"+repoName+"':"))
		for _, wt := range worktrees {
			if wt.Missing {
				fmt.Printf("  %s %s %s %s\n",
					warningStyle.Render("⚠"),
					missingStyle.Render(wt.Branch),
					arrowStyle.Render("→"),
					missingStyle.Render(wt.Path+" (missing)"))
			} else {
				fmt.Printf("  %s %s %s\n",
					branchStyle.Render(wt.Branch),
					arrowStyle.Render("→"),
					pathStyle.Render(wt.Path))
			}
		}
	},
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "list across all projects")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output JSON for scripting")
	listCmd.Flags().BoolVar(&listPathOnly, "path-only", false, "output only worktree paths")
}
