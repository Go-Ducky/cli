package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Go-Ducky/cli/internal/agent/tools"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi from mcp"), 0o600); err != nil {
		t.Fatal(err)
	}
	return New(dir, tools.DefaultRegistry(), "0.0.0-test")
}

func runServer(t *testing.T, s *Server, msgs ...string) string {
	t.Helper()
	var in, out bytes.Buffer
	for _, m := range msgs {
		in.WriteString(m + "\n")
	}
	if err := s.Run(context.Background(), &in, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestServerInitialize(t *testing.T) {
	s := newTestServer(t)
	out := runServer(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	for _, want := range []string{`"goducky"`, `2024-11-05`, `"serverInfo"`, `"id":1`} {
		if !strings.Contains(out, want) {
			t.Fatalf("initialize reply missing %q: %s", want, out)
		}
	}
}

func TestServerPingAndNotifications(t *testing.T) {
	s := newTestServer(t)
	out := runServer(t, s,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if strings.Contains(out, `"notifications/initialized"`) {
		t.Fatalf("notifications must not produce a reply: %s", out)
	}
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("ping reply missing: %s", out)
	}
}

func TestServerToolsCall(t *testing.T) {
	s := newTestServer(t)
	out := runServer(t, s,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read","arguments":{"file_path":"hello.txt"}}}`,
	)
	if !strings.Contains(out, `"grep"`) || !strings.Contains(out, `"inputSchema"`) {
		t.Fatalf("tools/list incomplete: %s", out)
	}
	if !strings.Contains(out, `hi from mcp`) {
		t.Fatalf("tools/call didn't return file content: %s", out)
	}
	if !strings.Contains(out, `"id":4`) {
		t.Fatalf("tools/call reply missing: %s", out)
	}
}

func TestServerUnknownTool(t *testing.T) {
	s := newTestServer(t)
	out := runServer(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	var resp struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatalf("expected a JSON-RPC error, got: %s", out)
	}
}
