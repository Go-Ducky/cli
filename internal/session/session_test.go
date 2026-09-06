package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Go-Ducky/cli/internal/provider"
)

func testIsolated(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestSaveAndList(t *testing.T) {
	testIsolated(t)

	s := &Session{
		Provider: "ollama",
		Model:    "qwen2.5-coder:7b",
		WorkDir:  "/tmp/foo",
		Messages: []provider.Message{
			provider.NewTextMessage(provider.RoleUser, "hello"),
			provider.NewTextMessage(provider.RoleAssistant, "hi there"),
		},
	}
	if err := Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasPrefix(s.Name, "chat-") {
		t.Fatalf("empty name should get an auto name, got %q", s.Name)
	}
	if err := Save(&Session{Name: "named", Messages: []provider.Message{provider.NewTextMessage(provider.RoleUser, "x")}}); err != nil {
		t.Fatalf("save named: %v", err)
	}
	list, err := List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "named" || !strings.HasPrefix(list[1].Name, "chat-") {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list[1].Provider != "ollama" {
		t.Fatalf("ollama session lost provider: %+v", list[1])
	}
}

func TestResolve(t *testing.T) {
	testIsolated(t)
	for i, name := range []string{"alpha", "beta", "gamma"} {
		s := &Session{Name: name, WorkDir: "/w", Messages: []provider.Message{provider.NewTextMessage(provider.RoleUser, "x")}}
		if i > 0 {
			time.Sleep(2 * time.Millisecond)
		}
		if err := Save(s); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	if _, err := Resolve("alpha"); err != nil {
		t.Fatalf("resolve by exact name: %v", err)
	}
	s, err := Resolve("1")
	if err != nil {
		t.Fatalf("resolve by number: %v", err)
	}
	if s.Name != "gamma" {
		t.Fatalf("number 1 should be newest (gamma), got %q", s.Name)
	}
	s, err = Resolve("GAM")
	if err != nil || s.Name != "gamma" {
		t.Fatalf("resolve by unique fragment: got %v %v", s, err)
	}
	if _, err := Resolve("nope"); err == nil {
		t.Fatal("expected error resolving unknown name")
	}
}

func TestRename(t *testing.T) {
	testIsolated(t)
	if err := Save(&Session{Name: "old name", WorkDir: "/w", Messages: []provider.Message{provider.NewTextMessage(provider.RoleUser, "x")}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := Rename("old name", "new name"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := Resolve("old name"); err == nil {
		t.Fatal("old name should be gone after rename")
	}
	s, err := Resolve("new name")
	if err != nil {
		t.Fatalf("resolve renamed: %v", err)
	}
	if s.Name != "new name" {
		t.Fatalf("name mismatch: %q", s.Name)
	}
	if _, err := os.Stat(mustPath("old name")); !os.IsNotExist(err) {
		t.Fatal("old file should be removed")
	}
	if err := Rename("new name", "new name"); err != nil {
		t.Fatalf("rename to same name should be a no-op, got %v", err)
	}
}

func mustPath(name string) string {
	p, err := PathFor(name)
	if err != nil {
		panic(err)
	}
	return p
}
