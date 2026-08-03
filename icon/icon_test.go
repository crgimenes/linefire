package icon

import (
	"image/color"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// The icon has to be the SHIP, drawn from this module's own vector art. What
// would silently stop being true: the asset was found (not the fallback
// outline), the hull wears the colour the art gives it, and it sits inside the
// field rather than running off it.
func TestTheIconIsTheShipInTheArtsOwnColour(t *testing.T) {
	const size = 128
	img := Ship(filoio.OSFS(), "../gameassets", "player", color.RGBA{}, size)
	if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
		t.Fatalf("icon is %v, want %dx%d", img.Bounds(), size, size)
	}

	// player.lfa is drawn in #80ffff, and nothing but the asset decides that.
	_, _, _, own := hull(filoio.OSFS(), "../gameassets", "player")
	want := color.RGBA{R: 0x80, G: 0xff, B: 0xff, A: 0xff}
	if own != want {
		t.Fatalf("the player hull's colour came out %v, want %v (the fallback would be a hint the asset was not read)", own, want)
	}

	near := func(c color.RGBA) bool {
		d := func(a, b uint8) int { return int(a) - int(b) }
		return abs(d(c.R, own.R)) < 60 && abs(d(c.G, own.G)) < 60 && abs(d(c.B, own.B)) < 60
	}
	ink := 0
	for y := range size {
		for x := range size {
			r, g, b, _ := img.At(x, y).RGBA()
			if near(color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}) {
				ink++
			}
		}
	}
	if ink < 200 {
		t.Fatalf("only %d pixels wear the hull's colour: it was not drawn", ink)
	}

	// The corners are the rounded field, so they must be transparent, and the
	// middle must be ink: an icon drawn off its own canvas passes every other
	// check here.
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Error("the corner is opaque: the field is not rounded")
	}
	if _, _, _, a := img.At(size/2, size/2).RGBA(); a == 0 {
		t.Error("the middle of the icon is empty")
	}
}

// A tint overrides the art's colour, which is how one drawing serves two
// products: the campaign wears the player's cyan, a skirmish fleet wears its
// faction's own.
func TestATintOverridesTheArtsColour(t *testing.T) {
	red := color.RGBA{R: 0xff, G: 0x55, B: 0x55, A: 0xff}
	img := Ship(filoio.OSFS(), "../gameassets", "enemy", red, 64)

	found := false
	for y := range 64 {
		for x := range 64 {
			r, g, b, _ := img.At(x, y).RGBA()
			c := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
			if c.R > 0xd0 && c.G > 0x30 && c.G < 0x80 && c.B > 0x30 && c.B < 0x80 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the tint never reached the hull")
	}
}

// Every size a window manager or an .iconset might ask for has to come out
// DRAWN, not scaled from one bitmap — 16px is where a bad rasterizer shows.
func TestTheIconDrawsAtEverySizeItIsAskedFor(t *testing.T) {
	for _, s := range []int{16, 32, 64, 128, 256, 512, 1024} {
		img := Ship(filoio.OSFS(), "../gameassets", "player", color.RGBA{}, s)
		if img.Bounds().Dx() != s {
			t.Fatalf("asked for %dpx, got %v", s, img.Bounds())
		}
		ink := 0
		for y := range s {
			for x := range s {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
					ink++
				}
			}
		}
		if ink < s*s/2 {
			t.Errorf("%dpx icon is mostly empty (%d of %d pixels)", s, ink, s*s)
		}
	}
	if _, err := PNG(filoio.OSFS(), "../gameassets", "player", color.RGBA{}, 64); err != nil {
		t.Fatalf("encoding: %v", err)
	}
}

// An asset that cannot be read costs the icon its fidelity and nothing else: a
// program must not fail to start because it could not draw its own face.
func TestAMissingAssetStillDrawsSomething(t *testing.T) {
	img := Ship(filoio.OSFS(), "../gameassets", "no-such-ship", color.RGBA{}, 64)
	if img.Bounds().Dx() != 64 {
		t.Fatalf("a missing asset produced %v", img.Bounds())
	}
	if _, _, _, a := img.At(32, 32).RGBA(); a == 0 {
		t.Fatal("the fallback drew nothing at all")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
