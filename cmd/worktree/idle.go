package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/todoengineering/wt/internal/idle"
)

var idleCmd = &cobra.Command{
	Use:   "idle",
	Short: "Track and switch to idle Claude agent sessions",
}

var idleRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Mark the current tmux pane as waiting for input",
	RunE:  runIdleRegister,
}

var idleActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Mark the current tmux pane as active",
	RunE:  runIdleActive,
}

var idleListAll bool

var idleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions waiting for input",
	RunE:  runIdleList,
}

var idleSwitchCmd = &cobra.Command{
	Use:   "switch <id-or-session>",
	Short: "Switch to an idle session by ID or session name",
	Args:  cobra.ExactArgs(1),
	RunE:  runIdleSwitch,
}

var idleCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove records for tmux sessions that no longer exist",
	RunE:  runIdleClean,
}

func init() {
	idleListCmd.Flags().BoolVarP(&idleListAll, "all", "a", false, "include active sessions")

	idleCmd.AddCommand(idleRegisterCmd)
	idleCmd.AddCommand(idleActiveCmd)
	idleCmd.AddCommand(idleListCmd)
	idleCmd.AddCommand(idleSwitchCmd)
	idleCmd.AddCommand(idleCleanCmd)
}

func runIdleRegister(_ *cobra.Command, _ []string) error {
	pane, ok := idle.CurrentPane()
	if !ok {
		return nil
	}
	sessions, err := idle.Load()
	if err != nil {
		return err
	}
	sessions = idle.Upsert(sessions, pane, idle.StatusWaiting)
	return idle.Save(sessions)
}

func runIdleActive(_ *cobra.Command, _ []string) error {
	pane, ok := idle.CurrentPane()
	if !ok {
		return nil
	}
	sessions, err := idle.Load()
	if err != nil {
		return err
	}
	if idle.Find(sessions, pane) == nil {
		return nil
	}
	sessions = idle.Upsert(sessions, pane, idle.StatusActive)
	return idle.Save(sessions)
}

func runIdleList(_ *cobra.Command, _ []string) error {
	sessions, err := idle.Load()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions.")
		return nil
	}

	liveSessions, _ := idle.LiveSessions()

	var filtered []idle.Session
	var skipped []string
	for _, s := range sessions {
		if !idleListAll && s.Status != idle.StatusWaiting {
			continue
		}
		if !liveSessions[s.SessionName] {
			skipped = append(skipped, s.SessionName)
			continue
		}
		filtered = append(filtered, s)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt > filtered[j].UpdatedAt
	})

	if len(filtered) == 0 {
		fmt.Println("No sessions.")
		if len(skipped) > 0 {
			fmt.Printf("Note: %d session(s) skipped (no longer in tmux): %s\n", len(skipped), strings.Join(skipped, ", "))
		}
		return nil
	}

	home, _ := os.UserHomeDir()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if idleListAll {
		fmt.Fprintln(w, "ID\tSESSION\tWINDOW\tPANE\tDIR\tSTATUS\tWAITING")
	} else {
		fmt.Fprintln(w, "ID\tSESSION\tWINDOW\tPANE\tDIR\tWAITING")
	}
	for _, s := range filtered {
		dir := shortenHomePath(s.ProjectDir, home)
		rel := idle.FormatRelative(s.UpdatedAt)
		if idleListAll {
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\t%s\n", s.ID, s.SessionName, s.WindowIndex, s.PaneID, dir, s.Status, rel)
		} else {
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\n", s.ID, s.SessionName, s.WindowIndex, s.PaneID, dir, rel)
		}
	}
	w.Flush()

	if len(skipped) > 0 {
		fmt.Printf("\nNote: %d session(s) skipped (no longer in tmux): %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	return nil
}

func shortenHomePath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func runIdleSwitch(_ *cobra.Command, args []string) error {
	sessions, err := idle.Load()
	if err != nil {
		return err
	}

	found := idle.FindByIDOrName(sessions, args[0])
	if found == nil {
		return fmt.Errorf("no session found for %q", args[0])
	}

	tmuxTarget := fmt.Sprintf("%s:%d.%s", found.SessionName, found.WindowIndex, found.PaneID)
	out, execErr := exec.Command("tmux", "switch-client", "-t", tmuxTarget).CombinedOutput()
	if execErr != nil {
		return fmt.Errorf("tmux switch-client failed: %s", strings.TrimSpace(string(out)))
	}

	pane := idle.PaneInfo{
		SessionName: found.SessionName,
		WindowIndex: found.WindowIndex,
		WindowName:  found.WindowName,
		PaneID:      found.PaneID,
	}
	sessions = idle.Upsert(sessions, pane, idle.StatusActive)
	return idle.Save(sessions)
}

func runIdleClean(_ *cobra.Command, _ []string) error {
	liveSessions, err := idle.LiveSessions()
	if err != nil {
		return err
	}
	sessions, err := idle.Load()
	if err != nil {
		return err
	}
	kept := []idle.Session{}
	removed := 0
	for _, s := range sessions {
		if liveSessions[s.SessionName] {
			kept = append(kept, s)
		} else {
			removed++
		}
	}
	if err := idle.Save(kept); err != nil {
		return err
	}
	if removed == 0 {
		fmt.Println("No records removed.")
	} else {
		fmt.Printf("Removed %d record(s).\n", removed)
	}
	return nil
}
