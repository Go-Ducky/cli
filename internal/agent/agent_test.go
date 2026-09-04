package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/Go-Ducky/cli/internal/agent/tools"
	"github.com/Go-Ducky/cli/internal/provider"
)

// mockProvider simulates a model that first calls a tool, then answers.
type mockProvider struct {
	firstCall bool
	gotTools  []string
	toolNum   int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) ListModels(ctx context.Context) ([]string, error) { return nil, nil }

func (m *mockProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	for _, t := range req.Tools {
		m.gotTools = append(m.gotTools, t.Name)
	}
	if !m.firstCall {
		m.firstCall = true
		// First call: request a bash tool call.
		m.toolNum++
		id := "t" + itoa(m.toolNum)
		return &provider.ChatResponse{
			Message: provider.Message{
				Role: provider.RoleAssistant,
				Content: []provider.ContentBlock{
					{Type: "tool_use", ID: id, Name: "bash", Input: json.RawMessage(`{"command":"echo hello"}`)},
				},
			},
			ToolCalls: []provider.ToolCall{
				{ID: id, Name: "bash", Input: json.RawMessage(`{"command":"echo hello"}`)},
			},
		}, nil
	}
	// Second call: final answer.
	return &provider.ChatResponse{
		Content: "done",
		Message: provider.NewTextMessage(provider.RoleAssistant, "done"),
	}, nil
}

type testCallback struct {
	text      string
	toolStart []string
}

func (c *testCallback) OnText(text string)                      { c.text += text }
func (c *testCallback) OnToolStart(n string, _ json.RawMessage) { c.toolStart = append(c.toolStart, n) }
func (c *testCallback) OnToolEnd(n string, r *tools.Result)     {}
func (c *testCallback) OnStatus(msg string)                     {}
func (c *testCallback) OnComplete(s string, u provider.Usage)   {}

// Since itoa is in the tools package, define a local one.
func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestAgentRunsToolThenAnswers(t *testing.T) {
	mp := &mockProvider{}
	reg := tools.NewRegistry(tools.NewBash())
	cfg := &Config{MaxIterations: 10}
	a := New(mp, "mock-model", "You are a test agent", t.TempDir(), cfg, reg)
	cb := &testCallback{}

	msgs := []provider.Message{provider.NewTextMessage(provider.RoleUser, "hello")}
	result, _, err := a.Run(context.Background(), msgs, cb)
	if err != nil {
		t.Fatalf("agent.Run error: %v", err)
	}
	if result != "done" {
		t.Fatalf("expected result 'done', got %q", result)
	}
	if len(cb.toolStart) != 1 || cb.toolStart[0] != "bash" {
		t.Fatalf("expected 1 bash tool start, got %v", cb.toolStart)
	}
	if len(mp.gotTools) == 0 || mp.gotTools[0] != "bash" {
		t.Fatalf("bashtool was not advertised to provider, got %v", mp.gotTools)
	}
}

func TestAgentStopsWhenNoToolCalls(t *testing.T) {
	// Provider always returns text with no tool calls.
	p := &textOnlyProvider{}
	reg := tools.DefaultRegistry()
	a := New(p, "mock", "test", t.TempDir(), &Config{}, reg)
	msgs := []provider.Message{provider.NewTextMessage(provider.RoleUser, "hi")}
	result, _, err := a.Run(context.Background(), msgs, &testCallback{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "answer" {
		t.Fatalf("expected 'answer', got %q", result)
	}
}

type textOnlyProvider struct{}

func (m *textOnlyProvider) Name() string                                     { return "text" }
func (m *textOnlyProvider) ListModels(ctx context.Context) ([]string, error) { return nil, nil }
func (m *textOnlyProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{
		Content: "answer",
		Message: provider.NewTextMessage(provider.RoleAssistant, "answer"),
	}, nil
}
