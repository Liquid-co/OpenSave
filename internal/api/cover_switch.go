package api

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png" // an icon may be either
	"math"

	"github.com/opensave/opensave/internal/switchtitle"
)

// A Switch game's cover, made from its icon.
//
// The emulators keep each Switch game's icon (presets/switchtitles.go): the
// game's own, exactly, and on this disk. It is square, and every place a cover
// is shown is wide or tall — cropping it to fit loses the top and bottom or
// the sides, which is usually where the game's title is printed. So the icon
// is placed whole in the middle, over a darkened, blurred copy of itself that
// fills the rest, and served in the shape asked for like any other cover. The
// screens showing it need not know it began as an icon.

const (
	wideCoverW, wideCoverH = 460, 215 // Steam's header, which every wide cover is
	tallCoverW, tallCoverH = 300, 450 // 2:3, as Steam's library art
)

// switchCover is the cover for a Switch title, or nil when no emulator here
// has its icon.
func (s *Server) switchCover(titleID string, portrait bool) []byte {
	if s.Daemon.Scanner == nil {
		return nil
	}
	// A tracked copy's save folder leads to its emulator's own cache, which
	// is the only way to a portable copy's.
	savePath := ""
	if games, err := s.Daemon.Store.ListGames(); err == nil {
		for _, g := range games {
			if switchtitle.Of(g.ID, g.SavePath) == titleID {
				savePath = g.SavePath
				break
			}
		}
	}
	icon := s.Daemon.Scanner.SwitchTitleIcon(titleID, savePath)
	if len(icon) == 0 {
		return nil
	}
	out, err := iconCover(icon, portrait)
	if err != nil {
		return nil
	}
	return out
}

// iconCover makes a wide or tall cover from a square icon.
func iconCover(icon []byte, portrait bool) ([]byte, error) {
	decoded, _, err := image.Decode(bytes.NewReader(icon))
	if err != nil {
		return nil, err
	}
	src := toRGBA(decoded)
	w, h := wideCoverW, wideCoverH
	if portrait {
		w, h = tallCoverW, tallCoverH
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// Opaque first, in the icon's average colour: an icon with see-through
	// corners would otherwise leave the backdrop see-through there too, and a
	// JPEG shows that as black.
	avg := shrink(src, 1).Pix
	fill := [4]uint8{0, 0, 0, 255}
	if a := uint32(avg[3]); a > 0 {
		for c := 0; c < 3; c++ {
			fill[c] = uint8(min(uint32(avg[c])*255/a, 255))
		}
	}
	for i := 0; i < len(dst.Pix); i += 4 {
		copy(dst.Pix[i:i+4], fill[:])
	}

	// The backdrop: the icon shrunk to a few pixels — which is the blur — and
	// stretched back over the whole cover, then darkened so the icon stands
	// clear of it.
	side := max(w, h)
	scaleInto(dst, centred(w, h, side), shrink(src, 8))
	darken(dst, 0.5)

	// The icon, whole. One larger than its place is averaged down to it:
	// sampled instead, a thin line in it — an outline, an edge — is either
	// skipped or picked up whole, and shows as a jagged dark streak.
	size := min(w, h)
	if src.Rect.Dx() > size && src.Rect.Dy() > size {
		src = shrink(src, size)
	}
	scaleInto(dst, centred(w, h, size), src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// centred is a side×side square in the middle of a w×h picture.
func centred(w, h, side int) image.Rectangle {
	x, y := (w-side)/2, (h-side)/2
	return image.Rect(x, y, x+side, y+side)
}

func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	return out
}

// shrink averages src into n×n pixels.
func shrink(src *image.RGBA, n int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for oy := 0; oy < n; oy++ {
		for ox := 0; ox < n; ox++ {
			var sum [4]uint64
			var count uint64
			for y := oy * sh / n; y < (oy+1)*sh/n; y++ {
				for x := ox * sw / n; x < (ox+1)*sw/n; x++ {
					i := src.PixOffset(x, y)
					for c := 0; c < 4; c++ {
						sum[c] += uint64(src.Pix[i+c])
					}
					count++
				}
			}
			if count == 0 {
				continue
			}
			j := out.PixOffset(ox, oy)
			for c := 0; c < 4; c++ {
				out.Pix[j+c] = uint8(sum[c] / count)
			}
		}
	}
	return out
}

// scaleInto draws src stretched over rect of dst — bilinear, over what is
// already there — clipped to dst.
func scaleInto(dst *image.RGBA, rect image.Rectangle, src *image.RGBA) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if sw == 0 || sh == 0 || rect.Empty() {
		return
	}
	clampX := func(v int) int { return min(max(v, 0), sw-1) }
	clampY := func(v int) int { return min(max(v, 0), sh-1) }
	area := rect.Intersect(dst.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
		fy := (float64(y-rect.Min.Y)+0.5)*float64(sh)/float64(rect.Dy()) - 0.5
		y0 := math.Floor(fy)
		wy := fy - y0
		r0, r1 := clampY(int(y0)), clampY(int(y0)+1)
		for x := area.Min.X; x < area.Max.X; x++ {
			fx := (float64(x-rect.Min.X)+0.5)*float64(sw)/float64(rect.Dx()) - 0.5
			x0 := math.Floor(fx)
			wx := fx - x0
			c0, c1 := clampX(int(x0)), clampX(int(x0)+1)
			p00, p10 := src.PixOffset(c0, r0), src.PixOffset(c1, r0)
			p01, p11 := src.PixOffset(c0, r1), src.PixOffset(c1, r1)
			var px [4]float64
			for c := 0; c < 4; c++ {
				top := float64(src.Pix[p00+c])*(1-wx) + float64(src.Pix[p10+c])*wx
				bottom := float64(src.Pix[p01+c])*(1-wx) + float64(src.Pix[p11+c])*wx
				px[c] = top*(1-wy) + bottom*wy
			}
			// Premultiplied "over": what shows through is what the icon's
			// alpha leaves.
			d := dst.PixOffset(x, y)
			keep := 1 - px[3]/255
			for c := 0; c < 4; c++ {
				dst.Pix[d+c] = uint8(math.Round(px[c] + float64(dst.Pix[d+c])*keep))
			}
		}
	}
}

func darken(img *image.RGBA, by float64) {
	for i := 0; i < len(img.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			img.Pix[i+c] = uint8(float64(img.Pix[i+c]) * by)
		}
	}
}
