package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	selStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
)

func RunSelect(title string, options []string) (int, bool, error) {
	if !isTerminal() {
		return 0, false, fmt.Errorf("not an interactive terminal")
	}
	restore, err := enterRaw()
	if err != nil {
		return 0, false, err
	}
	defer restore()

	selected := 0
	lastLines := 0
	w, _, _ := term.GetSize(os.Stdin.Fd())
	if w < 40 {
		w = 80
	}

	render := func() {
		s := menuView(title, options, selected)
		if lastLines > 0 {
			fmt.Printf("\x1b[%dA\x1b[J", lastLines)
		}
		fmt.Print(s)
		lastLines = screenLines(s, w)
	}
	render()
	for {
		k, err := readKey()
		if err != nil {
			return selected, false, err
		}
		switch k {
		case "up":
			if selected > 0 {
				selected--
				render()
			}
		case "down":
			if selected < len(options)-1 {
				selected++
				render()
			}
		case "enter":
			fmt.Println()
			return selected, false, nil
		case "esc", "ctrl+c":
			fmt.Println()
			return selected, true, nil
		}
	}
}

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

func menuView(title string, options []string, selected int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(title) + "\n\n")
	for i, opt := range options {
		if i == selected {
			sb.WriteString(selStyle.Render("❯ "+opt) + "\n")
		} else {
			sb.WriteString("  " + opt + "\n")
		}
	}
	sb.WriteString("\n" + dimStyle.Render("↑/↓ or W/S to move · Enter to pick · Esc to cancel"))
	return sb.String()
}

func screenLines(s string, width int) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		w := runewidthNoAnsi(line)
		if w == 0 {
			count++
		} else {
			count += (w + width - 1) / width
		}
	}
	return count
}

func runewidthNoAnsi(s string) int {
	return lipgloss.Width(s)
}

func isTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}
