package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Go-Ducky/cli/internal/agent/tools"
	"github.com/Go-Ducky/cli/internal/provider"
)

type Callback interface {
	OnText(text string)

	OnToolStart(name string, args json.RawMessage)

	OnToolEnd(name string, result *tools.Result)

	OnStatus(msg string)

	OnComplete(response string, usage provider.Usage)
}

type Agent struct {
	provider    provider.Provider
	model       string
	system      string
	registry    *tools.Registry
	workDir     string
	autoApprove bool
	approver    func(desc string, args map[string]any) bool
	cfg         *Config
}

func (a *Agent) SetApprover(f func(desc string, args map[string]any) bool) {
	a.approver = f
}

func (a *Agent) SetAutoApprove(v bool) {
	a.autoApprove = v
}

func (a *Agent) SetProvider(p provider.Provider) { a.provider = p }

func (a *Agent) SetModel(model string) { a.model = model }

func (a *Agent) ProviderName() string {
	if a.provider != nil {
		return a.provider.Name()
	}
	return ""
}

func (a *Agent) ModelName() string { return a.model }

type Config struct {
	MaxIterations  int
	MaxOutputChars int
	AutoApprove    bool
	ExcludeDirs    []string
}

func New(p provider.Provider, model, system, workDir string, cfg *Config, reg *tools.Registry) *Agent {
	return &Agent{
		provider: p,
		model:    model,
		system:   system,
		registry: reg,
		workDir:  workDir,
		cfg:      cfg,
	}
}

func (a *Agent) Run(ctx context.Context, messages []provider.Message, cb Callback) (string, provider.Usage, error) {
	cfg := a.cfg
	if cfg == nil {
		cfg = &Config{MaxIterations: 20, MaxOutputChars: 12000}
	}
	maxIters := cfg.MaxIterations
	if maxIters <= 0 {
		maxIters = 20
	}
	maxOut := cfg.MaxOutputChars
	if maxOut <= 0 {
		maxOut = 12000
	}

	approval := func(desc string, args map[string]any) bool {
		if a.autoApprove {
			return true
		}
		if a.approver != nil {
			if cb != nil {
				cb.OnStatus("⏳ waiting for approval...")
			}
			ok := a.approver(desc, args)
			if !ok {
				if cb != nil {
					cb.OnStatus("✖ action denied by user")
				}
				return false
			}
			return true
		}
		if cb != nil {
			cb.OnStatus("auto-approved " + desc)
		}
		return true
	}
	tctx := &tools.Context{
		WorkDir:  a.workDir,
		Approval: approval,
		OnLog: func(msg string) {
			if cb != nil {
				cb.OnStatus(msg)
			}
		},
	}

	history := make([]provider.Message, 0, len(messages)+8)
	history = append(history, messages...)

	var fullText strings.Builder
	var totalUsage provider.Usage

	for iter := 0; iter < maxIters; iter++ {
		req := provider.ChatRequest{
			Model:    a.model,
			System:   a.system,
			Messages: history,
			Tools:    a.registry.ToProviderTools(),
		}

		var streamed strings.Builder
		var pendingCalls []provider.ToolCall

		req.Stream = func(ev provider.StreamEvent) error {
			switch ev.Type {
			case "text":
				streamed.WriteString(ev.Text)
				fullText.WriteString(ev.Text)
				if cb != nil {
					cb.OnText(ev.Text)
				}
			case "tool_use":
				pendingCalls = append(pendingCalls, *ev.ToolCall)
			}
			return nil
		}

		resp, err := a.provider.Chat(ctx, req)
		if err != nil {
			return fullText.String(), totalUsage, err
		}
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		if len(resp.ToolCalls) > 0 {
			pendingCalls = append(pendingCalls, resp.ToolCalls...)
		}

		if streamed.Len() == 0 && resp.Content != "" {
			streamed.WriteString(resp.Content)
			fullText.WriteString(resp.Content)
			if cb != nil {
				cb.OnText(resp.Content)
			}
		}

		assistantBlocks := []provider.ContentBlock{}
		if streamed.Len() > 0 {
			assistantBlocks = append(assistantBlocks, provider.ContentBlock{Type: "text", Text: streamed.String()})
		}
		for _, tc := range pendingCalls {
			assistantBlocks = append(assistantBlocks, provider.ContentBlock{
				Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Input,
			})
		}
		assistantMsg := provider.Message{Role: provider.RoleAssistant, Content: assistantBlocks}
		history = append(history, assistantMsg)

		if len(pendingCalls) == 0 {
			answer := fullText.String()
			if cb != nil {
				cb.OnComplete(answer, totalUsage)
			}
			return answer, totalUsage, nil
		}

		results := []provider.ContentBlock{}
		for _, tc := range pendingCalls {
			if cb != nil {
				cb.OnToolStart(tc.Name, tc.Input)
			}
			result := a.executeTool(ctx, tctx, tc)
			if len(result.Content) > maxOut {
				result.Content = result.Content[:maxOut] + "\n...[truncated]"
			}
			results = append(results, provider.ContentBlock{
				Type: "tool_result", ToolUseID: tc.ID, Content: result.Content,
			})
			if cb != nil {
				cb.OnToolEnd(tc.Name, result)
			}
		}
		history = append(history, provider.Message{Role: provider.RoleTool, Content: results})
	}

	return fullText.String(), totalUsage, errors.New("agent reached max iterations without completing")
}

func (a *Agent) executeTool(ctx context.Context, tctx *tools.Context, tc provider.ToolCall) *tools.Result {
	t, ok := a.registry.Get(tc.Name)
	if !ok {
		return &tools.Result{Content: fmt.Sprintf("Unknown tool: %s", tc.Name), IsError: true}
	}
	res, err := t.Execute(ctx, tctx, tc.Input)
	if err != nil {
		return &tools.Result{Content: "Tool error: " + err.Error(), IsError: true}
	}
	if res == nil {
		res = &tools.Result{Content: "(no output)"}
	}
	return res
}
