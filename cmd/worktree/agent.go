package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/todoengineering/wt/internal/sessions"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Track and manage running agent instances",
}

var agentRegisterName string

var agentRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register the current tmux pane as an agent session",
	RunE:  runAgentRegister,
}

var agentDeregisterCmd = &cobra.Command{
	Use:   "deregister",
	Short: "Deregister the current tmux pane",
	RunE:  runAgentDeregister,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all running agent instances",
	RunE:  runAgentList,
}

var agentKillAll bool

var agentKillCmd = &cobra.Command{
	Use:   "kill [id]",
	Short: "Kill an agent instance by ID, or all instances with --all",
	RunE:  runAgentKill,
}

var agentSwitchCmd = &cobra.Command{
	Use:   "switch <id>",
	Short: "Switch tmux focus to an agent instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentSwitch,
}

var agentCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove records for panes that no longer exist",
	RunE:  runAgentClean,
}

func init() {
	agentRegisterCmd.Flags().StringVarP(&agentRegisterName, "name", "n", "", "agent name (e.g. claude, cursor, aider)")
	agentKillCmd.Flags().BoolVarP(&agentKillAll, "all", "a", false, "kill all agent instances")

	agentCmd.AddCommand(agentRegisterCmd)
	agentCmd.AddCommand(agentDeregisterCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentKillCmd)
	agentCmd.AddCommand(agentSwitchCmd)
	agentCmd.AddCommand(agentCleanCmd)
}

func runAgentRegister(_ *cobra.Command, _ []string) error {
	pane, ok := sessions.CurrentPane()
	if !ok {
		return nil
	}
	ss, err := sessions.Load()
	if err != nil {
		return err
	}
	ss = sessions.Register(ss, pane, agentRegisterName)
	return sessions.Save(ss)
}

func runAgentDeregister(_ *cobra.Command, _ []string) error {
	pane, ok := sessions.CurrentPane()
	if !ok {
		return nil
	}
	ss, err := sessions.Load()
	if err != nil {
		return err
	}
	ss = sessions.Deregister(ss, pane)
	return sessions.Save(ss)
}

func runAgentList(_ *cobra.Command, _ []string) error {
	ss, err := sessions.Load()
	if err != nil {
		return err
	}

	livePanes, _ := sessions.LivePanes()

	home, _ := os.UserHomeDir()
	var live []sessions.Session
	var stale []string
	for _, s := range ss {
		if !livePanes[s.PaneID] {
			stale = append(stale, s.PaneID)
			continue
		}
		live = append(live, s)
	}

	sort.Slice(live, func(i, j int) bool {
		return live[i].RegisteredAt > live[j].RegisteredAt
	})

	if len(live) == 0 {
		fmt.Println("No agent sessions.")
		if len(stale) > 0 {
			fmt.Printf("Note: %d stale record(s) (run 'wt agent clean')\n", len(stale))
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tAGENT\tSESSION\tWIN\tPANE\tDIR\tSTARTED")
	for _, s := range live {
		dir := shortenHomePath(s.ProjectDir, home)
		rel := sessions.FormatRelative(s.RegisteredAt)
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\t%s\t%s\n",
			s.ID, s.AgentName, s.SessionName, s.WindowIndex, s.PaneID, dir, rel)
	}
	w.Flush()

	if len(stale) > 0 {
		fmt.Printf("\nNote: %d stale record(s) (run 'wt agent clean')\n", len(stale))
	}
	return nil
}

func runAgentKill(_ *cobra.Command, args []string) error {
	ss, err := sessions.Load()
	if err != nil {
		return err
	}

	if agentKillAll {
		livePanes, _ := sessions.LivePanes()
		killed := 0
		for _, s := range ss {
			if !livePanes[s.PaneID] {
				continue
			}
			exec.Command("tmux", "kill-pane", "-t", s.PaneID).Run()
			killed++
		}
		if err := sessions.Save([]sessions.Session{}); err != nil {
			return err
		}
		fmt.Printf("Killed %d session(s).\n", killed)
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide an ID or use --all")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid ID %q: must be a number", args[0])
	}

	found := sessions.FindByID(ss, id)
	if found == nil {
		return fmt.Errorf("no session with ID %d", id)
	}

	out, execErr := exec.Command("tmux", "kill-pane", "-t", found.PaneID).CombinedOutput()
	if execErr != nil {
		return fmt.Errorf("tmux kill-pane: %s", strings.TrimSpace(string(out)))
	}

	pane := sessions.PaneInfo{PaneID: found.PaneID}
	ss = sessions.Deregister(ss, pane)
	return sessions.Save(ss)
}

func runAgentSwitch(_ *cobra.Command, args []string) error {
	ss, err := sessions.Load()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid ID %q: must be a number", args[0])
	}

	found := sessions.FindByID(ss, id)
	if found == nil {
		return fmt.Errorf("no session with ID %d", id)
	}

	tmuxTarget := fmt.Sprintf("%s:%d.%s", found.SessionName, found.WindowIndex, found.PaneID)
	out, execErr := exec.Command("tmux", "switch-client", "-t", tmuxTarget).CombinedOutput()
	if execErr != nil {
		return fmt.Errorf("tmux switch-client failed: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

func runAgentClean(_ *cobra.Command, _ []string) error {
	livePanes, err := sessions.LivePanes()
	if err != nil {
		return err
	}
	ss, err := sessions.Load()
	if err != nil {
		return err
	}
	kept := []sessions.Session{}
	removed := 0
	for _, s := range ss {
		if livePanes[s.PaneID] {
			kept = append(kept, s)
		} else {
			removed++
		}
	}
	if err := sessions.Save(kept); err != nil {
		return err
	}
	if removed == 0 {
		fmt.Println("No records removed.")
	} else {
		fmt.Printf("Removed %d record(s).\n", removed)
	}
	return nil
}
