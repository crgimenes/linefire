package game

import (
	"fmt"
	"image/color"
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	hudBarBg       = color.RGBA{0x10, 0x18, 0x20, 0xc0}
	hudBarBorder   = color.RGBA{0x80, 0xff, 0xff, 0xff}
	hudLifeColor   = color.RGBA{0x80, 0xff, 0xff, 0xff}
	hudShieldFill  = color.RGBA{0x60, 0xd0, 0xff, 0xff}
	hudDim         = color.RGBA{0x00, 0x00, 0x00, 0xa0}
	hudPanelBg     = color.RGBA{0x06, 0x0c, 0x12, 0xdc} // box behind the top-left readouts
	hudPanelBorder = color.RGBA{0x80, 0xff, 0xff, 0x70}
)

// drawHUD draws the player-facing overlay (health bar, lives, score and the
// end-of-round message). All sizes scale with the device pixel ratio so the HUD
// keeps its physical size at any display density.
func (g *Game) drawHUD(screen *ebiten.Image) {
	g.presentOverlay(screen, g.drawHUDContent)
}

// formatRunTime renders the run clock (frames at TPS) as m:ss.d — a speedrun timer,
// precise to a tenth of a second.
func formatRunTime(ticks int) string {
	tps := ebiten.TPS()
	if tps <= 0 {
		tps = 60
	}
	tenths := ticks * 10 / tps
	return fmt.Sprintf("%d:%02d.%d", tenths/600, (tenths/10)%60, tenths%10)
}

// modsLine is a compact readout of the active combat mods, so a stacked run is
// legible at a glance (the log feed only flashes each pickup). Empty when nothing is
// stacked, so a fresh ship shows no clutter.
func (g *Game) modsLine() string {
	var p []string
	if g.fireLevel > 0 {
		p = append(p, fmt.Sprintf("FIRE %d", g.fireLevel))
	}
	if g.rateLevel > 0 {
		p = append(p, fmt.Sprintf("RATE %d", g.rateLevel))
	}
	if g.damageLevel > 0 {
		p = append(p, fmt.Sprintf("DMG %d", g.damageLevel))
	}
	if g.seekLevel > 0 {
		p = append(p, fmt.Sprintf("HOMING %d", g.seekLevel))
	}
	if n := g.countAllies(modeEscort); n > 0 {
		p = append(p, fmt.Sprintf("ESC %d", n))
	}
	if n := g.countAllies(modeOrbit); n > 0 {
		p = append(p, fmt.Sprintf("DRN %d", n))
	}
	if g.bubbleTime > 0 {
		p = append(p, fmt.Sprintf("SHIELD %ds", g.bubbleTime/60+1))
	}
	if g.reflectTime > 0 {
		p = append(p, fmt.Sprintf("REFLECT %ds", g.reflectTime/60+1))
	}
	if len(p) == 0 {
		return ""
	}
	return "MODS  " + strings.Join(p, "  ")
}

