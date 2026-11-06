package presets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{0xFF, 0xFE}
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

func TestJudgeSettings(t *testing.T) {
	crimson := `<EngineOptionSave _selectPreset="Custom"><EngineOptionVideo Name="_videoOption">
<OptionBool Name="_enableFrameGeneration"/><OptionInt Name="_shadowQuality"/><OptionInt Name="_textureQuality"/>
<OptionBool Name="_vSync"/><OptionInt Name="_displayMode"/><OptionInt Name="_resolution"/><OptionInt Name="_upscaler" Value="DLSS"/>
<OptionFloat Name="_brightness"/><OptionInt Name="_antiAliasing"/></EngineOptionVideo></EngineOptionSave>`
	doom := "configVersion 9\nr_materialAniso \"16\"\nr_mode \"18\"\nr_fullscreen \"1\"\nr_swapInterval \"1\"\nr_lodScale \"1\"\nr_gamma \"1.2\"\n"
	cases := []struct {
		name string
		path string
		data []byte
		want bool
	}{
		{"display settings", "videoconfig.txt", []byte("setting.fullscreen 1\nsetting.mat_vsync 0\nsetting.gamma 1\nsetting.defaultres 1920\nsetting.ssao 1\n"), true},
		{"camelCase identifiers, save word outweighed", "user_engine_option_save.xml", []byte(crimson), true},
		{"engine abbreviations", "DOOMEternalConfig.local", []byte(doom), true},
		{"UTF-16", "GraphicsConfig.xml", utf16le(`<config><ScreenMode>1</ScreenMode><Resolution w="1920"/><Vsync>1</Vsync><AntiAliasing>2</AntiAliasing><ShadowQuality>3</ShadowQuality><TextureQuality>3</TextureQuality></config>`), true},
		{"Unreal settings folder", "Saved/Config/WindowsNoEditor/Input.ini", []byte("[/Script/Engine.InputSettings]\n"), true},
		{"save word, weak settings", "savegame.json", []byte(`{"fullscreen":true,"vsync":true,"resolution":"1080p","gold":12}`), false},
		{"progress in a settings-like file", "config.cfg", []byte("level 4\ngold 120\nhealth 80\n"), false},
		{"a log mentioning the GPU", "output_log.txt", []byte("GPU: RTX; resolution 1920; vsync on; fullscreen; monitor 1; hdr off"), false},
		{"binary", "graphics.cfg", []byte{0x01, 0x00, 0x02, 0x00, 0x03}, false},
		{"long save with a settings section", "state.json", []byte(`{"fullscreen":1,"vsync":1,"resolution":1,"gamma":1,"hdr":0,` + strings.Repeat(`"item":1,`, 5000) + `"x":0}`), false},
	}
	for _, c := range cases {
		if got, _ := judgeSettings(c.path, c.data); got != c.want {
			t.Errorf("%s: %s judged settings=%v, want %v", c.name, c.path, got, c.want)
		}
	}
}

// The game database's save locations are never taken, whatever they hold.
func TestDetectIn_SaveLocationsAreNeverTaken(t *testing.T) {
	root := t.TempDir()
	settings := "fullscreen=1\nvsync=1\nresolution=1920x1080\nshadow_quality=3\nanti_aliasing=2\n"
	for _, p := range []string{"video.ini", "slots/1/video.ini", "keep/video.ini"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o777)
		if err := os.WriteFile(full, []byte(settings), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	got := detectIn(root, []string{"slots", "keep/video.ini"})
	if len(got) != 1 || got[0].Path != "video.ini" {
		t.Errorf("detected %+v, want only video.ini", got)
	}
}
