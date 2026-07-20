package game

import (
	"fmt"
	"image/color"
	"maps"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"linefire/filoio"
)

// The title screen is the game's front door: the attract demo (the same autonomous sim the credits
// use — a modded ship fighting an endless horde on the credits arena) runs behind a big "LINEFIRE"
// and a blinking "PRESS SPACE TO START". It reuses the credits attract setup wholesale; the only
// differences are the overlay and that Space begins a real run (Esc quits). See credits.go for the
// shared sim, stepCreditsMeta for the input, and Draw for where drawCredits dispatches here.

var (
	titleAccent  = color.RGBA{0x70, 0xe0, 0xff, 0xff} // CRT cyan
	titleOutline = color.RGBA{0x00, 0x00, 0x00, 0xff} // dark outline so the text reads over the bright demo
)

// outlineDirs are the eight offsets the glyph is stamped at (in the dark outline color)
// to ring it, so the bright text stays legible over the running demo WITHOUT dimming the
// screen — an arcade-style outlined caption, not a panel.
var outlineDirs = [8][2]float64{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
}

// attractIdleFrames is how long the game-over screen waits before returning to the attract
// demo on its own (~15s at 60fps) — the arcade cabinet never sits dead on GAME OVER.
const attractIdleFrames = 15 * 60

// enterTitle raises the title over the attract demo (the game running itself).
func (g *Game) enterTitle() {
	g.enterCredits() // autonomous demo: modded ship, endless horde, autopilot, muted
	g.titleMode = true
}

// titleText draws one line centered at (cx,cy) in the shared system-screen style: the CRT-cyan,
// scaled-up debug glyph the title's prompt uses. Game over, victory and credits all render through
// this, so every system screen reads the same. The text is OUTLINED (a dark ring stamped around the
// glyph) rather than sitting on a dim panel, so the running demo keeps its full glow everywhere and
// a variable-width scrolling list has no ragged box behind it — legible up close, bright from across
// a room (an arcade cabinet at rest).
func (g *Game) titleText(dst *ebiten.Image, text string, cx, cy, scale float64) {
	img := g.numberImage(text)
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	off := 1 + scale*0.35 // outline thickens a little with the text size (bold logo, thin captions)
	for _, d := range outlineDirs {
		g.drawGlyph(dst, img, w, h, cx+d[0]*off, cy+d[1]*off, scale, titleOutline)
	}
	g.drawGlyph(dst, img, w, h, cx, cy, scale, titleAccent)
}

// attractPageFrames is how long each attract page (the title, then the records) holds before
// the front door flips to the other — an arcade cabinet cycling its screens (~8s at 60fps).
const attractPageFrames = 8 * 60

// bestTimesShown caps how many best-time rows the attract records page lists.
const bestTimesShown = 6

// drawTitle draws the front door over the running demo: it alternates a big LINEFIRE with a
// records page (HI score + best times), and always shows the blinking start prompt and the
// credits hint. Every line is outlined (see titleText), so the demo shows through around it.
func (g *Game) drawTitle(dst *ebiten.Image) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := float64(w) / 2

	if g.attractShowingRecords() {
		g.drawStyledScreen(dst, g.attractRecords) // the records table takes the center
	} else {
		g.titleText(dst, "LINEFIRE", cx, float64(h)*0.30, 6)
	}
	if (ebiten.Tick()/30)%2 == 0 { // blink the prompt
		g.titleText(dst, "PRESS SPACE TO START", cx, float64(h)*0.74, 2)
	}
	g.titleText(dst, "C: CREDITS", cx, float64(h)*0.82, 1.2)
	if webMusicBlocked() {
		// Web only: the browser refuses ALL audio until a user gesture (an iOS tab
		// reload lands here silent). Nothing can be played before a tap, so say so.
		g.titleText(dst, "TAP FOR SOUND", cx, float64(h)*0.88, 1.2)
	}
}

// attractShowingRecords reports whether the front door is on its records half this frame. A
// free-running clock drives it, so no page state must survive the periodic backdrop rebuild.
func (g *Game) attractShowingRecords() bool {
	return len(g.attractRecords) > 0 && (ebiten.Tick()/attractPageFrames)%2 == 1
}

// buildAttractRecords assembles the records page from the saved config: the HI score and the
// fastest stage clears. Empty when nothing is recorded yet (so the attract just shows the
// title). Read once per backdrop build (buildCreditsArena), never per frame.
func (g *Game) buildAttractRecords() []string {
	if g.sfx == nil || g.sfx.cfgPath == "" {
		return nil
	}
	cfg := filoio.LoadConfig(g.sfx.cfgPath)
	var out []string
	if cfg.HighScore > 0 {
		out = append(out, "HIGH SCORE", fmt.Sprintf("%d", cfg.HighScore), "")
	}
	if len(cfg.BestTimes) > 0 {
		out = append(out, "BEST TIMES")
		names := slices.Sorted(maps.Keys(cfg.BestTimes))
		if len(names) > bestTimesShown {
			names = names[:bestTimesShown]
		}
		for _, n := range names {
			out = append(out, fmt.Sprintf("%s   %s", n, formatRunTime(cfg.BestTimes[n])))
		}
	}
	return out
}
