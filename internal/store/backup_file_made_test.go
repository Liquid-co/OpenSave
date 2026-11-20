package store

import "testing"

func TestBackupFileMade_NoneThenTheLatest(t *testing.T) {
	s := openTestStore(t)
	if _, ok, err := s.LastBackupFile(); err != nil || ok {
		t.Fatalf("before any: ok=%v err=%v, want none", ok, err)
	}
	if err := s.NoteBackupFileMade(1000, 11); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteBackupFileMade(2000, 12); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.LastBackupFile()
	if err != nil || !ok || got.MadeMs != 2000 || got.Games != 12 {
		t.Fatalf("got %+v ok=%v err=%v, want the second one only", got, ok, err)
	}
}
