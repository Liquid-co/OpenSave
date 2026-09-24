package logging

import (
	"fmt"
	"strings"
	"unicode"
)

// Quote puts s in double quotes for a person to read.
//
// %q quotes for Go source, which doubles every backslash: a Windows path came
// out as "D:\\Games\\Saves" in the Activity log and in errors shown in the
// app. This leaves backslashes alone and still escapes control characters, so
// a game name or path with a newline in it cannot forge a second log line —
// the one thing %q was doing that matters here.
func Quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
