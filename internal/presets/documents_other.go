//go:build !windows

package presets

// documentsKnownFolder has no shell to ask off Windows; <winDocuments> is
// home\Documents there (see documents_windows.go).
func documentsKnownFolder(home string) string { return "" }
