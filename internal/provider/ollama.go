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

type Ollama struct {
	host   string
	model  string
	client *http.Client
}

func NewOllama(cfg *config.Config) *Ollama {
	host := cfg.Ollama.Host
	if env := os.Getenv("OLLAMA_HOST"); env != "" {
		host = env
	}
	return &Ollama{
		host:   host,
		model:  cfg.Ollama.Model,
		client: &http.Client{},
	}
}

func (o *Ollama) Name() string { return "ollama" }

type ollamaMessage struct {
	Role    string            `json:"role"`
	Content string            `json:"content"`
	Images  []string          `json:"images,omitempty"`
	Tools   []json.RawMessage `json:"-,"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Tools    []ollamaTool        `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message    ollamaChatMessage `json:"message"`
	Done       bool              `json:"done"`
	Error      string            `json:"error"`
	PromptEval int               `json:"prompt_eval_count"`
	Eval       int               `json:"eval_count"`
}

func (o *Ollama) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}

	msgs := make([]ollamaChatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, ollamaChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			msgs = append(msgs, ollamaChatMessage{Role: "system", Content: m.Text()})
		case RoleUser:
			msgs = append(msgs, ollamaChatMessage{Role: "user", Content: m.Text()})
		case RoleAssistant:
			msg := ollamaChatMessage{Role: "assistant", Content: m.Text()}
			for _, blk := range m.Content {
				if blk.Type == "tool_use" {
					msg.ToolCalls = append(msg.ToolCalls, ollamaToolCall{
						Function: struct {
							Name      string          `json:"name"`
							Arguments json.RawMessage `json:"arguments"`
						}{Name: blk.Name, Arguments: blk.Input},
					})
				}
			}
			msgs = append(msgs, msg)
		case RoleTool:

			content := ""
			for _, blk := range m.Content {
				if blk.Type == "tool_result" {
					txt := "Tool result: " + blk.Content
					if blk.ToolUseID != "" {
						txt = fmt.Sprintf("Tool %s result: %s", blk.ToolUseID, blk.Content)
					}
					content = txt
				} else {
					content += blk.Text
				}
			}
			msgs = append(msgs, ollamaChatMessage{Role: "user", Content: content})
		}
	}

	tools := make([]ollamaTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	payload := ollamaRequest{
		Model:    model,
		Messages: msgs,
		Tools:    tools,
		Stream:   req.Stream != nil,
		Options:  map[string]any{},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(o.host, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if req.Stream != nil {
		return o.streamChat(ctx, httpReq, payload, req.Stream)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, friendlyOllamaError(fmt.Errorf("ollama error (%d): %s", resp.StatusCode, strings.TrimSpace(string(b))))
	}

	var out ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, friendlyOllamaError(fmt.Errorf("ollama: %s", out.Error))
	}

	respMsg := ollamaToMessage(out.Message)
	return &ChatResponse{
		Content: out.Message.Content,
		Message: respMsg,
		Usage: Usage{
			InputTokens:  out.PromptEval,
			OutputTokens: out.Eval,
		},
	}, nil
}

func (o *Ollama) streamChat(ctx context.Context, httpReq *http.Request, payload ollamaRequest, cb func(StreamEvent) error) (*ChatResponse, error) {
	httpReq.Body = io.NopCloser(bytes.NewReader(mustMarshal(payload)))
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, friendlyOllamaError(fmt.Errorf("ollama error (%d): %s", resp.StatusCode, strings.TrimSpace(string(b))))
	}

	var usage Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk ollamaResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if chunk.Error != "" {
			return nil, friendlyOllamaError(fmt.Errorf("ollama: %s", chunk.Error))
		}
		if chunk.PromptEval > 0 {
			usage.InputTokens = chunk.PromptEval
		}
		if chunk.Eval > 0 {
			usage.OutputTokens = chunk.Eval
		}

		if chunk.Message.Content != "" {
			if err := cb(StreamEvent{Type: "text", Text: chunk.Message.Content}); err != nil {
				return nil, err
			}
		}
		for _, tc := range chunk.Message.ToolCalls {
			var input json.RawMessage
			if len(tc.Function.Arguments) > 0 {
				input = tc.Function.Arguments
			} else {
				input = json.RawMessage("{}")
			}
			if err := cb(StreamEvent{
				Type: "tool_use",
				ToolCall: &ToolCall{
					ID:    "ollama-" + tc.Function.Name,
					Name:  tc.Function.Name,
					Input: input,
				},
			}); err != nil {
				return nil, err
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &ChatResponse{
		Usage: usage,
	}, nil
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func friendlyOllamaError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "load failed") || strings.Contains(msg, "terminated") ||
		strings.Contains(msg, "exit status") || strings.Contains(msg, "insufficient") ||
		strings.Contains(msg, "out of memory") {
		return fmt.Errorf("%w\n→ The model failed to load in Ollama. Try a smaller model (e.g. qwen2.5-coder:3b), close other apps, or run `ollama rm <model>` and pull it again.", err)
	}
	return err
}

func ollamaToMessage(m ollamaChatMessage) Message {
	var blocks []ContentBlock
	if m.Content != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: m.Content})
	}
	for _, tc := range m.ToolCalls {
		var input json.RawMessage
		if len(tc.Function.Arguments) > 0 {
			input = tc.Function.Arguments
		} else {
			input = json.RawMessage("{}")
		}
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    "ollama-" + tc.Function.Name,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return Message{Role: RoleAssistant, Content: blocks}
}

func (o *Ollama) ListModels(ctx context.Context) ([]string, error) {
	url := strings.TrimRight(o.host, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: listing models failed: %d", resp.StatusCode)
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

func (o *Ollama) HasModel(ctx context.Context, name string) bool {
	models, err := o.ListModels(ctx)
	if err != nil {
		return false
	}
	for _, m := range models {
		if m == name {
			return true
		}
	}
	return false
}

func (o *Ollama) Pull(ctx context.Context, name string) error {
	payload, err := json.Marshal(map[string]any{"name": name, "stream": true})
	if err != nil {
		return err
	}
	url := strings.TrimRight(o.host, "/") + "/api/pull"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: could not reach %s: %w", o.host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ollama pull %s failed (%d): %s", name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Error != "" {
			return fmt.Errorf("ollama pull %s: %s (check the model name at https://ollama.com/library)", name, chunk.Error)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (o *Ollama) Remove(ctx context.Context, name string) error {
	payload, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return err
	}
	url := strings.TrimRight(o.host, "/") + "/api/delete"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama rm: could not reach %s: %w", o.host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ollama rm %s failed (%d): %s", name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (m Message) Text() string {
	var sb strings.Builder
	for _, blk := range m.Content {
		if blk.Type == "text" {
			sb.WriteString(blk.Text)
		}
	}
	return sb.String()
}
