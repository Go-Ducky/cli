//go:build windows

package tui

import (
	"errors"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
	procGlobalFree     = kernel32.NewProc("GlobalFree")
)

func winCopyToClipboard(text string) error {
	buf := utf16.Encode([]rune(text))
	buf = append(buf, 0)
	for attempt := 0; ; attempt++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r == 0 {
			if attempt > 8 {
				return errors.New("clipboard busy")
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}
		defer procCloseClipboard.Call()
		if r, _, _ := procEmptyClipboard.Call(); r == 0 {
			return errors.New("clipboard empty failed")
		}
		h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(buf))*2)
		if h == 0 {
			return errors.New("clipboard alloc failed")
		}
		p, _, _ := procGlobalLock.Call(h)
		if p == 0 {
			procGlobalFree.Call(h)
			return errors.New("clipboard lock failed")
		}
		dst := (*[1 << 30]uint16)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))[:len(buf):len(buf)]
		copy(dst, buf)
		procGlobalUnlock.Call(h)
		if r, _, _ := procSetClipboard.Call(cfUnicodeText, h); r == 0 {
			procGlobalFree.Call(h)
			return errors.New("clipboard set failed")
		}
		return nil
	}
}
