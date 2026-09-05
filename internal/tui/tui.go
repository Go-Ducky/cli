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

type modelsMsg struct {
	provider string
	models   []string
	err      error
}

// pickerState renders a small inline menu at the bottom of the transcript
// (used by /models, /provider and /config provider|model).
type pickerState struct {
	title    string
	options  []string
	selected int
	onPick   func(m *model, option string) tea.Cmd
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
	picker          *pickerState
}

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	alertStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	headerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Background(lipgloss.Color("236")).Padding(0, 1)
	chipStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Background(lipgloss.Color("237")).Padding(0, 1)
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
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
	m.addMetaItem("assistant", m.assistantName()+" ready. Type a message or /help. Ctrl+C or Ctrl+X to quit.")
	m.addMetaItem("assistant", fmt.Sprintf("Provider: %s  Model: %s  Dir: %s", m.provider, m.modelName, m.workDir))
	m.input.Focus()
	return tea.Batch(tea.EnterAltScreen, textarea.Blink)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.picker != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "up", "w", "a", "k":
				if m.picker.selected > 0 {
					m.picker.selected--
				}
			case "down", "s", "d", "j":
				if m.picker.selected < len(m.picker.options)-1 {
					m.picker.selected++
				}
			case "enter", " ":
				opt := m.picker.options[m.picker.selected]
				cb := m.picker.onPick
				m.closePicker()
				if cb != nil {
					if c := cb(m, opt); c != nil {
						cmds = append(cmds, c)
					}
				}
			case "esc", "ctrl+c", "ctrl+x", "q":
				m.closePicker()
			}
		}
		m.viewport.GotoBottom()
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 5
		m.viewport.SetContent(m.renderBody())
		if m.viewport.Height > 0 {
			m.input.SetWidth(m.width - 2)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+x", "ctrl+d":
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

	case modelsMsg:
		fixed := make([]string, 0, len(msg.models)+2)
		fixed = append(fixed, msg.models...)
		fixed = append(fixed, "── Cancel ──")
		title := "Models for " + msg.provider
		if msg.err != nil {
			title += " (live list failed, showing known models)"
		}
		m.picker = &pickerState{
			title:   title,
			options: fixed,
			onPick:  pickModel,
		}
		m.viewport.GotoBottom()
		return m, nil

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
	if m.picker != nil {
		sb.WriteString("\n\n" + highlightStyle.Render(m.picker.title) + "\n")
		for i, opt := range m.picker.options {
			if i == m.picker.selected {
				sb.WriteString(highlightStyle.Render("❯ "+opt) + "\n")
			} else {
				sb.WriteString("  " + opt + "\n")
			}
		}
		sb.WriteString(dimStyle.Render("↑/↓ or W/S to move · Enter to pick · Esc to cancel"))
	}
	return sb.String()
}

func (m *model) assistantName() string {
	return agent.AssistantName
}

func (m *model) closePicker() { m.picker = nil }

// openModelsPicker queries the current provider for available models (off the
// event loop) and opens a picker with the results when ready.
func (m *model) openModelsPicker() tea.Cmd {
	prov := m.agent.ProviderName()
	cfg := m.cfg
	auth := m.auth
	return func() tea.Msg {
		models, err := fetchModelsFor(prov, cfg, auth)
		return modelsMsg{provider: prov, models: models, err: err}
	}
}

// fetchModelsFor lists models for a provider. Ollama, Groq, OpenAI and
// OpenRouter are queried live; the rest use a small curated list.
func fetchModelsFor(prov string, cfg *config.Config, auth *config.Auth) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch prov {
	case "ollama":
		return provider.NewOllama(cfg).ListModels(ctx)
	case "openrouter":
		if free, err := provider.OpenRouterFreeModels(ctx, ""); err == nil && len(free) > 0 {
			return free, nil
		}
		return curatedModels(prov), nil
	case "groq":
		if models, err := provider.NewGroq(cfg, auth, true).ListModels(ctx); err == nil && len(models) > 0 {
			return models, nil
		}
		return curatedModels(prov), nil
	case "openai", "openai_compatible":
		if models, err := provider.NewOpenAI(cfg, auth, prov == "openai_compatible").ListModels(ctx); err == nil && len(models) > 0 {
			return models, nil
		}
		return curatedModels(prov), nil
	}
	return curatedModels(prov), nil
}

