package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Go-Ducky/cli/internal/config"
)

// Role identifies who authored a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentBlock is a single piece of content in a message.
type ContentBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result | thinking

	// text
	Text string `json:"text,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role Role, text string) Message {
	return Message{Role: role, Content: []ContentBlock{{Type: "text", Text: text}}}
}

// Tool represents a callable tool exposed to the model.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON schema
}

// ToolCall is a request from the model to invoke a tool.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult is the outcome of a tool call.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// StreamEvent is emitted during a streaming response.
type StreamEvent struct {
	Type       string // text | tool_use | done | error
	Text       string
	Model      string
	ToolCall   *ToolCall
	StopReason string
}

// ChatRequest describes a call to the model.
type ChatRequest struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []Tool
	Temperature *float64
	MaxTokens   int
	Stream      func(StreamEvent) error
}

// Provider is the common interface implemented by every backend.
type Provider interface {
	Name() string
	// Chat sends a completion. When req.Stream is non-nil, events are delivered
	// via the callback and Chat returns when streaming completes.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// ListModels returns models available on the backend (may be empty if unsupported).
	ListModels(ctx context.Context) ([]string, error)
}

// ChatResponse is the non-streamed result.
type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	Message    Message
	StopReason string
	Usage      Usage
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// New builds a provider from config.
func New(cfg *config.Config, auth *config.Auth) (Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllama(cfg), nil
	case "groq":
		return NewGroq(cfg, auth, true), nil
	case "openai":
		return NewOpenAI(cfg, auth, false), nil
	case "openai_compatible":
		return NewOpenAI(cfg, auth, true), nil
	case "anthropic":
		return NewAnthropic(cfg, auth), nil
	case "gemini":
		return NewGemini(cfg, auth), nil
	default:
		return nil, fmt.Errorf("%w: %q", config.ErrUnknownProvider, cfg.Provider)
	}
}

// ResolveModel resolves the effective model name from a provider + explicit model.
func ResolveModel(cfg *config.Config, explicit string) string {
	if explicit != "" {
		return explicit
	}
	want := strings.ToLower(cfg.Provider)
	switch {
	case strings.Contains(want, "ollama"):
		return cfg.Ollama.Model
	case strings.Contains(want, "groq"):
		return cfg.Groq.Model
	case strings.Contains(want, "openai"):
		return cfg.OpenAI.Model
	case strings.Contains(want, "anthropic"):
		return cfg.Anthropic.Model
	case strings.Contains(want, "gemini"):
		return cfg.Gemini.Model
	}
	return cfg.Model
}

// ErrNoAPIKey is returned when a required key is missing.
var ErrNoAPIKey = errors.New("no API key configured")
