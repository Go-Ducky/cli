package tui

import (
	"encoding/json"

	"github.com/Go-Ducky/cli/internal/agent"
	"github.com/Go-Ducky/cli/internal/agent/tools"
	"github.com/Go-Ducky/cli/internal/provider"
)

type tuiCallback struct {
	m *model
}

func (c *tuiCallback) send(m any) {
	if c.m != nil && c.m.program != nil {
		c.m.program.Send(m)
	}
}

func (c *tuiCallback) OnText(text string) {
	c.send(streamMsg{text: text})
}

func (c *tuiCallback) OnToolStart(name string, args json.RawMessage) {
	c.send(statusMsg{text: "⚙ running " + name})
}

func (c *tuiCallback) OnToolEnd(name string, result *tools.Result) {
	c.send(toolMsg{name: name, content: result.Content, isError: result.IsError})
}

func (c *tuiCallback) OnStatus(msg string) {
	c.send(statusMsg{text: msg})
}

func (c *tuiCallback) OnComplete(response string, usage provider.Usage) {
}

var _ agent.Callback = (*tuiCallback)(nil)
