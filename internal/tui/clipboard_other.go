//go:build !windows

package tui

import "errors"

func winCopyToClipboard(text string) error {
	return errors.New("win32 clipboard unavailable")
}
