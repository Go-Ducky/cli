package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Go-Ducky/cli/internal/agent"
	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/provider"
	"github.com/Go-Ducky/cli/internal/session"
	"github.com/Go-Ducky/cli/internal/setup"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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

// ollamaOpMsg reports the result of an async `ollama pull`/`ollama rm`.
type ollamaOpMsg struct {
	action string // "pull" | "rm"
	model  string
	err    error
}

// openErrMsg reports a failed attempt to open the system browser.
type openErrMsg struct{ err error }

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
	configPrompt    string // when set, the next Enter sets this config key
	promptKeyFor    string // when set, the input collects & saves an API key for this provider
	sessionName     string // set when the chat was resumed or /save was used
	history         []string
	histCursor      int

	selecting  bool             // a left-drag is in progress
	selStart   *selPos          // anchor (doc row, cell), nil when not selecting
	selEnd     *selPos          // current drag endpoint
	plainLines []string         // plain-text body lines, parallel to viewport lines

	runCtx    context.Context
	cancelRun context.CancelFunc // cancels the in-flight agent turn (Esc)
	cancelled bool               // the last turn was stopped by the user

	lastArrow  time.Time // last arrow-key press, for wheel-vs-history detection
	arrowBurst int       // how many arrow presses arrived in quick succession
}

// selPos is a position in the chat body: content row + cell column.
type selPos struct {
	row int
	col int
}

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	alertStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	selStyle       = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("231"))
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

// SetHistory preloads a previously saved chat into the transcript before the
// TUI starts (used by `goducky resume`). The prior messages naturally become
// the model's context on the next prompt.
func (m *model) SetHistory(name string, msgs []provider.Message) {
	if name != "" {
		m.sessionName = name
		m.addMetaItem("assistant", "Resumed chat "+name+".")
	}
	for _, msg := range msgs {
		for _, blk := range msg.Content {
			if blk.Type == "text" && blk.Text != "" && (msg.Role == provider.RoleUser || msg.Role == provider.RoleAssistant) {
				m.addItem(string(msg.Role), blk.Text)
			}
		}
	}
}

// Session returns the current chat in a form that can be saved to disk.
func (m *model) Session() *session.Session {
	return &session.Session{
		Name:      m.sessionName,
		Provider:  m.agent.ProviderName(),
		Model:     m.modelName,
		WorkDir:   m.workDir,
		Messages:  m.toProviderMessages(),
	}
}

