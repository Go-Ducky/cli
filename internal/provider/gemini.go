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

// Gemini is a provider for Google's Gemini API.
type Gemini struct {
	apiKey string
	model  string
	client *http.Client
}

// NewGemini creates a Gemini provider from config and auth.
func NewGemini(cfg *config.Config, auth *config.Auth) *Gemini {
	apiKey := cfg.Gemini.APIKey
	if apiKey == "" && auth != nil {
		apiKey = auth.GeminiAPIKey
	}
	if apiKey == "" && cfg.Gemini.EnvKey != "" {
		apiKey = os.Getenv(cfg.Gemini.EnvKey)
	}
	return &Gemini{
		apiKey: apiKey,
		model:  cfg.Gemini.Model,
		client: &http.Client{},
	}
}

func (g *Gemini) Name() string { return "gemini" }

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *struct {
		Parts []geminiPart `json:"parts"`
	} `json:"systemInstruction,omitempty"`
	Tools            []geminiTool `json:"tools,omitempty"`
	GenerationConfig struct {
		Temperature     *float64 `json:"temperature,omitempty"`
		MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	} `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (g *Gemini) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = g.model
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	var contents []geminiContent
	var sysParts []geminiPart

	if req.System != "" {
		sysParts = append(sysParts, geminiPart{Text: req.System})
	}

	for _, m := range req.Messages {
		role := m.Role
		gemRole := "user"
		if role == RoleAssistant {
			gemRole = "model"
		} else if role == RoleTool {
			gemRole = "function"
		}

		var parts []geminiPart
		for _, blk := range m.Content {
			switch blk.Type {
			case "text":
				if blk.Text != "" {
					parts = append(parts, geminiPart{Text: blk.Text})
				}
			case "tool_use":
				var args any
				json.Unmarshal(blk.Input, &args)
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{Name: blk.Name, Args: args},
				})
			case "tool_result":
				var resp any = map[string]string{"result": blk.Content}
				parts = append(parts, geminiPart{
					FunctionResponse: &geminiFunctionResponse{
						Name:     blk.ToolUseID,
						Response: resp,
					},
				})
			}
		}

		// For tool results, append to existing function role content if it exists.
		if role == RoleTool && len(contents) > 0 && contents[len(contents)-1].Role == "function" {
			contents[len(contents)-1].Parts = append(contents[len(contents)-1].Parts, parts...)
			continue
		}

		if len(parts) == 0 {
			if m.Text() != "" {
				parts = append(parts, geminiPart{Text: m.Text()})
			}
		}
		if len(parts) > 0 {
			contents = append(contents, geminiContent{Role: gemRole, Parts: parts})
		}
	}

	tools := []geminiTool{}
	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		tools = append(tools, geminiTool{FunctionDeclarations: decls})
	}

	payload := geminiGenerateContentRequest{
		Contents: contents,
		Tools:    tools,
	}
	if len(sysParts) > 0 {
		payload.SystemInstruction = &struct {
			Parts []geminiPart `json:"parts"`
		}{Parts: sysParts}
	}
	if req.Temperature != nil {
		payload.GenerationConfig.Temperature = req.Temperature
	}
	mt := maxTokens
	payload.GenerationConfig.MaxOutputTokens = &mt

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		urlEscapePath(model), g.apiKey)

	if req.Stream != nil {
		url = fmt.Sprintf(
			"https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s",
			urlEscapePath(model), g.apiKey)
		return g.streamChat(ctx, url, payload, req.Stream)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out geminiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini error (%d)", resp.StatusCode)
	}
	if len(out.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates (block reason %q)", out.PromptFeedback.BlockReason)
	}

	msg := geminiToMessage(out.Candidates[0].Content)
	return &ChatResponse{
		Content:    msg.Text(),
		Message:    msg,
		StopReason: out.Candidates[0].FinishReason,
	}, nil
}

func (g *Gemini) streamChat(ctx context.Context, url string, payload geminiGenerateContentRequest, cb func(StreamEvent) error) (*ChatResponse, error) {
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gemini error (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var chunk geminiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("gemini: %s", chunk.Error.Message)
		}
		if len(chunk.Candidates) == 0 {
			continue
		}
		msg := geminiToMessage(chunk.Candidates[0].Content)
		if msg.Text() != "" {
			if err := cb(StreamEvent{Type: "text", Text: msg.Text()}); err != nil {
				return nil, err
			}
		}
		for _, blk := range msg.Content {
			if blk.Type == "tool_use" {
				if err := cb(StreamEvent{
					Type: "tool_use",
					ToolCall: &ToolCall{
						ID:    blk.ID,
						Name:  blk.Name,
						Input: blk.Input,
					},
				}); err != nil {
					return nil, err
				}
			}
		}
		if chunk.Candidates[0].FinishReason != "" {
			break
		}
	}
	return &ChatResponse{}, scanner.Err()
}

func geminiToMessage(c geminiContent) Message {
	var out []ContentBlock
	for _, p := range c.Parts {
		switch {
		case p.Text != "":
			out = append(out, ContentBlock{Type: "text", Text: p.Text})
		case p.FunctionCall != nil:
			var raw json.RawMessage
			raw, _ = json.Marshal(p.FunctionCall.Args)
			out = append(out, ContentBlock{
				Type:  "tool_use",
				ID:    "gemini-" + p.FunctionCall.Name,
				Name:  p.FunctionCall.Name,
				Input: raw,
			})
		}
	}
	return Message{Role: RoleAssistant, Content: out}
}

// ListModels lists models via the Gemini API.
func (g *Gemini) ListModels(ctx context.Context) ([]string, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + g.apiKey
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
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

func urlEscapePath(s string) string {
	r := strings.NewReplacer("/", "%2F", ":", "%3A")
	return r.Replace(s)
}
