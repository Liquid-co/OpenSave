//go:build windows

package main

import "testing"

func TestFileURIForAToastPicture(t *testing.T) {
	if got := fileURI(`C:\Users\Siva Prakash\AppData\Local\Temp\OpenSave\notifications\ab.jpg`); got != "file:///C:/Users/Siva%20Prakash/AppData/Local/Temp/OpenSave/notifications/ab.jpg" {
		t.Errorf("fileURI = %q", got)
	}
}
