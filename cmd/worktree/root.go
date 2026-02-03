package worktree

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var noEditor bool
var noTmux bool
var verbose bool

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "A CLI tool for managing Git worktrees",
	Long: `wt is a Go-based CLI tool for managing Git worktrees 
with enhanced features including automatic tmux session management 
and editor integration.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Common behavior flags across subcommands that open things
	rootCmd.PersistentFlags().BoolVar(&noEditor, "no-editor", false, "don't open the editor")
	rootCmd.PersistentFlags().BoolVar(&noTmux, "no-tmux", false, "don't create/switch tmux")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output for debugging")

	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(deleteCmd)
}
