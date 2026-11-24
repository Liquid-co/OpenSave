package cliapp

import "testing"

const gamesWithMatch = `{"duck":{"name":"Duck","gameDBSuggestion":{"appId":"312530","name":"Duck Game"}},"ark":{"name":"ARK"}}`

// The game database's match is linked, or declined, as the Config tab does it.
func TestGameDBLinksOrDismisses(t *testing.T) {
	stub := startStubDaemon(t, map[string]answer{
		"GET /api/games":                   {200, gamesWithMatch},
		"POST /api/games/duck/gamedb-link": {200, `{"success":true}`},
	})
	if code := cmdGameDB([]string{"duck", "link"}); code != 0 {
		t.Fatalf("link exited %d", code)
	}
	if code := cmdGameDB([]string{"duck", "dismiss"}); code != 0 {
		t.Fatalf("dismiss exited %d", code)
	}
	got := stub.requests("POST", "/api/games/duck/gamedb-link")
	if len(got) != 2 || got[0].Body["accept"] != true || got[1].Body["accept"] != false {
		t.Errorf("sent %+v", got)
	}
}

// A game with nothing suggested is refused before anything is sent, and so is
// an answer that is neither.
func TestGameDBRefusesWithoutASuggestion(t *testing.T) {
	stub := startStubDaemon(t, map[string]answer{
		"GET /api/games":                  {200, gamesWithMatch},
		"POST /api/games/ark/gamedb-link": {200, `{"success":true}`},
	})
	for _, args := range [][]string{{"ark", "link"}, {"duck", "maybe"}, {"duck"}} {
		if code := cmdGameDB(args); code == 0 {
			t.Errorf("%v accepted", args)
		}
	}
	if n := len(stub.requests("POST", "/api/games/ark/gamedb-link")) + len(stub.requests("POST", "/api/games/duck/gamedb-link")); n != 0 {
		t.Errorf("%d answers sent, want none", n)
	}
}