// drawHUDContent draws the HUD at logical sizes into the overlay; presentOverlay
// then scales it to the device resolution, so the text and bars are the same
// physical size on every monitor.
func (g *Game) drawHUDContent(dst *ebiten.Image) {
	// A faction battle has no "the player": bars, pips, score, log and slots all
	// describe a single ship at a keyboard, which skirmish does not have. The
	// developer overlay is the one readout a spectator's debug run still wants.
	if g.skirmishMode {
		if g.debugHUD {
			g.drawDebug(dst)
		}
		return
	}
	const (
		pad     = 12.0
		barW    = 220.0
		barH    = 14.0
		pip     = 14.0
		gap     = 6.0
		shieldH = 6.0
		shieldY = pad + barH + 3
		boostH  = 6.0
		boostY  = shieldY + shieldH + 3
		boxPad  = 8.0
	)
	pipY := boostY + boostH + 8
	scoreY := pipY + pip + 8

	aim := "screen"
	if g.aimLocked {
		aim = "world"
	}
	auto := "off"
	if g.autoFire {
		auto = "ON"
	}
	// ENEMIES leads the readout: it is the room's clear-state at a glance (0 = cleared, the exit
	// opens), so a lingering count flags a straggler before you assume the map is done. HI is the
	// arcade high score to chase.
	hi := 0
	if g.sfx != nil {
		hi = g.sfx.highScore
	}
	score := fmt.Sprintf("ENEMIES %d   SCORE %d   HI %d   TIME %s   AIM %s (Tab)   AUTO %s (G)", g.enemiesLeft(), g.score, hi, formatRunTime(g.runTicks), aim, auto)
	if g.sfx != nil && g.sfx.silent() {
		score += "   MUTED (F6)"
	}
	if g.godMode {
		score += "   GOD (iddqd)"
	}
	if g.endless {
		score += fmt.Sprintf("   RIFT depth %d", g.digDepth) // endless: how deep the one-way run has gone
	}
	loadout := g.slotLine()
	mods := g.modsLine()

	// Loadout, active combat mods, then the objective checklist, drawn below the
	// score; the panel grows to enclose them.
	loadoutY := scoreY + 16.0
	modsY := loadoutY + 16.0
	objTop := loadoutY + 18.0
	if mods != "" {
		objTop = modsY + 18.0
	}
	objLines := g.objectiveLines()
	bottom := objTop - 2
	if len(objLines) > 0 {
		bottom = objTop + float64(len(objLines))*16
	}

	// Panel behind the top-left readouts, so they stay legible over the round-mask
	// frame or the bright world.
	clusterW := barW
	for _, s := range append([]string{score, loadout, mods}, objLines...) {
		w := float64(len(s) * 6)
		if w > clusterW {
			clusterW = w
		}
	}
	bx, by := float32(pad-boxPad), float32(pad-boxPad)
	bw, bh := float32(clusterW+2*boxPad), float32(bottom+boxPad-(pad-boxPad))
	vector.FillRect(dst, bx, by, bw, bh, hudPanelBg, false)
	vector.StrokeRect(dst, bx, by, bw, bh, 1, hudPanelBorder, true)

	// Health bar.
	vector.FillRect(dst, pad, pad, barW, barH, hudBarBg, false)
	frac := float64(g.health) / float64(maxHealth)
	if frac < 0 {
		frac = 0
	}
	vector.FillRect(dst, pad, pad, float32(barW*frac), barH, healthColor(frac), false)
	vector.StrokeRect(dst, pad, pad, barW, barH, 1.5, hudBarBorder, true)

	// Shield bar (thin) just under the health bar.
	vector.FillRect(dst, pad, shieldY, barW, shieldH, hudBarBg, false)
	if g.shield > 0 {
		sf := float64(g.shield) / float64(maxShield)
		vector.FillRect(dst, pad, shieldY, float32(barW*sf), shieldH, hudShieldFill, false)
	}

	// Booster bar (thin) under the shield bar — the afterburner fuel (Shift). It dims while
	// locked out (recharging past the re-arm threshold) and brightens once it is usable again.
	vector.FillRect(dst, pad, boostY, barW, boostH, hudBarBg, false)
	if g.boostFuel > 0 {
		boostFill := hudBoostFill
		if !g.boostArmed {
			boostFill = hudBoostCharging
		}
		vector.FillRect(dst, pad, boostY, float32(barW*g.boostFuel/boostMax), boostH, boostFill, false)
	}

	// Lives, as small upward ship pips under the bars.
	for i := 0; i < g.lives; i++ {
		drawLifePip(dst, pad+float64(i)*(pip+gap), pipY, pip, hudLifeColor)
	}

	ebitenutil.DebugPrintAt(dst, score, pad, int(scoreY))
	ebitenutil.DebugPrintAt(dst, loadout, pad, int(loadoutY))
	if mods != "" {
		ebitenutil.DebugPrintAt(dst, mods, pad, int(modsY))
	}
	for i, line := range objLines {
		ebitenutil.DebugPrintAt(dst, line, pad, int(objTop)+i*16)
	}

	g.drawLog(dst)
	if !g.arenaCam {
		// The minimap maps a cave you can only see part of. An arena camera shows
		// the whole field already, so a small copy of it in the corner is clutter.
		g.drawMinimap(dst)
	}
	g.drawWeaponSlots(dst)

	if g.debugHUD || webDebug {
		g.drawDebug(dst)
	}

	if g.over {
		g.drawGameOver(dst)
	}
	if g.titleBannerTicks > 0 && g.clearBannerTicks == 0 {
		drawTopBanner(dst, []string{g.levelTitle()}, g.titleBannerTicks)
	}
	if g.clearBannerTicks > 0 {
		drawTopBanner(dst, g.clearBannerLines(), g.clearBannerTicks)
	}
	if g.gameWon {
		g.drawStyledScreen(dst, g.victoryLines())
	}
}

// drawDebug prints developer telemetry (toggled with F3) at the bottom-left: frame
// rates, live counts, the motion-blur level and the current render mode.
func (g *Game) drawDebug(dst *ebiten.Image) {
	mode := "classic"
	if g.floodView {
		mode = "flood"
	}
	msg := fmt.Sprintf(
		"FPS %.0f  TPS %.0f  %s\nenemies %d  bullets %d  blur x%d  scale %.1f\nmode %s (F2)  fullscreen F11",
		ebiten.ActualFPS(), ebiten.ActualTPS(), g.memStatLine(),
		g.enemiesLeft(), len(g.enemyShots)+len(g.projectiles), g.blurSamples(), g.dpr,
		mode,
	)
	ebitenutil.DebugPrintAt(dst, msg, 12, dst.Bounds().Dy()-58)
}

