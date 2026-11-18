//go:build windows

package presets

import (
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// documentsKnownFolder asks the shell where the Documents folder of the
// account whose home is home actually is. OneDrive's folder backup, and any
// "Location" set in the folder's properties, moves it out of the home folder,
// and games follow it there, as they ask the shell too. Ludusavi resolves
// <winDocuments> the same way (the dirs crate's document_dir, a Known Folder
// lookup).
//
// The answer is about the account this process runs as, so it is only taken
// for that account's home: for any other home, "" and the caller falls back.
//
// Asked once per game in a scan, so the answer is kept per home.
func documentsKnownFolder(home string) string {
	if v, ok := documentsByHome.Load(home); ok {
		return v.(string)
	}
	docs := lookupDocumentsKnownFolder(home)
	documentsByHome.Store(home, docs)
	return docs
}

var documentsByHome sync.Map // home → Documents, "" for no answer

func lookupDocumentsKnownFolder(home string) string {
	profile, err := windows.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DEFAULT)
	if err != nil || !strings.EqualFold(filepath.Clean(profile), filepath.Clean(home)) {
		return ""
	}
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return ""
	}
	return docs
}