// curatedModels is a fallback / default list shown when a live query fails or
// the provider doesn't expose one.
func curatedModels(prov string) []string {
	switch prov {
	case "ollama":
		return []string{"qwen2.5-coder:7b", "qwen2.5-coder:3b", "llama3.2:3b", "llama3.2:1b"}
	case "groq":
		return []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "llama3-8b-8192"}
	case "openai":
		return []string{"gpt-4o-mini", "gpt-4o", "gpt-5-mini"}
	case "openai_compatible":
		return []string{"qwen2.5-coder:7b", "gpt-4o-mini"}
	case "anthropic":
		return []string{"claude-3-5-haiku-latest", "claude-3-5-sonnet-latest"}
	case "gemini":
		return []string{"gemini-2.0-flash", "gemini-1.5-flash", "gemini-1.5-pro"}
	case "openrouter":
		return []string{"openrouter/free", "aimlapi/qwen2.5-coder-3b", "qwen/qwen-2.5-coder-7b-instruct"}
	}
	return []string{"openrouter/free"}
}

// pickModel applies a selection from the /models or /config model picker.
func pickModel(m *model, opt string) tea.Cmd {
	if opt == "" || strings.HasPrefix(opt, "──") {
		return nil
	}
	model := strings.TrimSpace(opt)
	prov := m.agent.ProviderName()
	m.applyModel(model)
	if err := m.cfg.Save(); err != nil {
		m.addItem("assistant", "Model set, but saving config failed: "+err.Error())
	}
	m.addItem("assistant", "Using model "+model)

	// Offer to pull it if it's a local model we don't have yet.
	if prov == "ollama" {
		if local, err := provider.NewOllama(m.cfg).ListModels(m.agentCtx()); err == nil && !containsString(local, model) {
			m.addItem("assistant", "Model "+model+" isn't pulled yet. Use `ollama pull "+model+"` once.")
		}
	}
	return nil
}

func (m *model) agentCtx() context.Context { return context.Background() }

func containsString(list []string, s string) bool {
	for _, it := range list {
		if it == s {
			return true
		}
	}
	return false
}

// providerPicker opens the inline menu for choosing a provider.
func (m *model) providerPicker() {
	m.picker = &pickerState{
		title:   "Choose a provider",
		options: []string{"ollama", "groq", "openai", "openai_compatible", "anthropic", "gemini", "openrouter", "── Cancel ──"},
		onPick: func(m *model, opt string) tea.Cmd {
			if strings.HasPrefix(opt, "──") {
				return nil
			}
			return m.switchProvider(opt)
		},
	}
	m.viewport.GotoBottom()
}

