package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/provider"
)

type Session struct {
	Name      string             `json:"name"`
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	WorkDir   string             `json:"work_dir"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []provider.Message `json:"messages"`
}

func Dir() (string, error) {
	d, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "sessions"), nil
}

func Save(s *Session) error {
	if strings.TrimSpace(s.Name) == "" {
		s.Name = AutoName()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	s.UpdatedAt = time.Now()
	path, err := PathFor(s.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func PathFor(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeName(name)+".json"), nil
}

func AutoName() string {
	return "chat-" + time.Now().Format("2006-01-02-15-04")
}

func List() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Session{}, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt) })
	return sessions, nil
}

func Resolve(arg string) (*Session, error) {
	sessions, err := List()
	if err != nil {
		return nil, err
	}
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, errors.New("no session specified")
	}
	if n, nerr := strconv.Atoi(arg); nerr == nil {
		if n < 1 || n > len(sessions) {
			return nil, fmt.Errorf("no session #%d (you have %d saved)", n, len(sessions))
		}
		return &sessions[n-1], nil
	}
	for i := range sessions {
		if sessions[i].Name == arg {
			return &sessions[i], nil
		}
	}
	low := strings.ToLower(arg)
	var found []*Session
	for i := range sessions {
		if strings.Contains(strings.ToLower(sessions[i].Name), low) {
			found = append(found, &sessions[i])
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) > 1 {
		names := make([]string, 0, len(found))
		for _, s := range found {
			names = append(names, s.Name)
		}
		return nil, fmt.Errorf("%q matches multiple sessions: %s — use a full name or one of their numbers", arg, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("no session named %q", arg)
}

func Load(arg string) (*Session, error) {
	return Resolve(arg)
}

func Rename(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("new name cannot be empty")
	}
	s, err := Load(oldName)
	if err != nil {
		return err
	}
	oldPath, err := PathFor(s.Name)
	if err != nil {
		return err
	}
	newPath, err := PathFor(newName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(newPath); err == nil && oldPath != newPath {
		return fmt.Errorf("a session named %q already exists", newName)
	}
	s.Name = newName
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(newPath, data, 0o600); err != nil {
		return err
	}
	return os.Remove(oldPath)
}

func PrintList() error {
	sessions, err := List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("No saved chats yet. Chats are saved automatically when you quit the TUI, or with /save <name>.")
		return nil
	}
	fmt.Println("Saved chats (newest first):")
	for i, s := range sessions {
		wd := s.WorkDir
		if wd == "" {
			wd = "?"
		}
		fmt.Printf("  %2d. %-40s %s / %s  (%s)  %s\n", i+1, s.Name, s.Provider, s.Model, wd, s.UpdatedAt.Format("2006-01-02 15:04"))
	}
	fmt.Println("\nResume with:  goducky resume <number-or-name>")
	fmt.Println("Rename with:  goducky rename <number-or-name> <new-name>")
	return nil
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ' ':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	out := strings.TrimSpace(sb.String())
	if out == "" || out == "." || out == ".." {
		return "chat"
	}
	return out
}
