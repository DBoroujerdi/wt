package idle

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

const (
	StatusWaiting = "waiting"
	StatusActive  = "active"
)

type Session struct {
	ID           int    `json:"id"`
	SessionName  string `json:"session_name"`
	WindowIndex  int    `json:"window_index"`
	WindowName   string `json:"window_name"`
	PaneID       string `json:"pane_id"`
	ProjectDir   string `json:"project_dir"`
	Status       string `json:"status"`
	RegisteredAt string `json:"registered_at"`
	UpdatedAt    string `json:"updated_at"`
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
		storePathVal = filepath.Join(xdgDataHome, "wt", "idle-sessions.json")
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
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func Save(sessions []Session) error {
	if sessions == nil {
		sessions = []Session{}
	}
	path := StorePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".idle-sessions-*.tmp")
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

func Find(sessions []Session, pane PaneInfo) *Session {
	for i := range sessions {
		if sessions[i].SessionName == pane.SessionName && sessions[i].PaneID == pane.PaneID {
			return &sessions[i]
		}
	}
	return nil
}

func FindByIDOrName(sessions []Session, arg string) *Session {
	if id, err := strconv.Atoi(arg); err == nil {
		for i := range sessions {
			if sessions[i].ID == id {
				return &sessions[i]
			}
		}
		return nil
	}
	for i := range sessions {
		if sessions[i].SessionName == arg {
			return &sessions[i]
		}
	}
	return nil
}

func Upsert(sessions []Session, pane PaneInfo, status string) []Session {
	now := time.Now().UTC().Format(time.RFC3339)
	maxID := 0
	for i, s := range sessions {
		if s.ID > maxID {
			maxID = s.ID
		}
		if s.SessionName == pane.SessionName && s.PaneID == pane.PaneID {
			sessions[i].Status = status
			sessions[i].UpdatedAt = now
			sessions[i].WindowIndex = pane.WindowIndex
			sessions[i].WindowName = pane.WindowName
			return sessions
		}
	}
	projectDir, _ := os.Getwd()
	return append(sessions, Session{
		ID:           maxID + 1,
		SessionName:  pane.SessionName,
		WindowIndex:  pane.WindowIndex,
		WindowName:   pane.WindowName,
		PaneID:       pane.PaneID,
		ProjectDir:   projectDir,
		Status:       status,
		RegisteredAt: now,
		UpdatedAt:    now,
	})
}

func LiveSessions() (map[string]bool, error) {
	out, err := exec.Command("tmux", "ls", "-F", "#{session_name}").Output()
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