// memStatLine reports the memory picture, refreshed every ~half second (ReadMemStats
// briefly stops the world, so not per frame): the live Go heap, the total memory the
// runtime took from the OS (on wasm this is the linear memory, which NEVER shrinks —
// the number iOS ultimately kills the tab over), and completed GC cycles. Watching it
// on a device is how a "starts fast, slowly dies" report becomes a diagnosis.
func (g *Game) memStatLine() string {
	g.memTick--
	if g.memLine == "" || g.memTick <= 0 {
		g.memTick = 30
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		g.memLine = fmt.Sprintf("heap %dMB  sys %dMB  gc %d",
			m.HeapAlloc/(1<<20), m.Sys/(1<<20), m.NumGC)
	}
	return g.memLine
}

// drawLifePip draws one life indicator as an upward-pointing triangle.
func drawLifePip(dst *ebiten.Image, x, y, size float64, col color.RGBA) {
	var p vector.Path
	p.MoveTo(float32(x+size/2), float32(y))
	p.LineTo(float32(x+size), float32(y+size))
	p.LineTo(float32(x), float32(y+size))
	p.Close()

	var cs ebiten.ColorScale
	cs.ScaleWithColor(col)
	vector.FillPath(dst, &p, &vector.FillOptions{}, &vector.DrawPathOptions{
		AntiAlias:  true,
		ColorScale: cs,
	})
}

// drawGameOver shows GAME OVER + its controls in the shared title style (the CRT-cyan scaled glyph,
// each line dimming only its own box — see titleText), so it matches the title and victory screens
// and leaves the frozen world bright around the text.
func (g *Game) drawGameOver(dst *ebiten.Image) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := float64(w) / 2
	g.titleText(dst, "GAME OVER", cx, float64(h)*0.34, 4)

	// Run totals — the payoff for the run, especially a score-attack death deep in the Rift.
	lines := []string{
		fmt.Sprintf("SCORE  %d", g.score),
		fmt.Sprintf("TIME  %s", formatRunTime(g.runTicks)),
	}
	if g.newHighScore {
		lines = append(lines, "NEW HIGH SCORE!")
	} else if g.sfx != nil && g.sfx.highScore > 0 {
		lines = append(lines, fmt.Sprintf("HI  %d", g.sfx.highScore))
	}
	if g.endless {
		lines = append(lines, fmt.Sprintf("REACHED THE RIFT — DEPTH %d", g.digDepth))
	} else {
		lines = append(lines, fmt.Sprintf("STAGES CLEARED  %d", len(g.runLog)))
	}
	y := float64(h) * 0.46
	for _, s := range lines {
		g.titleText(dst, s, cx, y, 1.6)
		y += 30
	}

	controls := "R: RESTART    C: CREDITS"
	if g.checkpoint.valid {
		controls = "R: CHECKPOINT    SHIFT+R: RESTART    C: CREDITS"
	}
	g.titleText(dst, controls, cx, y+24, 1.5)
}

// drawTopBanner prints a short block of lines near the top of the screen, over a slim
// bar that fades out in the last second — a non-blocking notice (the level clear) that
// does not dim the game or steal input. ticks is the frames remaining.
func drawTopBanner(dst *ebiten.Image, lines []string, ticks int) {
	w := dst.Bounds().Dx()
	alpha := 1.0
	if fade := ebiten.TPS(); ticks < fade { // fade over the final ~second
		alpha = float64(ticks) / float64(fade)
	}
	bar := hudDim
	bar.A = uint8(float64(bar.A) * alpha)
	vector.FillRect(dst, 0, 18, float32(w), float32(len(lines)*16+8), bar, false)
	for i, s := range lines {
		ebitenutil.DebugPrintAt(dst, s, w/2-len(s)*3, 22+i*16)
	}
}

// drawStyledScreen draws a centered block of lines in the shared title style (CRT-cyan scaled glyph,
// each line dimming only its own box) — the victory breakdown uses it, so it matches the
// title/game-over look and the world stays bright around the text. Fixed-width rows (the per-stage
// table) still line up: same rune count -> same width -> centered.
func (g *Game) drawStyledScreen(dst *ebiten.Image, lines []string) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := float64(w) / 2
	const gap = 24.0
	y0 := float64(h)/2 - float64(len(lines))*gap/2
	for i, s := range lines {
		if s == "" {
			continue
		}
		g.titleText(dst, s, cx, y0+float64(i)*gap, 1.5)
	}
}

// healthColor fades the health bar from green (full) through yellow to red.
func healthColor(frac float64) color.RGBA {
	clamp := func(v float64) uint8 {
		switch {
		case v <= 0:
			return 0
		case v >= 1:
			return 255
		default:
			return uint8(v * 255)
		}
	}
	return color.RGBA{R: clamp(2 * (1 - frac)), G: clamp(2 * frac), B: 0x30, A: 0xff}
}
