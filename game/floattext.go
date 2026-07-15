package game

import (
	"image/color"
	"strconv"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	maxFloaters     = 32   // hard cap on live floating numbers
	floaterLife     = 50   // frames a number lives (~0.8s at 60 TPS)
	floaterRise     = -0.6 // world units per frame (y-down space, so up = negative)
	floaterScale    = 2.2  // text scale before the DPI factor (big enough to read)
	floaterFadeFrac = 0.5  // fraction of life at full opacity before fading out
	floaterCharW    = 6    // ebitenutil debug font cell width, px
	floaterCharH    = 16   // ebitenutil debug font cell height, px
)

// Damage numbers are colored by their source. Future types (critical, shield) get their
// own color via spawnDamageNumber's arg.
var damageColor = color.RGBA{0xff, 0xf0, 0xc0, 0xff} // gun

// floatText is a short-lived number that drifts up and fades over a world point.
// It stores the string, not a rendered image, so spawning and stepping stay pure
// (no graphics context) and the image is fetched lazily at draw time.
type floatText struct {
	x, y    float64
	vy      float64
	life    int
	maxLife int
	text    string
	col     color.RGBA
}

// spawnDamageNumber pops a number above (x, y), respecting the pool cap.
func (g *Game) spawnDamageNumber(x, y float64, n int, col color.RGBA) {
	if len(g.floaters) >= maxFloaters {
		return
	}
	g.floaters = append(g.floaters, floatText{
		x: x, y: y, vy: floaterRise,
		life: floaterLife, maxLife: floaterLife,
		text: strconv.Itoa(n), col: col,
	})
}

// stepFloaters drifts each number upward and ages it, dropping the dead ones.
// Pure motion, so it is cheap and unit-testable without a graphics context.
func (g *Game) stepFloaters() {
	kept := g.floaters[:0]
	for i := range g.floaters {
		f := g.floaters[i]
		f.life--
		if f.life <= 0 {
			continue
		}
		f.y += f.vy
		kept = append(kept, f)
	}
	g.floaters = kept
}

// numberImage returns a cached white-on-transparent rendering of s, building it on
// first use. The set of strings is tiny (small integers), so the cache never needs
// eviction. White glyphs let the draw tint to any color via ColorScale.
func (g *Game) numberImage(s string) *ebiten.Image {
	img, ok := g.numCache[s]
	if ok {
		return img
	}
	w := utf8.RuneCountInString(s)*floaterCharW + 2
	img = ebiten.NewImage(w, floaterCharH)
	ebitenutil.DebugPrintAt(img, s, 0, 0)
	g.numCache[s] = img
	return img
}

// drawFloaters renders each number at a constant screen size (so it stays legible
// at any zoom): a dark drop shadow for contrast against the bright glow, then the
// colored number. It holds full opacity for the first half of its life, then fades.
func (g *Game) drawFloaters(dst *ebiten.Image, cam ebiten.GeoM) {
	scale := floaterScale * g.dpr
	for i := range g.floaters {
		f := &g.floaters[i]
		img := g.numberImage(f.text)
		w := float64(img.Bounds().Dx())
		h := float64(img.Bounds().Dy())
		sx, sy := cam.Apply(f.x, f.y)

		frac := float64(f.life) / float64(f.maxLife)
		alpha := 1.0
		if frac < floaterFadeFrac {
			alpha = frac / floaterFadeFrac
		}

		shadow := color.RGBA{0, 0, 0, uint8(0.75 * alpha * 255)}
		g.drawGlyph(dst, img, w, h, sx+g.dpr, sy+g.dpr, scale, shadow)
		c := f.col
		c.A = uint8(alpha * float64(f.col.A))
		g.drawGlyph(dst, img, w, h, sx, sy, scale, c)
	}
}

// drawGlyph blits a cached glyph image centered on a screen point, scaled and
// tinted to c (premultiplied via ScaleWithColor, so c's alpha fades it correctly).
func (g *Game) drawGlyph(dst, img *ebiten.Image, w, h, cx, cy, scale float64, c color.RGBA) {
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(cx, cy)
	op.ColorScale.ScaleWithColor(c)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(img, &op)
}
