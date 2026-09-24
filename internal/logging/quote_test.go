package logging

import (
	"fmt"
	"strings"
	"testing"
)

func TestQuoteLeavesAWindowsPathAsItIs(t *testing.T) {
	path := `D:\Games\Saves\Elden Ring`
	if got, want := Quote(path), `"D:\Games\Saves\Elden Ring"`; got != want {
		t.Errorf("Quote(%s) = %s, want %s", path, got, want)
	}
	// What this replaces, for the record: %q doubles each backslash.
	if strings.Contains(Quote(path), `\\`) {
		t.Error("a backslash was doubled")
	}
	if !strings.Contains(fmt.Sprintf("%q", path), `\\`) {
		t.Fatalf("fmt's %%q verb no longer doubles backslashes, so this test proves nothing")
	}
}

func TestQuoteKeepsALineOnOneLine(t *testing.T) {
	got := Quote("Game\nwarn: forged entry\r\x07")
	if strings.ContainsAny(got, "\n\r\x07") {
		t.Errorf("a control character got through: %q", got)
	}
	if want := `"Game\nwarn: forged entry\r\u0007"`; got != want {
		t.Errorf("Quote = %s, want %s", got, want)
	}
}

func TestQuoteKeepsUnicodeAndQuotesReadable(t *testing.T) {
	if got, want := Quote(`Pokémon "Violet"`), `"Pokémon "Violet""`; got != want {
		t.Errorf("Quote = %s, want %s", got, want)
	}
}
