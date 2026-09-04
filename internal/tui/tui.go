package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Go-Ducky/cli/internal/agent"
	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/provider"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type streamMsg struct{ text string }
type statusMsg struct{ text string }
type toolMsg struct {
	name    string
	content string
	isError bool
}
type completeMsg struct{ text string }
type agentErrMsg struct{ err error }
type userMsg struct{ text string }
type approvalRequestMsg struct {
	desc    string
	args    map[string]any
	respond chan bool
}
type approvalAnswerMsg struct {
	approved bool
	respond  chan bool
}

type transcriptItem struct {
	kind string // user | assistant | tool
	text string
	meta bool // system greeting lines, rendered without a label and excluded from model context
}

type model struct {
	viewport viewport.Model
	input    textarea.Model
	height   int
	width    int

	items   []transcriptItem
	current string
	running bool
	status  string

	agent     *agent.Agent
	provider  string
	modelName string
	workDir   string

	cfg  *config.Config
	auth *config.Auth

	program         *tea.Program
	approvalChan    chan bool
	approvalPending bool
	pendingMessages []provider.Message
}

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	alertStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
)

// New creates the TUI model.
func New(a *agent.Agent, workDir, providerName, modelName string, cfg *config.Config, auth *config.Auth) *model {
	ta := textarea.New()
	ta.Placeholder = "Ask GoDucky... (Enter to send)"
	ta.Prompt = "❯ "
	ta.CharLimit = 10000
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(1)
	return &model{
		input:     ta,
		agent:     a,
		workDir:   workDir,
		provider:  providerName,
		modelName: modelName,
		cfg:       cfg,
		auth:      auth,
	}
}

// SetProgram wires the bubbletea program so callbacks can Send messages.
func (m *model) SetProgram(p *tea.Program) { m.program = p }

func (m *model) Init() tea.Cmd {
	m.addMetaItem("assistant", m.assistantName()+" ready. Type a message or /help. Ctrl+C to quit.")
	m.addMetaItem("assistant", fmt.Sprintf("Provider: %s  Model: %s  Dir: %s", m.provider, m.modelName, m.workDir))
	m.input.Focus()
	return tea.Batch(tea.EnterAltScreen, textarea.Blink)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
		m.viewport.SetContent(m.renderBody())
		if m.viewport.Height > 0 {
			m.input.SetWidth(m.width - 2)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "pgup":
			m.viewport.LineUp(viewportMouseWheelDelta)
			return m, nil
		case "pgdown":
			m.viewport.LineDown(viewportMouseWheelDelta)
			return m, nil
		case "home":
			m.viewport.GotoTop()
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			return m, nil
		}
		if m.running {
			return m, nil
		}

	case tea.MouseMsg:
		if m.viewport.Height > 0 {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case streamMsg:
		atBottom := m.viewport.AtBottom()
		m.current += msg.text
		m.viewport.SetContent(m.renderBody())
		if atBottom {
			m.viewport.GotoBottom()
		}
		return m, nil

	case statusMsg:
		m.status = msg.text
		return m, nil

	case toolMsg:
		m.current = ""
		txt := truncate(msg.content, 4000)
		label := "→ " + msg.name + ":\n"
		if msg.isError {
			label = "❌ " + msg.name + " failed:\n"
		}
		m.addItem("tool", label+txt)
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, nil

	case userMsg:
		if strings.HasPrefix(msg.text, "/") {
			cmd := m.handleCommand(msg.text)
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		m.addItem("user", msg.text)
		m.current = ""
		m.running = true
		m.status = "Thinking..."
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		msgs := m.toProviderMessages()
		msgs = append(msgs, provider.NewTextMessage(provider.RoleUser, msg.text))
		m.pendingMessages = msgs
		cmds = append(cmds, m.runAgent())

	case completeMsg:
		if m.current != "" {
			m.addItem("assistant", m.current)
		} else if msg.text != "" {
			m.addItem("assistant", msg.text)
		}
		m.current = ""
		m.running = false
		m.status = ""
		m.input.Focus()
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, textarea.Blink

	case agentErrMsg:
		m.addItem("assistant", "❌ "+msg.err.Error())
		m.running = false
		m.status = ""
		m.input.Focus()
		m.viewport.SetContent(m.renderBody())
		return m, textarea.Blink

	case approvalRequestMsg:
		m.status = "⚠ " + msg.desc + "? [Enter] approve  [Esc] deny"
		m.approvalChan = msg.respond
		m.approvalPending = true
		m.viewport.SetContent(m.renderBody())
		return m, nil

	case approvalAnswerMsg:
		if msg.respond != nil {
			msg.respond <- msg.approved
		}
		m.approvalPending = false
		m.approvalChan = nil
		if m.status != "" && strings.HasPrefix(m.status, "⚠ ") {
			m.status = ""
		}
		m.viewport.SetContent(m.renderBody())
		return m, nil
	}

	if m.approvalPending {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				return m, func() tea.Msg { return approvalAnswerMsg{approved: true, respond: m.approvalChan} }
			case "esc":
				return m, func() tea.Msg { return approvalAnswerMsg{approved: false, respond: m.approvalChan} }
			}
		}
		return m, nil
	}

	if !m.running {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				m.input.Reset()
				cmds = append(cmds, func() tea.Msg { return userMsg{text: text} })
			}
		}
	}

	return m, tea.Batch(cmds...)
}