func (m *model) Init() tea.Cmd {
	m.addMetaItem("assistant", "Type a message or /help — Ctrl+C copies the last reply, Ctrl+X quits.")
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
		if m.promptKeyFor != "" {
			// Collecting an API key: Enter saves it, Esc cancels, other keys
			// keep editing (rendered masked).
			switch msg.String() {
			case "enter":
				return m.submitKeyPrompt()
			case "esc":
				m.promptKeyFor = ""
				m.input.Reset()
				m.input.Placeholder = "Ask GoDucky... (Enter to send)"
				m.addItem("assistant", "Cancelled — still on "+m.agent.ProviderName()+".")
				m.viewport.SetContent(m.renderBody())
				m.viewport.GotoBottom()
				return m, nil
			}
			var c tea.Cmd
			m.input, c = m.input.Update(msg)
			return m, c
		}
		switch msg.String() {
		case "ctrl+c":
			return m.copyToClipCmd()
		case "ctrl+b":
			return m.copyAllCmd()
		case "ctrl+x", "ctrl+d":
			return m, tea.Quit
		case "enter":
			if m.approvalPending && m.approvalChan != nil {
				return m.approvalAnswer(true)
			}
		case "esc":
			if m.approvalPending && m.approvalChan != nil {
				return m.approvalAnswer(false)
			}
			if m.running {
				return m.stopTurn()
			}
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
		case "up", "down":
			// Arrow up/down recall previous prompts like a shell history,
			// unless they come in a rapid burst — terminals that can't report
			// the mouse translate the wheel into fast arrow keys, and those
			// should scroll the chat instead of recalling your last prompt.
			if !m.running && !m.approvalPending {
				now := time.Now()
				if now.Sub(m.lastArrow) <= 200*time.Millisecond {
					m.arrowBurst++
				} else {
					m.arrowBurst = 1
				}
				m.lastArrow = now
				if m.arrowBurst >= 2 {
					if msg.String() == "up" {
						m.viewport.LineUp(viewportMouseWheelDelta)
					} else {
						m.viewport.LineDown(viewportMouseWheelDelta)
					}
					return m, nil
				}
				if msg.String() == "up" {
					m.recallHistory()
				} else {
					m.forwardHistory()
				}
				return m, nil
			}
		}
		if m.running {
			return m, nil
		}

	case tea.MouseMsg:
		if m.viewport.Height > 0 {
			// The wheel always scrolls the chat (like a messaging app). It is
			// handled right here so it never falls through to prompt-recall.
			if msg.Button == tea.MouseButtonWheelUp && msg.Action == tea.MouseActionPress {
				m.viewport.LineUp(viewportMouseWheelDelta)
				return m, nil
			}
			if msg.Button == tea.MouseButtonWheelDown && msg.Action == tea.MouseActionPress {
				m.viewport.LineDown(viewportMouseWheelDelta)
				return m, nil
			}
			// Left-drag selects (or starts a new selection).
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				if row, col, ok := m.mouseToDoc(msg); ok {
					m.selecting = true
					p := selPos{row: row, col: col}
					m.selStart, m.selEnd = &p, &p
				} else {
					m.selecting = false
					m.selStart, m.selEnd = nil, nil
				}
				return m, nil
			}
			if m.selecting && msg.Action == tea.MouseActionMotion {
				if row, col, ok := m.mouseToDoc(msg); ok {
					m.selEnd = &selPos{row: row, col: col}
					return m, nil
				}
			}
			if m.selecting && msg.Action == tea.MouseActionRelease {
				m.selecting = false
				return m.copySelectionCmd()
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
		m.cancelled = false
		m.status = "Thinking..."
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		msgs := m.toProviderMessages()
		msgs = append(msgs, provider.NewTextMessage(provider.RoleUser, msg.text))
		m.pendingMessages = msgs
		var ctx context.Context
		ctx, m.cancelRun = context.WithCancel(context.Background())
		cmds = append(cmds, m.runAgent(ctx))

	case completeMsg:
		if m.cancelled {
			return m.finishStopped(), textarea.Blink
		}
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
		if m.cancelled {
			return m.finishStopped(), textarea.Blink
		}
		m.addItem("assistant", "❌ "+msg.err.Error())
		m.running = false
		m.status = ""
		m.input.Focus()
		m.viewport.SetContent(m.renderBody())
		return m, textarea.Blink

	case apiKeyMsg:
		m.status = ""
		m.promptKeyFor = ""
		m.input.Reset()
		m.input.Placeholder = "Ask GoDucky... (Enter to send)"
		if msg.err != nil {
			m.addItem("assistant", "❌ Key not saved: "+msg.err.Error())
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
			return m, nil
		}
		setAuthKey(m.auth, msg.provider, msg.key)
		if msg.verified {
			m.addItem("assistant", "API key for "+msg.provider+" saved and verified.")
		} else {
			m.addItem("assistant", "API key for "+msg.provider+" saved (couldn't verify — offline?).")
		}
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, m.switchProvider(msg.provider)

	case openErrMsg:
		m.addItem("assistant", "Could not open your browser: "+msg.err.Error()+"\nThe repo is at https://github.com/Go-Ducky/cli")
		return m, nil

	case ollamaOpMsg:
		m.status = ""
		switch msg.action {
		case "rm":
			if msg.err != nil {
				m.addItem("assistant", "❌ Could not remove "+msg.model+": "+msg.err.Error())
			} else {
				m.addItem("assistant", "Removed model "+msg.model+".")
			}
		default:
			if msg.err != nil {
				m.addItem("assistant", "❌ Could not pull "+msg.model+":\n"+msg.err.Error())
			} else {
				m.addItem("assistant", "Model "+msg.model+" pulled and ready to use.")
			}
		}
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, nil

	case copiedMsg:
		m.status = "Copied to " + msg.method + "."
		m.viewport.SetContent(m.renderBody())
		return m, nil

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
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" && m.configPrompt != "" {
			m.configPrompt = ""
			m.status = ""
			m.input.SetValue("")
			m.addItem("assistant", "Cancelled.")
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				if m.configPrompt != "" {
					key := m.configPrompt
					m.configPrompt = ""
					m.status = ""
					m.input.Reset()
					cmds = append(cmds, m.configCmd(key+" "+text))
				} else {
					m.pushHistory(text)
					m.input.Reset()
					cmds = append(cmds, func() tea.Msg { return userMsg{text: text} })
				}
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

// pushHistory records a sent prompt and resets the recall cursor to "new input".
func (m *model) pushHistory(text string) {
	if len(m.history) > 0 && m.history[len(m.history)-1] == text {
		m.histCursor = len(m.history)
		return
	}
	m.history = append(m.history, text)
	m.histCursor = len(m.history)
}

// recallHistory walks back through sent prompts (arrow up).
func (m *model) recallHistory() {
	if len(m.history) == 0 || m.histCursor <= 0 {
		return
	}
	m.histCursor--
	m.input.SetValue(m.history[m.histCursor])
	m.input.CursorEnd()
}

// forwardHistory walks forward again toward a fresh input (arrow down).
func (m *model) forwardHistory() {
	if m.histCursor >= len(m.history) {
		return
	}
	m.histCursor++
	if m.histCursor == len(m.history) {
		m.input.Reset()
		return
	}
	m.input.SetValue(m.history[m.histCursor])
	m.input.CursorEnd()
}

func (m *model) renderBody() string {
	w := m.width - 2
	if w < 20 {
		w = 80
	}
	m.plainLines = m.plainBodyLines(w)
	if m.selStart != nil && m.selEnd != nil {
		return m.highlightedBody()
	}
	var sb strings.Builder
	for _, it := range m.items {
		switch it.kind {
		case "user":
			sb.WriteString("\n" + userStyle.Render("You:") + "\n" + wrapText(it.text, w) + "\n")
		case "assistant":
			if it.text != "" {
				if it.meta {
					sb.WriteString("\n" + wrapText(it.text, w) + "\n")
				} else {
					sb.WriteString("\n" + assistantStyle.Render(m.assistantName()+":") + "\n" + wrapText(it.text, w) + "\n")
				}
			}
		case "tool":
			sb.WriteString("\n" + toolStyle.Render(wrapText(it.text, w)) + "\n")
		}
	}
	if m.current != "" {
		sb.WriteString("\n" + assistantStyle.Render(m.assistantName()+":") + "\n" + wrapText(m.current, w))
	}
	if m.picker != nil {
		sb.WriteString("\n\n" + highlightStyle.Render(wrapText(m.picker.title, w)) + "\n")
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

// plainBodyLines builds the same lines as renderBody but without styling, one
// entry per viewport line. It is used both to highlight and to extract the
// selected text, so it must wrap exactly like renderBody does.
func (m *model) plainBodyLines(w int) []string {
	var lines []string
	add := func(s string) {
		for _, ln := range strings.Split(s, "\n") {
			lines = append(lines, ln)
		}
	}
	for _, it := range m.items {
		switch it.kind {
		case "user":
			add("\nYou:\n" + wrapText(it.text, w) + "\n")
		case "assistant":
			if it.text != "" {
				if it.meta {
					add("\n" + wrapText(it.text, w) + "\n")
				} else {
					add("\n" + m.assistantName() + ":\n" + wrapText(it.text, w) + "\n")
				}
			}
		case "tool":
			add("\n" + wrapText(it.text, w) + "\n")
		}
	}
	if m.current != "" {
		add("\n" + m.assistantName() + ":\n" + wrapText(m.current, w))
	}
	if m.picker != nil {
		add("\n\n" + wrapText(m.picker.title, w))
		for i, opt := range m.picker.options {
			if i == m.picker.selected {
				add("\n❯ " + opt)
			} else {
				add("\n  " + opt)
			}
		}
	}
	return lines
}

// highlightedBody renders the plain body with the current selection painted,
// matching the terminal's own look: a highlighted block from the anchor cell to
// the endpoint cell.
func (m *model) highlightedBody() string {
	a, b := m.normSel()
	if b.row >= len(m.plainLines) {
		b.row = len(m.plainLines) - 1
	}
	if b.row < 0 {
		return m.bodyPlain()
	}
	var sb strings.Builder
	for i, line := range m.plainLines {
		if i < a.row || i > b.row {
			sb.WriteString(line)
			sb.WriteString("\n")
			continue
		}
		switch {
		case a.row == b.row:
			if a.col > b.col {
				a, b = b, a
			}
			sb.WriteString(selSlice(line, a.col, b.col, selStyle))
		case i == a.row:
			sb.WriteString(selSlice(line, a.col, displayWidth(line), selStyle))
		case i == b.row:
			sb.WriteString(selSlice(line, 0, b.col, selStyle))
		default:
			sb.WriteString(selStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	s := sb.String()
	return strings.TrimSuffix(s, "\n")
}

// normSel returns the selection endpoints ordered from top-left to bottom-right.
func (m *model) normSel() (selPos, selPos) {
	a := selPos{}
	b := selPos{}
	if m.selStart != nil {
		a, b = *m.selStart, *m.selEnd
		if a.row > b.row || (a.row == b.row && a.col > b.col) {
			a, b = b, a
		}
	}
	if a.row < 0 {
		a.row = 0
	}
	if a.col < 0 {
		a.col = 0
	}
	if b.col < 0 {
		b.col = 0
	}
	return a, b
}

// bodyPlain returns the unhighlighted plain body (used when there is no valid
// selection to paint).
func (m *model) bodyPlain() string {
	return strings.Join(m.plainLines, "\n")
}

// selSlice renders a substring of a plain line (covering display cells from..to)
// with the selection style, leaving the rest unstyled.
func selSlice(line string, from, to int, st lipgloss.Style) string {
	w := displayWidth(line)
	if from < 0 {
		from = 0
	}
	if to > w {
		to = w
	}
	if from > w {
		from = w
	}
	if from >= to {
		return line
	}
	pre := subCells(line, 0, from)
	sel := subCells(line, from, to)
	post := subCells(line, to, w)
	return pre + st.Render(sel) + post
}

// displayWidth returns the terminal display width of s.
func displayWidth(s string) int { return runewidth.StringWidth(s) }

// subCells returns the substring of s covering display cells [from, to).
func subCells(s string, from, to int) string {
	if from >= to {
		return ""
	}
	var w int
	start := -1
	end := -1
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		cellBefore, cellAfter := w, w+runewidth.RuneWidth(r)
		if start < 0 && cellAfter > from {
			start = i
		}
		if end < 0 && cellBefore >= to {
			end = i
			break
		}
		w = cellAfter
		i += sz
	}
	if start < 0 {
		return ""
	}
	if end < 0 {
		end = len(s)
	}
	return s[start:end]
}

// wrapText wraps plain text to width cells using terminal display width, so it
// scrolls down instead of overflowing. Word-aware, with a rune-level fallback
// that keeps Czech, Polish, Spanish, CJK etc. intact.
func wrapText(s string, width int) string {
	if width < 8 {
		width = 8
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, wrapLine(line, width))
	}
	return strings.Join(out, "\n")
}

func wrapLine(line string, width int) string {
	if runewidth.StringWidth(line) <= width {
		return line
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return strings.Join(wrapRunes(line, width), "\n")
	}
	var out []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		if curW > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
	}
	for _, w := range words {
		wW := runewidth.StringWidth(w)
		if wW > width {
			flush()
			out = append(out, wrapRunes(w, width)...)
			continue
		}
		if curW == 0 {
			cur.WriteString(w)
			curW = wW
			continue
		}
		if curW+1+wW <= width {
			cur.WriteString(" ")
			cur.WriteString(w)
			curW += 1 + wW
			continue
		}
		flush()
		cur.WriteString(w)
		curW = wW
	}
	flush()
	if len(out) == 0 {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// wrapRunes breaks a token into width-sized pieces without splitting a
// multi-byte character (measured by display width).
func wrapRunes(s string, width int) []string {
	var out []string
	var cur strings.Builder
	curW := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw < 1 {
			rw = 1
		}
		if curW+rw > width {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	if curW > 0 {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
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
		// Show what's installed (marked) plus the whole recommended shortlist,
		// so picking is never stuck with just one model.
		installed, err := provider.NewOllama(cfg).ListModels(ctx)
		seen := map[string]bool{}
		out := make([]string, 0, len(installed)+len(setup.RecommendedModelIDs()))
		for _, m := range installed {
			seen[m] = true
			out = append(out, m+"  (pulled)")
		}
		for _, m := range setup.RecommendedModelIDs() {
			if !seen[m] {
				out = append(out, m)
			}
		}
		return out, err
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
		return setup.RecommendedModelIDs()
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
	model := strings.TrimSuffix(strings.TrimSpace(opt), "  (pulled)")
	prov := m.agent.ProviderName()
	m.applyModel(model)
	if err := m.cfg.Save(); err != nil {
		m.addItem("assistant", "Model set, but saving config failed: "+err.Error())
	}
	m.addItem("assistant", "Using model "+model)

	// If it's a local model we don't have yet, pull it automatically.
	if prov == "ollama" {
		return m.ensureOllamaModel(model)
	}
	return nil
}

// ensureOllamaModel pulls a model through the Ollama API when it isn't local
// yet (and errors clearly if the name doesn't exist in the Ollama library).
func (m *model) ensureOllamaModel(model string) tea.Cmd {
	if m.agent.ProviderName() != "ollama" {
		return nil
	}
	o := provider.NewOllama(m.cfg)
	if o.HasModel(m.agentCtx(), model) {
		return nil
	}
	m.addItem("assistant", "Model "+model+" isn't local — pulling it through Ollama. Grab a coffee, this can take a while.")
	return func() tea.Msg {
		err := o.Pull(context.Background(), model)
		return ollamaOpMsg{action: "pull", model: model, err: err}
	}
}

// agentCtx returns the default background context for helper calls.
func (m *model) agentCtx() context.Context { return context.Background() }

// runOpenBrowser opens a URL in the system browser, cross-platform.
func (m *model) runOpenBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		return openErrMsg{err: cmd.Start()}
	}
}

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

// copiedMsg reports after a Ctrl+C text copy.
type copiedMsg struct{ method string }

// copyText returns what Ctrl+C copies: the last assistant reply, or the whole
// transcript when there is no reply yet.
func (m *model) copyText() string {
	for i := len(m.items) - 1; i >= 0; i-- {
		if m.items[i].kind == "assistant" {
			return m.items[i].text
		}
	}
	var sb strings.Builder
	for _, it := range m.items {
		if it.meta {
			continue
		}
		sb.WriteString(it.text)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// copyAllText returns the full conversation, prompts and replies, for Ctrl+B.
func (m *model) copyAllText() string {
	var sb strings.Builder
	for _, it := range m.items {
		if it.meta {
			continue
		}
		label := "assistant"
		if it.kind == "user" {
			label = "user"
		}
		sb.WriteString(label + ": ")
		sb.WriteString(it.text)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// copyToClipCmd is the Ctrl+C handler: it copies the last reply (or the whole
// transcript) to the clipboard instead of quitting.
func (m *model) copyToClipCmd() (tea.Model, tea.Cmd) {
	text := m.copyText()
	if text == "" {
		m.status = "Nothing to copy yet."
		m.viewport.SetContent(m.renderBody())
		return m, nil
	}
	m.status = "Copying…"
	m.viewport.SetContent(m.renderBody())
	return m, func() tea.Msg {
		return copiedMsg{method: copyToClipboard(text)}
	}
}

// copyAllCmd is the Ctrl+B handler: it copies the entire conversation.
func (m *model) copyAllCmd() (tea.Model, tea.Cmd) {
	text := m.copyAllText()
	if text == "" {
		m.status = "Nothing to copy yet."
		m.viewport.SetContent(m.renderBody())
		return m, nil
	}
	m.status = "Copying whole conversation…"
	m.viewport.SetContent(m.renderBody())
	return m, func() tea.Msg {
		return copiedMsg{method: copyToClipboard(text)}
	}
}

// mouseToDoc maps a mouse event to a body position (content row + cell column),
// accounting for the one-line header above the viewport.
func (m *model) mouseToDoc(msg tea.MouseMsg) (row, col int, ok bool) {
	header := 0
	if m.width > 0 {
		header = 1
	}
	localY := msg.Y - header
	if localY < 0 || localY >= m.viewport.Height {
		return 0, 0, false
	}
	row = m.viewport.YOffset + localY
	if row < 0 {
		row = 0
	}
	if maxRow := len(m.plainLines) - 1; row > maxRow {
		row = maxRow
	}
	if row < 0 {
		return 0, 0, false
	}
	if msg.X < 0 {
		msg.X = 0
	}
	return row, msg.X, true
}

// copySelectionCmd copies the mouse-selected text on release.
func (m *model) copySelectionCmd() (tea.Model, tea.Cmd) {
	text := m.selectedText()
	m.selStart, m.selEnd = nil, nil
	m.viewport.SetContent(m.renderBody())
	if text == "" {
		m.status = "Nothing selected — drag over text to copy it."
		return m, nil
	}
	m.status = "Copied selected text."
	return m, func() tea.Msg {
		return copiedMsg{method: copyToClipboard(text)}
	}
}

// selectedText returns the text covered by the current selection, using the
// plain body lines that mirror the rendered output.
func (m *model) selectedText() string {
	if m.selStart == nil || m.selEnd == nil {
		return ""
	}
	a, b := m.normSel()
	if a.row < 0 || a.row >= len(m.plainLines) || b.row < 0 || b.row >= len(m.plainLines) {
		return ""
	}
	var sb strings.Builder
	for i := a.row; i <= b.row; i++ {
		line := m.plainLines[i]
		if i > a.row {
			sb.WriteString("\n")
		}
		switch {
		case i == a.row && i == b.row:
			sb.WriteString(subCells(line, a.col, b.col))
		case i == a.row:
			sb.WriteString(subCells(line, a.col, displayWidth(line)))
		case i == b.row:
			sb.WriteString(subCells(line, 0, b.col))
		default:
			sb.WriteString(line)
		}
	}
	return sb.String()
}

// copyToClipboard copies text using a native helper when available, otherwise
// the OSC 52 terminal clipboard (works in Windows Terminal, iTerm2, kitty…).
func copyToClipboard(text string) string {
	var candidates [][]string
	switch runtime.GOOS {
	case "windows":
		candidates = [][]string{{"clip"}}
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	default:
		candidates = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return "system clipboard"
		}
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", b64)
	return "terminal clipboard"
}

func (m *model) View() string {
	body := m.renderBody()
	content := body
	m.viewport.SetContent(content)
	view := m.viewport.View()

	header := ""
	if m.width > 0 {
		chat := m.sessionName
		if chat == "" {
			chat = "new chat"
		}
		line := "📁 " + m.workDir + "   ·   💬 " + chat
		if runewidth.StringWidth(line) > m.width {
			line = truncateWidth(line, m.width-1)
		}
		header = dimStyle.Render(line) + "\n"
	}
	statusLine := ""
	if m.status != "" {
		statusLine = statusStyle.Render(m.status) + "\n"
	}
	inputView := ""
	if m.promptKeyFor != "" {
		// Mask the API key while it is being pasted in.
		n := len([]rune(m.input.Value()))
		if max := m.width - 20; n > max {
			n = max
		}
		masked := strings.Repeat("•", n)
		if n == 0 {
			masked = "paste API key…"
		}
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("🔑 " + masked)
	} else if !m.running && m.picker == nil {
		inputView = m.input.View()
	} else if m.picker == nil {
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("GoDucky is working... (Esc to stop · Ctrl+X to quit)")
	}
	return header + view + "\n" + statusLine + inputView
}

// runAgent starts the agent in a returned tea.Cmd (runs in its own goroutine)
// using the given context, so Esc can cancel the turn.
func (m *model) runAgent(ctx context.Context) tea.Cmd {
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
		result, _, err := m.agent.Run(ctx, messages, cb)
		if err != nil {
			return agentErrMsg{err: err}
		}
		return completeMsg{text: result}
	}
}

// approvalAnswer answers a pending tool-approval prompt.
func (m *model) approvalAnswer(ok bool) (tea.Model, tea.Cmd) {
	if m.approvalChan == nil {
		return m, nil
	}
	ch := m.approvalChan
	return m, func() tea.Msg {
		return approvalAnswerMsg{approved: ok, respond: ch}
	}
}

// stopTurn cancels the in-flight agent turn (Esc while running).
func (m *model) stopTurn() (tea.Model, tea.Cmd) {
	if m.cancelRun != nil {
		m.cancelRun()
		m.cancelRun = nil
	}
	m.cancelled = true
	m.running = false
	m.status = "Stopping… keeping what was said so far"
	m.input.Focus()
	m.viewport.SetContent(m.renderBody())
	return m, nil
}

// finishStopped commits the partial reply and reports that the turn was stopped.
func (m *model) finishStopped() tea.Model {
	if m.current != "" {
		m.addItem("assistant", m.current)
	}
	m.addItem("assistant", "✋ Stopped.")
	m.current = ""
	m.cancelled = false
	m.cancelRun = nil
	m.running = false
	m.status = ""
	m.input.Focus()
	m.viewport.SetContent(m.renderBody())
	m.viewport.GotoBottom()
	return m
}

// apiKeyMsg reports the outcome of saving an API key typed in the chat.
type apiKeyMsg struct {
	provider string
	key      string
	verified bool
	err      error
}

// submitKeyPrompt validates and saves the key typed into the input, then
// returns the flow to apiKeyMsg so the provider switch can finish on the loop.
func (m *model) submitKeyPrompt() (tea.Model, tea.Cmd) {
	key := strings.TrimSpace(m.input.Value())
	prov := m.promptKeyFor
	if key == "" {
		m.status = "Empty key — paste your API key and press Enter."
		return m, nil
	}
	m.status = "Checking key with " + prov + "…"
	m.viewport.SetContent(m.renderBody())
	return m, func() tea.Msg {
		verified := true
		if err := provider.ValidateAPIKey(prov, key); err != nil {
			if strings.Contains(err.Error(), "rejected") {
				return apiKeyMsg{provider: prov, key: key, err: err}
			}
			verified = false // offline/unreachable: save anyway
		}
		auth, err := config.LoadAuth()
		if err != nil {
			return apiKeyMsg{provider: prov, key: key, err: err}
		}
		setAuthKey(auth, prov, key)
		if err := auth.Save(); err != nil {
			return apiKeyMsg{provider: prov, key: key, err: err}
		}
		return apiKeyMsg{provider: prov, key: key, verified: verified}
	}
}

// setAuthKey stores a key on an Auth struct.
func setAuthKey(a *config.Auth, prov, key string) {
	switch prov {
	case "groq":
		a.GroqAPIKey = key
	case "openai", "openai_compatible":
		a.OpenAIAPIKey = key
	case "anthropic":
		a.AnthropicAPIKey = key
	case "gemini":
		a.GeminiAPIKey = key
	case "openrouter":
		a.OpenRouterAPIKey = key
	}
}

// hasProviderKey reports whether a key is already available (auth file or env).
func (m *model) hasProviderKey(prov string) bool {
	var akey, env string
	switch prov {
	case "groq":
		akey, env = m.auth.GroqAPIKey, m.cfg.Groq.EnvKey
	case "openai", "openai_compatible":
		akey, env = m.auth.OpenAIAPIKey, m.cfg.OpenAI.EnvKey
	case "anthropic":
		akey, env = m.auth.AnthropicAPIKey, m.cfg.Anthropic.EnvKey
	case "gemini":
		akey, env = m.auth.GeminiAPIKey, m.cfg.Gemini.EnvKey
	case "openrouter":
		akey, env = m.auth.OpenRouterAPIKey, m.cfg.OpenRouter.EnvKey
	}
	if akey != "" {
		return true
	}
	return env != "" && os.Getenv(env) != ""
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
			return m.ensureOllamaModel(arg)
		}
		return m.openModelsPicker()
	case "/models":
		return m.openModelsPicker()
	case "/providers", "/provider", "/use":
		if arg != "" {
			return m.switchProvider(arg)
		}
		m.providerPicker()
	case "/login":
		m.addItem("assistant",
			"To add a cloud API key, just type:\n  /provider openrouter\n  /provider groq\n  /provider openai\n  /provider anthropic\n  /provider gemini\nGoDucky prompts you to paste the key, verifies it live, saves it, and switches.\nYou can also save one without the chat: run `goducky --login openrouter`.")
	case "/save":
		name := strings.TrimSpace(arg)
		if name == "" {
			m.addItem("assistant", "Usage: /save <name>\nThe chat is also auto-saved when you quit and resumed with `goducky resume`.")
			return nil
		}
		s := m.Session()
		s.Name = name
		if err := session.Save(s); err != nil {
			m.addItem("assistant", "Save failed: "+err.Error())
			return nil
		}
		m.sessionName = name
		m.addItem("assistant", "Chat saved as \""+name+"\". Resume later with: goducky resume "+name)
	case "/rename":
		name := strings.TrimSpace(arg)
		if name == "" {
			m.addItem("assistant", "Usage: /rename <new-name>")
			return nil
		}
		old := m.sessionName
		if old == "" {
			m.addItem("assistant", "This chat isn't saved yet — use /save <name> first.")
			return nil
		}
		if err := session.Rename(old, name); err != nil {
			m.addItem("assistant", "Rename failed: "+err.Error())
			return nil
		}
		m.sessionName = name
		m.addItem("assistant", "Chat renamed to \""+name+"\".")
	case "/sessions":
		var sb strings.Builder
		sessions, err := session.List()
		if err != nil {
			m.addItem("assistant", "Could not list chats: "+err.Error())
			return nil
		}
		sb.WriteString("Saved chats (resume from a new terminal with `goducky resume <n>`):\n")
		for i, s := range sessions {
			fmt.Fprintf(&sb, "  %2d. %s (%s / %s)\n", i+1, s.Name, s.Provider, s.Model)
		}
		m.addItem("assistant", strings.TrimSuffix(sb.String(), "\n"))
	case "/pull":
		if arg == "" {
			m.addItem("assistant", "Usage: /pull <model>  (e.g. /pull qwen2.5-coder:7b)")
			return nil
		}
		o := provider.NewOllama(m.cfg)
		if o.HasModel(m.agentCtx(), arg) {
			m.addItem("assistant", "Model "+arg+" is already pulled locally.")
			return nil
		}
		m.addItem("assistant", "Pulling model "+arg+" from the Ollama library...")
		return func() tea.Msg {
			err := o.Pull(context.Background(), arg)
			return ollamaOpMsg{action: "pull", model: arg, err: err}
		}
	case "/rm", "/remove":
		if arg == "" {
			m.addItem("assistant", "Usage: /rm <model>  (e.g. /rm qwen2.5-coder:7b)")
			return nil
		}
		o := provider.NewOllama(m.cfg)
		m.addItem("assistant", "Removing model "+arg+" from your local Ollama...")
		return func() tea.Msg {
			err := o.Remove(context.Background(), arg)
			return ollamaOpMsg{action: "rm", model: arg, err: err}
		}
	case "/github":
		m.addItem("assistant", "Opening https://github.com/Go-Ducky/cli ...")
		return m.runOpenBrowser("https://github.com/Go-Ducky/cli")
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
	if name != "ollama" && !m.hasProviderKey(name) {
		// Cloud provider with no key yet: collect one inline before switching.
		m.promptKeyFor = name
		m.input.Reset()
		m.input.Placeholder = "paste API key…"
		m.addItem("assistant",
			name+" needs an API key to answer.\n"+
				"Paste your "+name+" API key below and press Enter (Esc to cancel) — "+
				"it's verified live, saved, and then you're switched over automatically.")
		m.input.Focus()
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
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
		"Switched to provider "+name+" (model: "+modelName+").")
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
		m.configPicker()
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
		m.addItem("assistant", configHelp(m))
		return nil
	}
	if err := m.cfg.Set(key, val); err != nil {
		m.addItem("assistant", "Error: "+err.Error())
		return nil
	}
	var pullCmd tea.Cmd
	switch key {
	case "provider":
		return m.switchProvider(val)
	case "model":
		m.applyModel(val)
		pullCmd = m.ensureOllamaModel(val)
	case "auto-approve", "autoapprove", "approve":
		m.agent.SetAutoApprove(m.cfg.Agent.AutoApprove)
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
	return pullCmd
}

// configValuePrompt asks the user to type a value for a config key in the input bar.
func (m *model) configValuePrompt(key, hint string) tea.Cmd {
	m.configPrompt = key
	m.addItem("assistant", "⚙ Setting "+key+": "+hint+"\nType the new value and press Enter (Esc to cancel).")
	m.viewport.SetContent(m.renderBody())
	m.viewport.GotoBottom()
	return nil
}

// configPicker opens the interactive settings menu behind /config.
func (m *model) configPicker() {
	opt := func(name, val string) string { return name + "   " + val }
	excludes := strings.Join(m.cfg.Agent.ExcludeDirs, ", ")
	if excludes == "" {
		excludes = "(none)"
	}
	m.picker = &pickerState{
		title: "GoDucky settings — pick one to change (Esc to cancel)",
		options: []string{
			opt("Provider", m.agent.ProviderName()),
			opt("Model", m.modelName),
			opt("Auto-approve", onOff(m.cfg.Agent.AutoApprove)),
			opt("Ollama host", m.cfg.Ollama.Host),
			opt("Iterations", strconv.Itoa(m.cfg.Agent.MaxIterations)),
			opt("Max output", strconv.Itoa(m.cfg.Agent.MaxOutputChars)),
			opt("Excluded dirs", excludes),
			"── Cancel ──",
		},
		onPick: func(m *model, opt string) tea.Cmd {
			switch {
			case strings.HasPrefix(opt, "──"):
				return nil
			case strings.HasPrefix(opt, "Provider"):
				m.providerPicker()
				return nil
			case strings.HasPrefix(opt, "Model"):
				return m.openModelsPicker()
			case strings.HasPrefix(opt, "Auto-approve"):
				next := !m.cfg.Agent.AutoApprove
				m.cfg.Agent.AutoApprove = next
				m.agent.SetAutoApprove(next)
				if err := m.cfg.Save(); err != nil {
					m.addItem("assistant", "Error saving config: "+err.Error())
					return nil
				}
				m.addItem("assistant", "Auto-approve is now "+onOff(next)+".")
				m.viewport.SetContent(m.renderBody())
				m.viewport.GotoBottom()
				return nil
			case strings.HasPrefix(opt, "Ollama host"):
				return m.configValuePrompt("host", "where your local Ollama runs (e.g. http://localhost:11434)")
			case strings.HasPrefix(opt, "Iterations"):
				return m.configValuePrompt("iterations", "max tool steps per reply (positive integer)")
			case strings.HasPrefix(opt, "Max output"):
				return m.configValuePrompt("output", "max characters per tool result (positive integer)")
			case strings.HasPrefix(opt, "Excluded dirs"):
				return m.configValuePrompt("exclude", "comma-separated directories to skip (e.g. .git,dist)")
			}
			return nil
		},
	}
	m.viewport.GotoBottom()
}

// applyModel sets the model everywhere: the active agent and the active
// provider's config block, so it survives a restart.
func (m *model) applyModel(model string) {
	m.agent.SetModel(model)
	m.modelName = model
	m.cfg.SetProviderModel(m.agent.ProviderName(), model)
}

func configHelp(m *model) string {
	prov := m.agent.ProviderName()
	model := m.modelName
	sb := &strings.Builder{}
	sb.WriteString("You are on " + prov + " with model " + model + ".\n\n")
	sb.WriteString("Type  /config  (no arguments) to open the settings menu, or set keys\ndirectly:\n")
	fmt.Fprintf(sb, "  /config provider  %-16s pick another provider\n", "<name>")
	fmt.Fprintf(sb, "  /config model     %-16s set the model (auto-pulls for Ollama)\n", "<name>")
	fmt.Fprintf(sb, "  /config host      %-16s where local Ollama runs\n", "<url>")
	fmt.Fprintf(sb, "  /config auto-approve on|off   skip permission prompts\n")
	fmt.Fprintf(sb, "  /config iterations <n>        max tool steps per reply\n")
	fmt.Fprintf(sb, "  /config output <n>            max chars per tool result\n")
	fmt.Fprintf(sb, "  /config exclude .git,dist     directories to skip\n")
	sb.WriteString("\nCurrent values:\n")
	fmt.Fprintf(sb, "  provider        : %s\n", prov)
	fmt.Fprintf(sb, "  model           : %s\n", model)
	if prov == "ollama" {
		fmt.Fprintf(sb, "  ollama host     : %s\n", m.cfg.Ollama.Host)
	}
	fmt.Fprintf(sb, "  auto-approve    : %s\n", onOff(m.cfg.Agent.AutoApprove))
	fmt.Fprintf(sb, "  iterations      : %d\n", m.cfg.Agent.MaxIterations)
	fmt.Fprintf(sb, "  max output      : %d\n", m.cfg.Agent.MaxOutputChars)
	fmt.Fprintf(sb, "  excluded dirs   : %s\n", strings.Join(m.cfg.Agent.ExcludeDirs, ", "))
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
  /models          Pick a model for the current provider (free list for OpenRouter)
  /config          Open the settings menu (or /config <key> <value> to edit directly)
  /model <name>    Set the model (auto-pulls it for local Ollama)
  /provider        Choose a provider interactively (or: /provider <name>)
  /pull <name>     Pull a model through Ollama (e.g. /pull qwen2.5-coder:7b)
  /rm <name>       Remove a local Ollama model
  /save <name>     Save this chat so you can resume it later
  /rename <name>   Rename the current chat
  /sessions        List saved chats (resume with goducky resume <n>)
  /github          Open the GoDucky repo in your browser
  /login           How to add a cloud API key
  /clear           Clear the conversation
  /exit            Quit GoDucky

Controls:
  Enter            Send / run command  ·  Esc  Stop the reply mid-generation
  Arrow up/down    Recall previous prompts (like a terminal history)
  PageUp/PageDown  Scroll the chat   ·   Home/End  Jump to top/bottom
  Mouse            Wheel scrolls the chat; drag selects text and it is copied
                   to your clipboard automatically when you let go
  Ctrl+C           Copy the last reply to the clipboard
  Ctrl+B           Copy the whole conversation to the clipboard
  Ctrl+X           Quit
  Paste with Ctrl+V or right-click

Chats are auto-saved when you quit. Resume with goducky resume <number-or-name>
and rename with goducky rename <number-or-name> <new-name>.

Changing the model under local Ollama checks the name against the Ollama library
and pulls it automatically when missing.
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

// truncateWidth cuts a string to fit width terminal cells (ANSI-free text),
// keeping an ellipsis.
func truncateWidth(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	var sb strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw < 1 {
			rw = 1
		}
		if w+rw > width-1 {
			break
		}
		sb.WriteRune(r)
		w += rw
	}
	return sb.String() + "…"
}
