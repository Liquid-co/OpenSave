package switchtitle

import "testing"

func TestFromSavePath(t *testing.T) {
	for path, want := range map[string]string{
		`C:\Users\a\AppData\Roaming\citron\nand\user\save\0000000000000000\8F3A1B2C4D5E6F708192A3B4C5D6E7F8\0100F2C0115B6000`: "0100F2C0115B6000",
		"/home/deck/.local/share/eden/nand/user/save/0000000000000000/8f3a/0100f2c0115b6000/":                                 "0100F2C0115B6000",
		// A Windows path read on Linux, and the other way round.
		`C:\x\save\0000000000000000\p\0100000000010000`: "0100000000010000",
		// Not the NAND layout: the title id must sit three levels below save/.
		"/home/deck/.local/share/eden/nand/user/save/0100f2c0115b6000": "",
		"/games/Hades/0100f2c0115b6000":                                "",
		"/home/deck/.config/Ryujinx/bis/user/save/0000000000000001/0":  "",
		"/x/save/0000000000000000/p/not-a-title-id":                    "",
		"/x/save/0000000000000000/p/0100F2C0115B600":                   "",
		"": "",
	} {
		if got := FromSavePath(path); got != want {
			t.Errorf("FromSavePath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestFromGameID(t *testing.T) {
	for id, want := range map[string]string{
		"switch-0100f2c0115b6000":                          "0100F2C0115B6000",
		"switch-0100f2c0115b6000-2":                        "0100F2C0115B6000",
		"citron-switch-emulator-title-id-0100f2c0115b6000": "0100F2C0115B6000",
		"eden-switch-emulator-title-id-0100000000010000-3": "0100000000010000",
		"hades":                  "",
		"switch-0100f2c0115b600": "",
		"the-legend-of-zelda-tears-of-the-kingdom": "",
	} {
		if got := FromGameID(id); got != want {
			t.Errorf("FromGameID(%q) = %q, want %q", id, got, want)
		}
	}
	if GameID("0100F2C0115B6000") != "switch-0100f2c0115b6000" {
		t.Error(GameID("0100F2C0115B6000"))
	}
	if FromGameID(GameID("0100F2C0115B6000")) != "0100F2C0115B6000" {
		t.Error("GameID does not read back")
	}
}