const viewportMouseWheelDelta = 3

func (m *model) addItem(kind, text string) {
	m.items = append(m.items, transcriptItem{kind: kind, text: text})
}

func (m *model) addMetaItem(kind, text string) {
	m.items = append(m.items, transcriptItem{kind: kind, text: text, meta: true})
}

func (m *model) renderBody() string {
	var sb strings.Builder
	for _, it := range m.items {
		switch it.kind {
		case "user":
			sb.WriteString("\n" + userStyle.Render("You:") + "\n" + it.text + "\n")
		case "assistant":
			if it.text != "" {
				if it.meta {
					sb.WriteString("\n" + it.text + "\n")
				} else {
					sb.WriteString("\n" + assistantStyle.Render(m.assistantName()+":") + "\n" + it.text + "\n")
				}
			}
		case "tool":
			sb.WriteString("\n" + toolStyle.Render(it.text) + "\n")
		}
	}
	if m.current != "" {
		sb.WriteString("\n" + assistantStyle.Render(m.assistantName()+":") + "\n" + m.current)
	}
	return sb.String()
}

func (m *model) assistantName() string {
	return agent.AssistantName
}

func (m *model) View() string {
	body := m.renderBody()
	content := body
	m.viewport.SetContent(content)
	view := m.viewport.View()

	statusLine := ""
	if m.status != "" {
		statusLine = statusStyle.Render(m.status) + "\n"
	}
	inputView := ""
	if !m.running {
		inputView = m.input.View()
	} else {
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("GoDucky is working... (Ctrl+C to quit)")
	}
	return view + "\n" + statusLine + inputView
}

// runAgent starts the agent in a returned tea.Cmd (runs in its own goroutine).
func (m *model) runAgent() tea.Cmd {
	return func() tea.Msg {
		messages := m.pendingMessages
		m.agent.SetApprover(func(desc string, args map[string]any) bool {
			ch := make(chan bool, 1)
			if m.program != nil {
				m.program.Send(approvalRequestMsg{desc: approvalLabel(desc, args), args: args, respond: ch})
			}
			select {
			case ok := <-ch:
				return ok
			case <-time.After(10 * time.Minute):
				return false
			}
		})
		cb := &tuiCallback{m: m}
		result, _, err := m.agent.Run(context.Background(), messages, cb)
		if err != nil {
			return agentErrMsg{err: err}
		}
		return completeMsg{text: result}
	}
}

func approvalLabel(desc string, args map[string]any) string {
	var sb strings.Builder
	sb.WriteString(desc)
	if len(args) > 0 {
		sb.WriteString(" ")
		parts := make([]string, 0, len(args))
		for k, v := range args {
			vs := fmt.Sprintf("%v", v)
			if len(vs) > 60 {
				vs = vs[:60] + "..."
			}
			parts = append(parts, k+"="+vs)
		}
		sb.WriteString(strings.Join(parts, " "))
	}
	return sb.String()
}

