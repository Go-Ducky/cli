package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Go-Ducky/goducky-cli/internal/config"
)

// Anthropic is a provider for Anthropic's Claude API.
type Anthropic struct {
	apiKey string
	model  string
	client *http.Client
}

// NewAnthropic creates an Anthropic provider from config and auth.
func NewAnthropic(cfg *config.Config, auth *config.Auth) *Anthropic {
	apiKey := cfg.Anthropic.APIKey
	if apiKey == "" && auth != nil {
		apiKey = auth.AnthropicAPIKey
	}
	if apiKey == "" && cfg.Anthropic.EnvKey != "" {
		apiKey = os.Getenv(cfg.Anthropic.EnvKey)
	}
	return &Anthropic{
		apiKey: apiKey,
		model:  cfg.Anthropic.Model,
		client: &http.Client{},
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

type anthropicContentBlock struct {
	Type      string `json:"type"` // text | tool_use | tool_result | thinking
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = a.model
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	msgs := []anthropicMessage{}
	var pendingToolResults []anthropicMessage
	var system string

	if req.System != "" {
		system = req.System
	}

	// We track consecutive tool_results to merge into a single assistant
	// message as Anthropic requires tool_use + tool_result in the same turn.
	var tw []anthropicContentBlock // tool result blocks for current turn
	var twRole string
	flush := func() {
		if len(tw) == 0 {
			return
		}
		msgs = append(msgs, anthropicMessage{Role: twRole, Content: tw})
		tw = nil
	}

	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if system != "" {
				system += "\n\n"
			}
			system += m.Text()
		case RoleUser:
			flush()
			var blocks []anthropicContentBlock
			for _, blk := range m.Content {
				if blk.Type == "text" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: blk.Text})
				}
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Text()})
			}
			msgs = append(msgs, anthropicMessage{Role: "user", Content: blocks})
		case RoleAssistant:
			flush()
			var blocks []anthropicContentBlock
			for _, blk := range m.Content {
				switch blk.Type {
				case "text":
					if blk.Text != "" {
						blocks = append(blocks, anthropicContentBlock{Type: "text", Text: blk.Text})
					}
				case "tool_use":
					var input any
					json.Unmarshal(blk.Input, &input)
					blocks = append(blocks, anthropicContentBlock{
						Type: "tool_use", ID: blk.ID, Name: blk.Name, Input: input,
					})
				}
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Text()})
			}
			msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
		case RoleTool:
			for _, blk := range m.Content {
				if blk.Type != "tool_result" {
					continue
				}
				if twRole == "" {
					twRole = "user"
				}
				tw = append(tw, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: blk.ToolUseID,
					Content:   blk.Content,
				})
			}
		}
	}
	flush()

	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		schema := t.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	_ = pendingToolResults

	payload := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		System:    system,
		Tools:     tools,
		Stream:    req.Stream != nil,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	if req.Stream != nil {
		return a.streamChat(ctx, httpReq, payload, req.Stream)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out anthropicResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (%d)", resp.StatusCode)
	}

	msg := anthropicToMessage(out.Content)
	return &ChatResponse{
		Content:    msg.Text(),
		Message:    msg,
		StopReason: out.StopReason,
		Usage: Usage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
		},
	}, nil
}

// anthropicToMessage converts Anthropic content blocks into our Message type.
func anthropicToMessage(blocks []anthropicContentBlock) Message {
	var out []ContentBlock
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				out = append(out, ContentBlock{Type: "text", Text: b.Text})
			}
		case "tool_use":
			var raw json.RawMessage
			raw, _ = json.Marshal(b.Input)
			out = append(out, ContentBlock{Type: "tool_use", ID: b.ID, Name: b.Name, Input: raw})
		}
	}
	return Message{Role: RoleAssistant, Content: out}
}

func (a *Anthropic) streamChat(ctx context.Context, httpReq *http.Request, payload anthropicRequest, cb func(StreamEvent) error) (*ChatResponse, error) {
	httpReq.Body = io.NopCloser(bytes.NewReader(mustMarshal(payload)))
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var usage Usage
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var evt struct {
			Type  string `json:"type"`
			Delta *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			ContentBlock *struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Name  string `json:"name"`
				Input any    `json:"input"`
			} `json:"content_block"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if evt.Error != nil {
			return nil, fmt.Errorf("anthropic: %s", evt.Error.Message)
		}
		switch evt.Type {
		case "message_start":
			if evt.Usage != nil {
				usage.InputTokens = evt.Usage.InputTokens
			}
		case "content_block_start":
			if evt.ContentBlock != nil && evt.ContentBlock.Type == "tool_use" {
				var raw json.RawMessage
				raw, _ = json.Marshal(evt.ContentBlock.Input)
				if err := cb(StreamEvent{
					Type: "tool_use",
					ToolCall: &ToolCall{
						ID:    evt.ContentBlock.ID,
						Name:  evt.ContentBlock.Name,
						Input: raw,
					},
				}); err != nil {
					return nil, err
				}
			}
		case "content_block_delta":
			if evt.Delta != nil && evt.Delta.Type == "text_delta" && evt.Delta.Text != "" {
				if err := cb(StreamEvent{Type: "text", Text: evt.Delta.Text}); err != nil {
					return nil, err
				}
			}
		case "message_delta":
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &ChatResponse{Usage: usage}, nil
}

// ListModels is not supported by the Anthropic API without extra calls.
func (a *Anthropic) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}