func (m *model) View() string {
	body := m.renderBody()
	content := body
	m.viewport.SetContent(content)
	view := m.viewport.View()

	header := ""
	if m.width > 0 {
		header = headerStyle.Render(" GoDucky ") + " " + chipStyle.Render(m.provider) + " " + chipStyle.Render(m.modelName) + "\n"
	}
	statusLine := ""
	if m.status != "" {
		statusLine = statusStyle.Render(m.status) + "\n"
	}
	inputView := ""
	if !m.running && m.picker == nil {
		inputView = m.input.View()
	} else if m.picker == nil {
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("GoDucky is working... (Ctrl+C to quit)")
	}
	return header + view + "\n" + statusLine + inputView
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
			if len([]rune(vs)) > 60 {
				vs = truncRunes(vs, 60)
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
	case "/config":
		return m.configCmd(arg)
	case "/model":
		if arg != "" {
			m.applyModel(arg)
			_ = m.cfg.Save()
			m.addItem("assistant", "Set model to "+arg+" on provider "+m.agent.ProviderName())
		} else {
			return m.openModelsPicker()
		}
	case "/models":
		return m.openModelsPicker()
	case "/providers", "/provider", "/use":
		if arg != "" {
			return m.switchProvider(arg)
		}
		m.providerPicker()
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

// configCmd shows or edits config keys in-app via /config.
func (m *model) configCmd(arg string) tea.Cmd {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		m.addItem("assistant", configHelp(m.cfg))
		return nil
	}
	key, val, _ := strings.Cut(arg, " ")
	key = strings.ToLower(strings.TrimSpace(key))
	val = strings.TrimSpace(val)
	if val == "" {
		switch key {
		case "provider":
			m.providerPicker()
			return nil
		case "model":
			return m.openModelsPicker()
		}
		m.addItem("assistant", configHelp(m.cfg))
		return nil
	}
	if err := m.cfg.Set(key, val); err != nil {
		m.addItem("assistant", "Error: "+err.Error())
		return nil
	}
	switch key {
	case "provider":
		return m.switchProvider(val)
	case "model":
		m.applyModel(val)
	case "host", "ollama.host":
		if m.agent.ProviderName() == "ollama" {
			if p, err := provider.New(m.cfg, m.auth); err == nil {
				m.agent.SetProvider(p)
			}
		}
	}
	if err := m.cfg.Save(); err != nil {
		m.addItem("assistant", "Error saving config: "+err.Error())
		return nil
	}
	m.addItem("assistant", "Saved config: "+key+" = "+val)
	return nil
}

// applyModel sets the model everywhere: the active agent and the active
// provider's config block, so it survives a restart.
func (m *model) applyModel(model string) {
	m.agent.SetModel(model)
	m.modelName = model
	m.cfg.Model = model
	switch m.agent.ProviderName() {
	case "ollama":
		m.cfg.Ollama.Model = model
	case "groq":
		m.cfg.Groq.Model = model
	case "openai", "openai_compatible":
		m.cfg.OpenAI.Model = model
	case "openrouter":
		m.cfg.OpenRouter.Model = model
	case "anthropic":
		m.cfg.Anthropic.Model = model
	case "gemini":
		m.cfg.Gemini.Model = model
	}
}

func configHelp(cfg *config.Config) string {
	sb := &strings.Builder{}
	sb.WriteString("Current settings (saved to config.json):\n")
	fmt.Fprintf(sb, "  provider            : %s\n", cfg.Provider)
	fmt.Fprintf(sb, "  model               : %s\n", cfg.Model)
	fmt.Fprintf(sb, "  local Ollama        : %s (%s)\n", cfg.Ollama.Host, cfg.Ollama.Model)
	fmt.Fprintf(sb, "  groq                : %s\n", cfg.Groq.Model)
	fmt.Fprintf(sb, "  openai              : %s (%s)\n", cfg.OpenAI.Model, cfg.OpenAI.BaseURL)
	fmt.Fprintf(sb, "  openrouter          : %s\n", cfg.OpenRouter.Model)
	fmt.Fprintf(sb, "  anthropic           : %s\n", cfg.Anthropic.Model)
	fmt.Fprintf(sb, "  gemini              : %s\n", cfg.Gemini.Model)
	fmt.Fprintf(sb, "  auto-approve tools  : %s\n", onOff(cfg.Agent.AutoApprove))
	fmt.Fprintf(sb, "  max iterations      : %d\n", cfg.Agent.MaxIterations)
	fmt.Fprintf(sb, "  max output chars    : %d\n", cfg.Agent.MaxOutputChars)
	fmt.Fprintf(sb, "  exclude dirs        : %s\n", strings.Join(cfg.Agent.ExcludeDirs, ", "))
	sb.WriteString("\nEasy examples:\n")
	sb.WriteString("  /config model <name>          pick a model (or just /config model)\n")
	sb.WriteString("  /config provider              choose a provider interactively\n")
	sb.WriteString("  /config host http://localhost:11434\n")
	sb.WriteString("  /config auto-approve on       skip permission prompts\n")
	sb.WriteString("  /config iterations 50\n")
	sb.WriteString("  /config exclude .git,dist\n")
	sb.WriteString("  /config openrouter.model <id>  any dotted key works too\n")
	return sb.String()
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func helpText() string {
	return `Commands:
  /help            Show this help
  /models          Pick a model for the current provider
  /config          View or edit configuration (aliases: host, auto-approve, iterations, output)
  /model <name>    Set the model for the current provider
  /provider        Choose a provider interactively (or: /provider <name>)
  /login           How to add a cloud API key
  /clear           Clear the conversation
  /exit            Quit GoDucky

Controls:
  Enter            Send / run command
  Ctrl+C / Ctrl+X  Quit
  PageUp/PageDown  Scroll
  Mouse wheel      Scroll
  Arrow/WASD       Navigate menus (Enter picks, Esc cancels)
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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n...[truncated]"
}

// truncRunes cuts a string to n runes without splitting a multi-byte character,
// which matters for Czech, Polish, Spanish and other non-ASCII text.
func truncRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