// handleCommand processes slash-commands typed in the input. It renders
// feedback into the transcript and optionally returns a quit command.
func (m *model) handleCommand(cmd string) tea.Cmd {
	parts := strings.Fields(strings.TrimSpace(cmd))
	if len(parts) == 0 {
		return nil
	}
	command := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
	}

	switch command {
	case "/help", "/?":
		m.addItem("assistant", helpText())
	case "/exit", "/quit", "/q":
		return tea.Quit
	case "/model":
		if arg != "" {
			m.agent.SetModel(arg)
			m.modelName = arg
			m.cfg.Model = arg
			_ = m.cfg.Save()
			m.addItem("assistant", "Set model to "+arg+" on provider "+m.agent.ProviderName())
		} else {
			m.addItem("assistant", "Current: "+m.agent.ProviderName()+" / "+m.agent.ModelName()+
				"\nUse /model <name> to change, or /providers to see options.")
		}
	case "/providers", "/provider", "/use":
		if arg != "" {
			return m.switchProvider(arg)
		}
		m.addItem("assistant", providersHelp(m.cfg))
	case "/login":
		m.addItem("assistant",
			"To add a cloud API key, quit and run:\n  goducky --login openrouter\n  goducky --login groq\n  goducky --login openai\n  goducky --login anthropic\n  goducky --login gemini\nThen start goducky again. Groq has a free tier.")
	case "/setup":
		m.addItem("assistant", "Run `goducky` in a terminal after quitting to re-run setup, or pull a local model with `ollama pull qwen2.5-coder:7b`.")
	case "/clear":
		m.items = nil
		m.viewport.SetContent(m.renderBody())
	default:
		m.addItem("assistant", "Unknown command: "+command+"\nType /help for available commands.")
	}
	return nil
}

// switchProvider rebuilds the backend from config for the named provider.
func (m *model) switchProvider(name string) tea.Cmd {
	name = strings.ToLower(strings.TrimSpace(name))
	valid := map[string]bool{"ollama": true, "groq": true, "openai": true, "openai_compatible": true, "anthropic": true, "gemini": true, "openrouter": true}
	if !valid[name] {
		m.addItem("assistant", "Unknown provider: "+name+"\nValid: ollama, groq, openai, openai_compatible, anthropic, gemini, openrouter")
		return nil
	}
	oldProvider := m.agent.ProviderName()
	if name == oldProvider {
		m.addItem("assistant", "Already using "+name)
		return nil
	}
	m.cfg.Provider = name
	p, err := provider.New(m.cfg, m.auth)
	if err != nil {
		m.addItem("assistant", "Error switching provider: "+err.Error())
		return nil
	}
	m.agent.SetProvider(p)
	modelName := provider.ResolveModel(m.cfg, "")
	m.agent.SetModel(modelName)
	m.modelName = modelName
	m.provider = name
	_ = m.cfg.Save()
	m.addItem("assistant",
		"Switched to provider "+name+" (model: "+modelName+").\n"+
			"If no API key is set, run `goducky --login "+name+"`.")
	return nil
}

func providersHelp(cfg *config.Config) string {
	sb := &strings.Builder{}
	sb.WriteString("Providers:\n")
	fmt.Fprintf(sb, "  current   : %s / %s\n", cfg.Provider, cfg.Model)
	fmt.Fprintf(sb, "  ollama    : local, free (%s)\n", cfg.Ollama.Model)
	fmt.Fprintf(sb, "  groq      : cloud, free tier (%s)\n", cfg.Groq.Model)
	fmt.Fprintf(sb, "  openai    : cloud (%s)\n", cfg.OpenAI.Model)
	fmt.Fprintf(sb, "  openrouter: cloud, many models (%s)\n", cfg.OpenRouter.Model)
	fmt.Fprintf(sb, "  anthropic : cloud (%s)\n", cfg.Anthropic.Model)
	fmt.Fprintf(sb, "  gemini    : cloud (%s)\n", cfg.Gemini.Model)
	sb.WriteString("Route: /provider <name>  ·  Model: /model <name>")
	return sb.String()
}

func helpText() string {
	return `Commands:
  /help            Show this help
  /provider <name> Switch provider (ollama, groq, openai, openai_compatible, anthropic, gemini, openrouter)
  /model <name>    Set the model for the current provider
  /providers       List providers and switch
  /login           How to add a cloud API key
  /clear           Clear the conversation
  /exit            Quit GoDucky

Controls:
  Enter            Send / run command
  Ctrl+C           Quit
  PageUp/PageDown  Scroll
  ↑/↓              Input history
`

}

func (m *model) toProviderMessages() []provider.Message {
	var out []provider.Message
	for _, it := range m.items {
		if it.meta {
			continue
		}
		switch it.kind {
		case "user":
			out = append(out, provider.NewTextMessage(provider.RoleUser, it.text))
		case "assistant":
			out = append(out, provider.NewTextMessage(provider.RoleAssistant, it.text))
		}
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}
