package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Metadata must remain writable while an unrelated import owns admission.
// Relocation must still refuse without changing the stored path.
func TestMetadataUpdatesBypassHeldImportClaim(t *testing.T) {
	s := openTestStore(t)
	g := Game{ID: "existing", Name: "Existing", SavePath: t.TempDir()}
	if err := s.CreateGame(g); err != nil {
		t.Fatal(err)
	}
	g, err := s.GetGame(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.BeginImportPath(filepath.Join(t.TempDir(), "incoming"), "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	s.claimWaitFor = 20 * time.Millisecond
	g.CoverURL = "https://example.invalid/cover.png"
	g.ExePath = "synthetic-game"
	g.MaxSnapshots = 7
	if err := s.UpdateGame(g); err != nil {
		t.Fatalf("metadata blocked by import: %v", err)
	}
	got, err := s.GetGame(g.ID)
	if err != nil || got.CoverURL != g.CoverURL || got.ExePath != g.ExePath || got.MaxSnapshots != 7 {
		t.Fatalf("metadata not committed: %+v %v", got, err)
	}
	g.SavePath = t.TempDir()
	if err := s.UpdateGame(g); !errors.Is(err, ErrPathClaimBusy) {
		t.Fatalf("relocation escaped claim: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.UpdateGameContext(ctx, g); !errors.Is(err, context.Canceled) {
		t.Fatalf("relocation cancellation: %v", err)
	}
	got, err = s.GetGame(g.ID)
	if err != nil || got.SavePath == g.SavePath {
		t.Fatalf("refused relocation mutated path: %+v %v", got, err)
	}
}
