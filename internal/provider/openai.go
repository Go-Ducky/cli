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

	"github.com/Go-Ducky/cli/internal/config"
)

// OpenAI is a provider for any OpenAI-compatible chat completions endpoint.
// When compatible is true, it uses the /chat/completions style endpoint with
// tools support — this covers OpenAI, OpenRouter, Groq, LM Studio, LocalAI,
// vLLM and many other self-hosted backends.
type OpenAI struct {
	baseURL    string
	apiKey     string
	model      string
	compatible bool
	name       string
	client     *http.Client
}

// NewOpenAI creates an OpenAI-compatible provider from config and auth.
// If compatible is true, the provider also supports custom base URLs
// (used by self-hosted / OpenAI-compatible servers) via env var.
func NewOpenAI(cfg *config.Config, auth *config.Auth, compatible bool) *OpenAI {
	apiKey := cfg.OpenAI.APIKey
	if apiKey == "" && auth != nil {
		apiKey = auth.OpenAIAPIKey
	}
	if apiKey == "" && cfg.OpenAI.EnvKey != "" {
		apiKey = os.Getenv(cfg.OpenAI.EnvKey)
	}
	base := cfg.OpenAI.BaseURL
	if env := os.Getenv("OPENAI_BASE_URL"); env != "" {
		base = env
	}
	return &OpenAI{
		baseURL:    base,
		apiKey:     apiKey,
		model:      cfg.OpenAI.Model,
		compatible: compatible,
		client:     &http.Client{},
	}
}

func (o *OpenAI) Name() string {
	if o.name != "" {
		return o.name
	}
	if o.compatible {
		return "openai-compatible"
	}
	return "openai"
}

type openAIChatMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Index    int            `json:"index"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string        `json:"type"`
	Function openAIFuncDef `json:"function"`
}

type openAIFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Tools       []openAITool        `json:"tools,omitempty"`
	Stream      bool                `json:"stream"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
}

type openAIChoice struct {
	Message      openAIChatMessage  `json:"message"`
	Delta        *openAIChatMessage `json:"delta,omitempty"`
	FinishReason string             `json:"finish_reason"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta        openAIChatMessage `json:"delta"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (o *OpenAI) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}

	msgs := []openAIChatMessage{}
	if req.System != "" {
		msgs = append(msgs, openAIChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			msgs = append(msgs, openAIChatMessage{Role: "system", Content: m.Text()})
		case RoleUser:
			msgs = append(msgs, openAIChatMessage{Role: "user", Content: m.Text()})
		case RoleAssistant:
			msg := openAIChatMessage{Role: "assistant", Content: m.Text()}
			for _, blk := range m.Content {
				if blk.Type == "tool_use" {
					msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
						ID:   blk.ID,
						Type: "function",
						Function: openAIFunction{
							Name:      blk.Name,
							Arguments: string(blk.Input),
						},
					})
				}
			}
			msgs = append(msgs, msg)
		case RoleTool:
			for _, blk := range m.Content {
				if blk.Type == "tool_result" {
					msgs = append(msgs, openAIChatMessage{
						Role:       "tool",
						ToolCallID: blk.ToolUseID,
						Content:    blk.Content,
					})
				}
			}
		}
	}

	tools := make([]openAITool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIFuncDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	payload := openAIRequest{
		Model:    model,
		Messages: msgs,
		Tools:    tools,
		Stream:   req.Stream != nil,
	}
	if req.Temperature != nil {
		payload.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		payload.MaxTokens = &mt
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(o.baseURL, "/") + "/chat/completions"
	if !o.compatible && strings.Contains(o.baseURL, "openai.com") {
		endpoint = strings.TrimRight(o.baseURL, "/") + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	if req.Stream != nil {
		return o.streamChat(ctx, httpReq, payload, req.Stream)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out openAIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s error: %s", o.Name(), out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s error (%d)", o.Name(), resp.StatusCode)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty response", o.Name())
	}

	msg := openAIToMessage(out.Choices[0].Message)
	usage := Usage{}
	if out.Usage != nil {
		usage.InputTokens = out.Usage.PromptTokens
		usage.OutputTokens = out.Usage.CompletionTokens
	}
	return &ChatResponse{
		Content:    msg.Text(),
		Message:    msg,
		StopReason: out.Choices[0].FinishReason,
		Usage:      usage,
	}, nil
}

func (o *OpenAI) streamChat(ctx context.Context, httpReq *http.Request, payload openAIRequest, cb func(StreamEvent) error) (*ChatResponse, error) {
	httpReq.Body = io.NopCloser(bytes.NewReader(mustMarshal(payload)))
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%s error (%d): %s", o.Name(), resp.StatusCode, strings.TrimSpace(string(b)))
	}

	type accToolCall struct {
		id        string
		name      string
		arguments []string
	}
	reader := bufio.NewReader(resp.Body)
	var usage Usage
	var pending []*accToolCall

	flushTool := func(tc *accToolCall) error {
		input := json.RawMessage("{}")
		if len(tc.arguments) > 0 {
			joined := strings.Join(tc.arguments, "")
			if strings.TrimSpace(joined) != "" {
				input = json.RawMessage(joined)
			}
		}
		return cb(StreamEvent{
			Type: "tool_use",
			ToolCall: &ToolCall{
				ID:    tc.id,
				Name:  tc.name,
				Input: input,
			},
		})
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				return nil, fmt.Errorf("%s: %s", o.Name(), chunk.Error.Message)
			}
			if chunk.Usage != nil {
				usage.InputTokens = chunk.Usage.PromptTokens
				usage.OutputTokens = chunk.Usage.CompletionTokens
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			if content := deltaString(choice.Delta.Content); content != "" {
				if err := cb(StreamEvent{Type: "text", Text: content}); err != nil {
					return nil, err
				}
			}

			for _, tc := range choice.Delta.ToolCalls {
				for len(pending) <= tc.Index {
					pending = append(pending, &accToolCall{})
				}
				acc := pending[tc.Index]
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.arguments = append(acc.arguments, tc.Function.Arguments)
				}
			}

			if choice.FinishReason != "" {
				break
			}
		}
		if err == io.EOF {
			break
		}
	}

	for _, acc := range pending {
		if acc != nil && acc.name != "" {
			if err := flushTool(acc); err != nil {
				return nil, err
			}
		}
	}
	return &ChatResponse{Usage: usage}, nil
}

// deltaString extracts a non-empty string from an OpenAI delta content value.
func deltaString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return ""
	}
}

// openAIToMessage converts an OpenAI message into our Message type.
func openAIToMessage(m openAIChatMessage) Message {
	var blocks []ContentBlock
	if m.Content != nil {
		if s, ok := m.Content.(string); ok && s != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: s})
		}
	}
	for _, tc := range m.ToolCalls {
		var input json.RawMessage
		if len(tc.Function.Arguments) > 0 {
			input = json.RawMessage(tc.Function.Arguments)
		} else {
			input = json.RawMessage("{}")
		}
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return Message{Role: RoleAssistant, Content: blocks}
}

// ListModels lists models for OpenAI-compatible endpoints that support it.
func (o *OpenAI) ListModels(ctx context.Context) ([]string, error) {
	url := strings.TrimRight(o.baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil // models listing unsupported
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		names = append(names, m.ID)
	}
	return names, nil
}
