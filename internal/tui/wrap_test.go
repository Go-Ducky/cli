package tui

import (
	"strings"
	"testing"
)

func TestWrapLineShort(t *testing.T) {
	text := "hola"
	if got := wrapText(text, 80); got != text {
		t.Fatalf("short text altered: %q", got)
	}
}

func TestWrapLineLongWords(t *testing.T) {
	text := strings.Join([]string{
		"este", "es", "un", "mensaje", "en",
		"español", "que", "debería", "envolverse",
	}, " ")
	got := wrapText(text, 15)
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 15 {
			t.Fatalf("line too long (%d): %q in %q", len([]rune(line)), line, got)
		}
	}
	if strings.Count(got, "\n") < 2 {
		t.Fatalf("expected a multi-line wrap, got %q", got)
	}
}

func TestWrapRunesCJK(t *testing.T) {
	// Czech example: over-long single token must wrap without corrupting runes.
	text := "přílišžluťoučkýkůňpřeskákalpotůcky"
	got := wrapText(text, 12)
	for _, line := range strings.Split(got, "\n") {
		if line == "" {
			t.Fatalf("empty line in %q", got)
		}
	}
	joined := strings.ReplaceAll(got, "\n", "")
	if joined != text {
		t.Fatalf("runes were altered: %q != %q", joined, text)
	}
}
