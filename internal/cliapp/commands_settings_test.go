package cliapp

import (
	"reflect"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

// A suggested settings file is answered by its path as status prints it, or
// as typed on Windows, and sent as the Config tab sends it.
func TestSettingsAnswersASuggestedFile(t *testing.T) {
	stub := startStubDaemon(t, map[string]answer{
		"POST /api/games/ark/suggested-settings": {200, `{"success":true}`},
	})
	if code := cmdSettings([]string{"ark", "keep", `Saved\Config\GameUserSettings.ini`}); code != 0 {
		t.Fatalf("keep exited %d", code)
	}
	if code := cmdSettings([]string{"ark", "sync", "/Engine.ini"}); code != 0 {
		t.Fatalf("sync exited %d", code)
	}
	got := stub.requests("POST", "/api/games/ark/suggested-settings")
	if len(got) != 2 {
		t.Fatalf("sent %d answers, want 2", len(got))
	}
	if got[0].Body["path"] != "Saved/Config/GameUserSettings.ini" || got[0].Body["keepHere"] != true {
		t.Errorf("keep sent %+v", got[0].Body)
	}
	if got[1].Body["path"] != "Engine.ini" || got[1].Body["keepHere"] != false {
		t.Errorf("sync sent %+v", got[1].Body)
	}

	for _, args := range [][]string{{"ark", "keep"}, {"ark", "maybe", "Engine.ini"}, {"ark"}} {
		if code := cmdSettings(args); code == 0 {
			t.Errorf("%v accepted", args)
		}
	}
	if n := len(stub.requests("POST", "/api/games/ark/suggested-settings")); n != 2 {
		t.Errorf("%d answers sent, want the 2 from before", n)
	}
}

// A file the daemon no longer waits on is refused, not confirmed.
func TestSettingsRefusesAFileNotSuggested(t *testing.T) {
	startStubDaemon(t, map[string]answer{
		"POST /api/games/ark/suggested-settings": {409, `{"error":"that file is not a suggested settings file"}`},
	})
	if code := cmdSettings([]string{"ark", "keep", "save.sav"}); code == 0 {
		t.Error("an answer the daemon refused exited 0")
	}
}

// Status lists only the files still waiting, as they are typed back.
func TestSuggestedSettingsOfListsOnlyThoseWaiting(t *testing.T) {
	d := linksTestDaemon(t)
	g := store.Game{ID: "ark", Name: "ARK", SavePath: t.TempDir()}
	if err := d.Store.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	for _, f := range []store.SettingsFile{
		{GameID: "ark", Path: "Saved/Config/GameUserSettings.ini", Verdict: store.VerdictSuggested, Reason: "graphics settings"},
		{GameID: "ark", Path: "Engine.ini", Verdict: store.VerdictSettings},
		{GameID: "ark", Path: "input.ini", Verdict: store.VerdictDeclined},
	} {
		if err := d.Store.SetSettingsFile(f); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := suggestedSettingsOf(d, g), []string{"Saved/Config/GameUserSettings.ini"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
