package api

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/presets"
)

// A square icon with a red middle on a white field.
func testIcon(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 96 && x < 160 && y >= 96 && y < 160 {
				c = color.RGBA{220, 20, 20, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func near(got color.Color, want color.RGBA, tol int) bool {
	r, g, b, _ := got.RGBA()
	d := func(a uint32, b uint8) int { return int(a>>8) - int(b) }
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(d(r, want.R)) <= tol && abs(d(g, want.G)) <= tol && abs(d(b, want.B)) <= tol
}

// The icon is placed whole in the middle of a cover of the shape asked for,
// over a darker copy of itself — not cropped to fit.
func TestIconCoverKeepsTheIconWhole(t *testing.T) {
	icon := testIcon(t)
	for _, tc := range []struct {
		portrait bool
		w, h     int
	}{{false, wideCoverW, wideCoverH}, {true, tallCoverW, tallCoverH}} {
		out, err := iconCover(icon, tc.portrait)
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != tc.w || b.Dy() != tc.h {
			t.Fatalf("portrait=%v: %dx%d, want %dx%d", tc.portrait, b.Dx(), b.Dy(), tc.w, tc.h)
		}
		if c := img.At(tc.w/2, tc.h/2); !near(c, color.RGBA{220, 20, 20, 255}, 30) {
			t.Errorf("portrait=%v: the middle is %v, not the icon's red", tc.portrait, c)
		}
		// The icon's white corner is inside the cover, whole...
		side := min(tc.w, tc.h)
		x, y := (tc.w-side)/2+4, (tc.h-side)/2+4
		if c := img.At(x, y); !near(c, color.RGBA{255, 255, 255, 255}, 20) {
			t.Errorf("portrait=%v: the icon's corner at %d,%d is %v — cropped?", tc.portrait, x, y, c)
		}
		// ...and the backdrop around it is darker.
		if c := img.At(1, 1); near(c, color.RGBA{255, 255, 255, 255}, 60) {
			t.Errorf("portrait=%v: the backdrop is not darkened: %v", tc.portrait, c)
		}
	}
}

// Asked by title id, the cover is made from the icon an emulator here keeps.
func TestSwitchCoverComesFromTheEmulatorsIcon(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	ts := startTestServer(t)
	home := t.TempDir()
	ts.server.Daemon.Scanner = &presets.Scanner{GOOS: "linux", HomeDir: home}
	const zelda = "0100F2C0115B6000"
	dir := filepath.Join(home, ".cache", "eden", "game_list")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, zelda+".jpeg"), testIcon(t), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.base + "/api/cover?titleId=" + zelda + "&name=Tears+of+the+Kingdom")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	img, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != wideCoverW || b.Dy() != wideCoverH {
		t.Errorf("%dx%d", b.Dx(), b.Dy())
	}

	// A title id on its own is enough to ask with; a malformed one is not.
	if resp, _ := ts.do(t, http.MethodGet, "/api/cover?titleId="+zelda+"&portrait=1", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("portrait by title id alone: %d", resp.StatusCode)
	}
	if resp, _ := ts.do(t, http.MethodGet, "/api/cover?titleId=../../etc", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a malformed title id: %d, want 400", resp.StatusCode)
	}
}
