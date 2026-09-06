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

func TestSubCells(t *testing.T) {
	cases := []struct {
		line string
		from int
		to   int
		want string
	}{
		{"hello world", 0, 5, "hello"},
		{"hello world", 6, 11, "world"},
		{"hello world", 3, 8, "lo wo"},
		{"héllo", 1, 4, "éll"},
		{"日本語", 0, 6, "日本語"},
		{"日本語", 0, 2, "日"},
		{"日本語", 2, 4, "本"},
		{"日本語", 4, 6, "語"},
		{"hello", 0, 99, "hello"},
		{"hello", 5, 8, ""},
		{"", 0, 3, ""},
	}
	for _, c := range cases {
		if got := subCells(c.line, c.from, c.to); got != c.want {
			t.Errorf("subCells(%q,%d,%d) = %q, want %q", c.line, c.from, c.to, got, c.want)
		}
	}
}

func TestSelectedTextOrderReverse(t *testing.T) {
	m := &model{
		plainLines: []string{"first line", "second line", "third"},
	}

	b := selPos{row: 0, col: 6}
	e := selPos{row: 2, col: 5}
	m.selStart, m.selEnd = &e, &b
	if got, want := m.selectedText(), "line\nsecond line\nthird"; got != want {
		t.Fatalf("selectedText = %q, want %q", got, want)
	}

	m.selStart, m.selEnd = &selPos{row: 1, col: 2}, &selPos{row: 1, col: 11}
	if got, want := m.selectedText(), "cond line"; got != want {
		t.Fatalf("selectedText single = %q, want %q", got, want)
	}
}
