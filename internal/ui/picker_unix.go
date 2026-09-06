//go:build !windows
// +build !windows

package ui

import (
	"os"
	"time"

	"github.com/charmbracelet/x/term"
)

func enterRaw() (func(), error) {
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return nil, err
	}
	return func() { term.Restore(os.Stdin.Fd(), state) }, nil
}

func readKey() (string, error) {
	var b [1]byte
	if _, err := os.Stdin.Read(b[:]); err != nil {
		return "", err
	}
	switch b[0] {
	case '\r', '\n', ' ':
		return "enter", nil
	case 0x03:
		return "ctrl+c", nil
	case 'w', 'W', 'a', 'A', 'k', 'K':
		return "up", nil
	case 's', 'S', 'd', 'D', 'j', 'J':
		return "down", nil
	case 'q', 'Q':
		return "esc", nil
	case 0x1b:
		os.Stdin.SetReadDeadline(time.Now().Add(60 * time.Millisecond))
		n, err := os.Stdin.Read(b[:])
		if err != nil || n == 0 || b[0] != '[' {
			return "esc", nil
		}
		var seq []byte
		for {
			os.Stdin.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			m, err := os.Stdin.Read(b[:])
			if err != nil || m == 0 {
				break
			}
			seq = append(seq, b[0])
			if b[0] >= 0x40 && b[0] <= 0x7e {
				break
			}
		}
		os.Stdin.SetReadDeadline(time.Time{})
		switch string(seq) {
		case "A":
			return "up", nil
		case "B":
			return "down", nil
		}
		return "other", nil
	}
	return "other", nil
}
