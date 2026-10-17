package store

import (
	"strings"
	"testing"
)

func TestCleanGameName(t *testing.T) {
	for in, want := range map[string]string{
		"Hades":                  "Hades",
		"  Hades II  ":           "Hades II",
		"Elden Ring: Nightreign": "Elden Ring: Nightreign",
		"ゼルダの伝説":                 "ゼルダの伝説",
		strings.Repeat("é", 120): strings.Repeat("é", 120),
	} {
		got, err := CleanGameName(in)
		if err != nil || got != want {
			t.Errorf("CleanGameName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for why, in := range map[string]string{
		"empty":               "",
		"only spaces":         "   \t ",
		"too long":            strings.Repeat("a", 121),
		"a line break":        "Hades\nII",
		"a control char":      "Hades\x00",
		"a tab in the middle": "Hades\tII",
	} {
		if got, err := CleanGameName(in); err == nil {
			t.Errorf("%s: CleanGameName(%q) = %q, want an error", why, in, got)
		}
	}
}
