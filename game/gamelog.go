package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// The game log is a short terminal-style feed at the bottom of the screen: a line
// per IMPORTANT event (a pickup, a swap, something destroyed, a warp) plus quick
// tips, scrolling up as new lines arrive and slowly fading out. It narrates the
// run in the game's CRT voice without ever demanding attention.

const (
	logLife = 300 // frames a line lives (~5s at 60 TPS)
	logFade = 100 // the last frames fade to nothing
	logMax  = 6   // visible lines; older ones scroll off
	logRowH = 16  // logical px per line (the debug font cell height)
	logPadX = 12  // left margin, logical px
	logPadY = 34  // bottom margin, logical px (hugs the screen bottom)
)

// logColor is the feed's terminal green-cyan, distinct from the white HUD text.
var logColor = color.RGBA{0x80, 0xff, 0xc0, 0xff}

// logLine is one feed entry aging toward transparency.
type logLine struct {
	text string
	life int
}

// logf appends a line to the feed, scrolling the oldest off past the cap.
func (g *Game) logf(format string, args ...any) {
	g.log = append(g.log, logLine{text: fmt.Sprintf(format, args...), life: logLife})
	if len(g.log) > logMax {
		g.log = g.log[len(g.log)-logMax:]
	}
}

// stepLog ages the feed, dropping fully faded lines.
func (g *Game) stepLog() {
	kept := g.log[:0]
	for _, l := range g.log {
		l.life--
		if l.life <= 0 {
			continue
		}
		kept = append(kept, l)
	}
	g.log = kept
}

// drawLog renders the feed bottom-up (newest at the bottom, like a terminal),
// each line fading as it ages. Runs in the logical overlay, like the HUD.
func (g *Game) drawLog(dst *ebiten.Image) {
	h := float64(dst.Bounds().Dy())
	pad := float64(logPadY)
	if g.debugHUD {
		pad = 100 // clear the F3 telemetry block while it is on
	}
	n := len(g.log)
	for i, l := range g.log {
		alpha := 1.0
		if l.life < logFade {
			alpha = float64(l.life) / logFade
		}
		img := g.numberImage(l.text)
		w := float64(img.Bounds().Dx())
		ih := float64(img.Bounds().Dy())
		cx := logPadX + w/2
		cy := h - pad - float64(n-1-i)*logRowH
		shadow := color.RGBA{0, 0, 0, uint8(0.7 * alpha * 255)}
		g.drawGlyph(dst, img, w, ih, cx+1, cy+1, 1, shadow)
		c := logColor
		c.A = uint8(alpha * 255)
		g.drawGlyph(dst, img, w, ih, cx, cy, 1, c)
	}
}

func onOffWord(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func mutedWord(muted bool) string {
	if muted {
		return "muted (F6)"
	}
	return "unmuted"
}
