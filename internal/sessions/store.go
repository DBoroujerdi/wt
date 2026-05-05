package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID           int    `json:"id"`
	AgentName    string `json:"agent_name"`
	SessionName  string `json:"session_name"`
	WindowIndex  int    `json:"window_index"`
	WindowName   string `json:"window_name"`
	PaneID       string `json:"pane_id"`
	ProjectDir   string `json:"project_dir"`
	RegisteredAt string `json:"registered_at"`
}

type PaneInfo struct {
	SessionName string
	WindowIndex int
	WindowName  string
	PaneID      string
}

var (
	storePathOnce sync.Once
	storePathVal  string
)

func StorePath() string {
	storePathOnce.Do(func() {
		xdgDataHome := os.Getenv("XDG_DATA_HOME")
		if xdgDataHome == "" {
			home, _ := os.UserHomeDir()
			localShare := filepath.Join(home, ".local", "share")
			if _, err := os.Stat(localShare); err == nil {
				xdgDataHome = localShare
			} else {
				xdgDataHome = filepath.Join(home, ".wt")
			}
		}
		storePathVal = filepath.Join(xdgDataHome, "wt", "agent-sessions.json")
	})
	return storePathVal
}

func Load() ([]Session, error) {
	data, err := os.ReadFile(StorePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Session{}, nil
		}
		return nil, err
	}
	var ss []Session
	if err := json.Unmarshal(data, &ss); err != nil {
		return nil, err
	}
	return ss, nil
}

func Save(ss []Session) error {
	if ss == nil {
		ss = []Session{}
	}
	path := StorePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ss, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".claude-sessions-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func CurrentPane() (PaneInfo, bool) {
	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return PaneInfo{}, false
	}
	out, err := exec.Command("tmux", "display-message", "-p",
		"#{session_name}\t#{window_index}\t#{window_name}").Output()
	if err != nil {
		return PaneInfo{}, false
	}
	parts := strings.SplitN(strings.TrimRight(string(out), "\n"), "\t", 3)
	if len(parts) < 3 {
		return PaneInfo{}, false
	}
	windowIndex, _ := strconv.Atoi(parts[1])
	return PaneInfo{
		SessionName: parts[0],
		WindowIndex: windowIndex,
		WindowName:  parts[2],
		PaneID:      paneID,
	}, true
}

// Register adds or updates the session for the given pane (upsert by PaneID).
func Register(ss []Session, pane PaneInfo, agentName string) []Session {
	now := time.Now().UTC().Format(time.RFC3339)
	maxID := 0
	for i, s := range ss {
		if s.ID > maxID {
			maxID = s.ID
		}
		if s.PaneID == pane.PaneID {
			ss[i].AgentName = agentName
			ss[i].SessionName = pane.SessionName
			ss[i].WindowIndex = pane.WindowIndex
			ss[i].WindowName = pane.WindowName
			return ss
		}
	}
	projectDir, _ := os.Getwd()
	return append(ss, Session{
		ID:           maxID + 1,
		AgentName:    agentName,
		SessionName:  pane.SessionName,
		WindowIndex:  pane.WindowIndex,
		WindowName:   pane.WindowName,
		PaneID:       pane.PaneID,
		ProjectDir:   projectDir,
		RegisteredAt: now,
	})
}

// Deregister removes the session matching pane.PaneID.
func Deregister(ss []Session, pane PaneInfo) []Session {
	out := ss[:0]
	for _, s := range ss {
		if s.PaneID != pane.PaneID {
			out = append(out, s)
		}
	}
	return out
}

func FindByID(ss []Session, id int) *Session {
	for i := range ss {
		if ss[i].ID == id {
			return &ss[i]
		}
	}
	return nil
}

// LivePanes returns the set of pane IDs that currently exist in tmux.
func LivePanes() (map[string]bool, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		return map[string]bool{}, nil
	}
	result := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			result[line] = true
		}
	}
	return result, nil
}

func FormatRelative(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
