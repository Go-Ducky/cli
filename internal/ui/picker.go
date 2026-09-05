// Package ui provides small standalone interactive terminal widgets used
// outside the main TUI (first-run wizard, installers).
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	selStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
)

type pickerModel struct {
	title    string
	options  []string
	selected int
	done     bool
	result   int
	cancel   bool
}

func (p pickerModel) Init() tea.Cmd { return nil }

func (p pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.done {
		return p, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "w", "a", "k":
			if p.selected > 0 {
				p.selected--
			}
		case "down", "s", "d", "j":
			if p.selected < len(p.options)-1 {
				p.selected++
			}
		case "enter", " ":
			p.result = p.selected
			p.done = true
		case "esc", "ctrl+c", "q":
			p.cancel = true
			p.done = true
		}
	}
	return p, nil
}

func (p pickerModel) View() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(p.title) + "\n\n")
	for i, opt := range p.options {
		if i == p.selected {
			sb.WriteString(selStyle.Render("❯ "+opt) + "\n")
		} else {
			sb.WriteString("  " + opt + "\n")
		}
	}
	sb.WriteString("\n" + dimStyle.Render("↑/↓ or W/S to move · Enter to pick · Esc to cancel"))
	return sb.String()
}

// RunSelect shows an interactive single-choice menu and returns the chosen index.
// cancel is true when the user pressed Esc / Ctrl+C / Q without choosing.
func RunSelect(title string, options []string) (int, bool, error) {
	p := tea.NewProgram(pickerModel{title: title, options: options})
	m, err := p.Run()
	if err != nil {
		return 0, false, err
	}
	pm := m.(pickerModel)
	return pm.result, pm.cancel, nil
}

// RunConfirm asks a yes/no question with arrow keys. yesLabel and noLabel are
// the two choices. A cancelled selection counts as "no".
func RunConfirm(title, yesLabel, noLabel string) (bool, error) {
	idx, cancel, err := RunSelect(title, []string{yesLabel, noLabel})
	if err != nil {
		return false, err
	}
	if cancel {
		return false, nil
	}
	return idx == 0, nil
}
