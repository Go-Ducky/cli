package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Go-Ducky/cli/internal/config"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

func NewTextMessage(role Role, text string) Message {
	return Message{Role: role, Content: []ContentBlock{{Type: "text", Text: text}}}
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

type StreamEvent struct {
	Type       string
	Text       string
	Model      string
	ToolCall   *ToolCall
	StopReason string
}

type ChatRequest struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []Tool
	Temperature *float64
	MaxTokens   int
	Stream      func(StreamEvent) error
}

type Provider interface {
	Name() string

	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	ListModels(ctx context.Context) ([]string, error)
}

type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	Message    Message
	StopReason string
	Usage      Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

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
	case "openrouter":
		return NewOpenRouter(cfg, auth), nil
	case "opencode":
		return NewOpenCodeZen(cfg, auth), nil
	case "anthropic":
		return NewAnthropic(cfg, auth), nil
	case "gemini":
		return NewGemini(cfg, auth), nil
	default:
		return nil, fmt.Errorf("%w: %q", config.ErrUnknownProvider, cfg.Provider)
	}
}

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
	case strings.Contains(want, "openrouter"):
		return cfg.OpenRouter.Model
	case strings.Contains(want, "opencode"):
		return cfg.OpenCode.Model
	case strings.Contains(want, "anthropic"):
		return cfg.Anthropic.Model
	case strings.Contains(want, "gemini"):
		return cfg.Gemini.Model
	}
	return cfg.Model
}

var ErrNoAPIKey = errors.New("no API key configured")
