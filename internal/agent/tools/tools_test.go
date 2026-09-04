package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestCtx(t *testing.T) (*Context, string) {
	t.Helper()
	dir := t.TempDir()
	return &Context{WorkDir: dir, Approval: func(string, map[string]any) bool { return true }}, dir
}

func TestWriteReadEditRoundtrip(t *testing.T) {
	tctx, dir := newTestCtx(t)
	ctx := context.Background()

	// Write
	w := NewWrite()
	pres, err := w.Execute(ctx, tctx, json.RawMessage(`{"file_path":"hello.txt","content":"line1\nline2"}`))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if pres.IsError {
		t.Fatalf("write returned error: %s", pres.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, "hello.txt")); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	// Read
	r := NewRead()
	rres, err := r.Execute(ctx, tctx, json.RawMessage(`{"file_path":"hello.txt"}`))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if !strings.Contains(rres.Content, "line1") || !strings.Contains(rres.Content, "line2") {
		t.Fatalf("read content mismatch: %q", rres.Content)
	}

	// Edit
	e := NewEdit()
	eres, err := e.Execute(ctx, tctx, json.RawMessage(`{"file_path":"hello.txt","old_string":"line2","new_string":"CHANGED"}`))
	if err != nil {
		t.Fatalf("edit error: %v", err)
	}
	if eres.IsError {
		t.Fatalf("edit returned error: %s", eres.Content)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if !strings.Contains(string(data), "CHANGED") {
		t.Fatalf("edit did not apply: %q", string(data))
	}
}

func TestGlobFindsFiles(t *testing.T) {
	tctx, dir := newTestCtx(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("z"), 0o644)

	g := NewGlob()
	res, err := g.Execute(ctx, tctx, json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if !strings.Contains(res.Content, "a.go") || !strings.Contains(res.Content, "b.go") {
		t.Fatalf("glob did not find .go files: %q", res.Content)
	}
	if strings.Contains(res.Content, "c.txt") {
		t.Fatalf("glob should not match c.txt: %q", res.Content)
	}
}

func TestListDirectory(t *testing.T) {
	tctx, dir := newTestCtx(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	l := NewList()
	res, err := l.Execute(ctx, tctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if !strings.Contains(res.Content, "file.txt") || !strings.Contains(res.Content, "sub") {
		t.Fatalf("list content mismatch: %q", res.Content)
	}
}

func TestBashEcho(t *testing.T) {
	tctx, _ := newTestCtx(t)
	ctx := context.Background()
	b := NewBash()
	res, err := b.Execute(ctx, tctx, json.RawMessage(`{"command":"echo hello-ducky"}`))
	if err != nil {
		t.Fatalf("bash error: %v", err)
	}
	if res.IsError {
		t.Fatalf("bash returned error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello-ducky") {
		t.Fatalf("bash output mismatch: %q", res.Content)
	}
}
