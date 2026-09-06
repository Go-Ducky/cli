//go:build windows
// +build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

var consoleIn = windows.Handle(os.Stdin.Fd())

func enterRaw() (func(), error) {
	var mode uint32
	if err := windows.GetConsoleMode(consoleIn, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(consoleIn, 0); err != nil {
		return nil, err
	}
	return func() {
		healed := mode | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT
		healed &^= windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_WINDOW_INPUT
		windows.SetConsoleMode(consoleIn, healed)
	}, nil
}

func readKey() (string, error) {
	var buf [2]uint16
	var n uint32
	if err := windows.ReadConsole(consoleIn, &buf[0], 2, &n, nil); err != nil {
		return "", err
	}
	if n == 0 {
		return readKey()
	}
	if buf[0] == 0 {
		switch buf[1] {
		case 0x48, 0x26:
			return "up", nil
		case 0x50, 0x28:
			return "down", nil
		case 0x4b, 0x4d, 0x25, 0x27:
			return "other", nil
		}
		return "other", nil
	}
	switch buf[0] {
	case '\r', '\n', ' ':
		return "enter", nil
	case 0x03:
		return "ctrl+c", nil
	case 0x1b:
		return "esc", nil
	case 'w', 'W', 'a', 'A', 'k', 'K':
		return "up", nil
	case 's', 'S', 'd', 'D', 'j', 'J':
		return "down", nil
	case 'q', 'Q':
		return "esc", nil
	}
	return "other", nil
}
