package presets

import (
	"fmt"
	"strings"
)

// SettingsReason is why a file was taken for a game's settings, or given back
// as part of the save: a code the window says in the reader's language, with
// the counts some codes carry. Text is the English the CLI and the log say.
type SettingsReason struct {
	Code string `json:"code"`
	N    int    `json:"n,omitempty"`
	Of   int    `json:"of,omitempty"`
	// text is a sentence stored before there were codes, said as it is.
	text string
}

// The codes. Those with counts: changed in N of the Of snapshots, or times,
// in which the rest of the save changed.
const (
	ReasonUnrealConfig    = "unreal-config"
	ReasonContents        = "contents"
	ReasonContentsAndName = "contents-and-name"
	ReasonSnapshots       = "snapshots"
	ReasonChanges         = "changes"
)

var reasonText = map[string]string{
	ReasonUnrealConfig:    "Unreal Engine settings folder",
	ReasonContents:        "display settings in its contents",
	ReasonContentsAndName: "display settings in its contents, and its name",
	ReasonSnapshots:       "changed in %d of the %d snapshots in which the save changed",
	ReasonChanges:         "changed in %d of the %d times the save changed",
}

// Known reports whether it has a code the window can say.
func (r SettingsReason) Known() bool {
	_, ok := reasonText[r.Code]
	return ok
}

// Text is the reason in English.
func (r SettingsReason) Text() string {
	f, ok := reasonText[r.Code]
	switch {
	case !ok:
		return r.text
	case strings.Contains(f, "%d"):
		return fmt.Sprintf(f, r.N, r.Of)
	}
	return f
}

// Encode is the reason as it is stored: its code, then its counts.
func (r SettingsReason) Encode() string {
	f, ok := reasonText[r.Code]
	switch {
	case !ok:
		return r.text
	case strings.Contains(f, "%d"):
		return fmt.Sprintf("%s %d %d", r.Code, r.N, r.Of)
	}
	return r.Code
}

// ParseSettingsReason reads a stored reason: what Encode wrote, or the
// English sentence test builds stored before there were codes. Anything else
// is kept as it is, and said as it is.
func ParseSettingsReason(s string) SettingsReason {
	var r SettingsReason
	if n, _ := fmt.Sscanf(s, "%s %d %d", &r.Code, &r.N, &r.Of); n > 0 && r.Known() && r.Encode() == s {
		return r
	}
	for code, f := range reasonText {
		r = SettingsReason{Code: code}
		if strings.Contains(f, "%d") {
			_, _ = fmt.Sscanf(s, f, &r.N, &r.Of)
		}
		if r.Text() == s {
			return r
		}
	}
	return SettingsReason{text: s}
}
