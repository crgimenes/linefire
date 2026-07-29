package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/procgen"
)

// The credits screen is the game running ITSELF (an indestructible, auto-piloted,
// fully-modded ship mowing down an endless horde — a busy attract-mode backdrop) with
// the credits scrolling up on top, each line OUTLINED so the demo keeps its glow around
// the text. An easter egg — the Konami code — hands control to the player, so you can play
// the credits. Reached from the title and the end screens; Esc backs OUT to whichever of
// those opened the credits (see enterCreditsReturning), R starts a fresh run.

const (
	creditsScrollSpeed  = 0.5 // logical px per frame the credits rise
	creditsWanderFrames = 130 // frames before the autopilot picks a fresh roam target
	creditsTurnRate     = 1.4 // degrees/frame the demo ship turns toward its heading (calm)
)

// konamiSeq is the classic cheat sequence that unlocks player control of the demo.
var konamiSeq = []ebiten.Key{
	ebiten.KeyArrowUp, ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowDown,
	ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	ebiten.KeyB, ebiten.KeyA,
}

// creditsLines is the scrolling text. Keep it honest: who built it and what it
// stands on.
func creditsLines() []string {
	return []string{
		"L I N E F I R E",
		"",
		"a vector shoot'em up",
		"",
		"",
		"CODE",
		"crgimenes",
		"",
		"BUILT ON",
		"Ebitengine  -  hajimehoshi",
		"gion  -  sound synthesis",
		"minigui  -  ui toolkit",
		"Filo  -  the config language",
		"native  -  platform glue",
		"",
		"THANKS",
		"the Go community",
		"everyone who playtested",
		"you, for reaching the end",
		"",
		"",
		"* * *",
		"",
	}
}

// creditsMapName is the reserved name the attract demo runs under (the map itself is generated,
// not a file), so nothing tries to save/reload it.
const creditsMapName = "credits"

// enterCredits builds an autonomous demo on a freshly GENERATED arena (so the backdrop is never
// the same twice — the title and credits both use this): an indestructible ship on a BASIC loadout
// (front gun + laser turret + fire computer, no mods, no wing) so it barely digs the walls and the
// cave survives for it to fly around, an endless horde for it to fight, a random music track, and
// live sound — like an arcade attract mode, not a muted one. resolvePortals is inert in
// creditsMode, so the arena's portal is just decoration.
func (g *Game) enterCredits() {
	g.buildCreditsArena()
	g.titleMode = false // enterCredits is the CREDITS proper, not the title
	g.creditsScroll = 0
	g.creditsPlayable = false
	g.konamiN = 0
	g.logf("CREDITS")
}

// enterCreditsReturning rolls the credits but REMEMBERS the non-playable screen it was opened
// from (the title, game over or victory), so Esc backs out to that screen instead of dropping
// into a game — "Esc returns to the previous screen". A full struct snapshot is safe because
// the credits demo rebuilds every slice/map through New, leaving the saved copy untouched (the
// same idiom enterMap/restart already use); only the audio context is shared, and each screen
// re-derives its sound each frame.
func (g *Game) enterCreditsReturning() {
	// Release the transient buffers FIRST, so the snapshot copies nil pointers
	// instead of images about to be deallocated — the restored screen then simply
	// rebuilds them (and re-hydrates its fog from the discovery grid it keeps).
	g.releaseTransientImages()
	saved := *g // captured BEFORE enterCredits wipes the world
	g.enterCredits()
	g.screenReturn = &saved
}

// creditsRegenFrames regenerates the attract backdrop this often (~20s at 60fps), so the demo is
// never stuck destroying the same cave — a fresh map appears to keep flying and fighting in.
const creditsRegenFrames = 1200

// buildCreditsArena (re)generates the attract arena and its busy demo, PRESERVING the credits meta
// state (scroll, konami progress, title/playable flags), so a periodic regen swaps the map without
// restarting the credits scroll or dropping the title.
func (g *Game) buildCreditsArena() {
	scroll, konami, playable, title := g.creditsScroll, g.konamiN, g.creditsPlayable, g.titleMode
	ret := g.screenReturn // the remembered back-out screen must survive the periodic regen

	// A freshly generated cave (randIntN is auto-seeded, so it varies per launch too); the
	// empty-target exit portal is inert while the demo runs, so it is just decoration. The arena
	// declares NO music — the attract soundtrack is driven separately (updateCreditsMusic) so a
	// full random track plays to its end regardless of when the backdrop swaps.
	lvl := procgen.GenCaveRoom(int64(randIntN(1<<30)), "") // #nosec G115 -- a display seed, not crypto
	name := creditsMapName
	mapDir, startMap := g.mapDir, g.startMap
	sfx := g.sfx
	g.releaseTransientImages()
	*g = *newWithContent(g.content, g.player, lvl, mapDir, false)
	g.sfx, g.mapName, g.startMap = sfx, name, startMap

	// Basic loadout: front gun (slot 0) + laser turret (slot 1) + fire computer. Weaker than the
	// old maxed spectacle, so it stops chewing every wall to rubble.
	g.collectWeapon(catLaser)
	g.slotArsIdx[0], g.slotArsIdx[1] = 0, 1
	g.syncSlots()
	g.hasComputer, g.autoFire = true, true

	// An endless horde makes the backdrop busy on any map.
	g.horde = newHorde(level.Horde{
		Types: []string{"enemy", "rusher", "sniper", "tank"}, Interval: 24, MaxAlive: 18, Ramp: 600, Seed: 7,
	})

	if g.sfx != nil {
		g.sfx.demoMute = false // attract mode plays out loud (respecting the player's own mute), like an arcade
	}
	g.creditsMode = true
	g.creditsScroll, g.konamiN, g.creditsPlayable, g.titleMode = scroll, konami, playable, title
	g.screenReturn = ret
	g.creditsRegenCD = creditsRegenFrames
	g.attractRecords = g.buildAttractRecords() // refresh the records page (New wiped it); reflects the latest run

	// The soundtrack needs no hand-off here: the jukebox lives on the sound bank,
	// which every world reset carries, so the song simply plays on (stepJukebox).

	g.pickWanderTarget() // seed the autopilot's first roam target
}

// enterCreditsPlay is the easter egg payoff: the player takes the wheel (and the
// sound comes back). The ship stays indestructible — it is a toy, not a run.
func (g *Game) enterCreditsPlay() {
	g.creditsPlayable = true
	if g.sfx != nil {
		g.sfx.demoMute = false // hand back the audio; the saved mute preference is respected
	}
	g.logf("PLAYER CONTROL  -  play the credits")
}

// stepCreditsMeta advances the scroll, watches for the Konami code (until unlocked), and
// leaves the credits: Esc backs out to the screen they were opened from (title / game over /
// victory, via screenReturn), while R starts a fresh run from the first map. Returns true when
// it exited. (On the TITLE, Esc quits the program instead — handled in Update.)
func (g *Game) stepCreditsMeta() bool {
	g.creditsRegenCD--
	if g.creditsRegenCD <= 0 {
		g.buildCreditsArena() // swap in a fresh backdrop (keeps the scroll/title/konami)
		return false          // resume the meta next frame on the new arena
	}
	if consumeWebUnlock() {
		return false // web: this gesture just woke the audio after a silent reload — that was its job
	}
	if g.titleMode {
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) || touchJustTapped() {
			g.restartCampaign() // Space (or a tap) begins a real run from level 1
			return true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			g.enterCreditsReturning() // C rolls the credits; Esc will come back to the title
			return true
		}
		return false // Esc (quit) is handled in Update, where it can end the program
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || (!g.creditsPlayable && touchJustTapped()) {
		if g.screenReturn != nil {
			sfx := g.sfx               // keep the live audio context (the snapshot shares it, but be explicit)
			g.releaseTransientImages() // the credits world is over; hand its buffers back now
			*g = *g.screenReturn       // back to the screen the credits were opened from (title / game over / victory)
			g.sfx = sfx
			return true
		}
		g.restartCampaign() // no remembered screen (credits opened directly): a fresh run
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.restartCampaign() // R still starts a fresh run from level 1
		return true
	}
	g.creditsScroll += creditsScrollSpeed
	if !g.creditsPlayable {
		if k, ok := firstKonamiKeyJustPressed(); ok {
			g.konamiN = konamiStep(g.konamiN, k)
			if g.konamiN >= len(konamiSeq) {
				g.enterCreditsPlay()
			}
		}
	}
	return false
}

// konamiStep advances the sequence progress for a single pressed key: the next key
// advances, the first key (re)starts, anything else resets. Reaching len(konamiSeq)
// means the code was entered.
func konamiStep(n int, k ebiten.Key) int {
	if n < len(konamiSeq) && k == konamiSeq[n] {
		return n + 1
	}
	if k == konamiSeq[0] {
		return 1
	}
	return 0
}

// firstKonamiKeyJustPressed returns the first sequence-relevant key pressed this
// frame (the sequence keys are distinct enough that one-at-a-time is fine).
func firstKonamiKeyJustPressed() (ebiten.Key, bool) {
	for _, k := range []ebiten.Key{
		ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
		ebiten.KeyB, ebiten.KeyA,
	} {
		if inpututil.IsKeyJustPressed(k) {
			return k, true
		}
	}
	return 0, false
}

const (
	autoStandoff   = 240.0 // fighting distance the demo keeps from its target
	autoStrafe     = 130.0 // how wide it orbits the target while it fires
	autoAimConeDeg = 26.0  // fire the nose gun only when the target is within this cone of the heading
	wallHug        = 140.0 // distance the demo keeps from a wall while following it (far enough not to snag on a jutting edge)
	wallCorrect    = 1.1   // how hard it steers to hold that distance (gentle, so it does not slam the wall)
	wallLook       = 240.0 // look-ahead distance for the wall-follow target
)

// autoPilot flies the demo ship like a competent player: it HUNTS the nearest enemy it has a shot
// at, holding a fighting standoff and orbiting so it keeps moving and firing; with no target in
// sight it roams the map (clear-path points, whole-map, not just around the start). It faces the
// enemy while hunting (so the forward gun and aimed mounts land) and its heading while roaming.
func (g *Game) autoPilot() bool {
	ex, ey, hunting := g.autoHuntTarget()

	var tx, ty float64
	if hunting {
		// a point at standoff on the ship's side of the enemy, swept tangentially so it circles.
		nx, ny := g.x-ex, g.y-ey
		d := math.Hypot(nx, ny)
		if d < 1 {
			d = 1
		}
		nx, ny = nx/d, ny/d
		tang := math.Sin(float64(ebiten.Tick())*0.02) * autoStrafe
		tx = ex + nx*autoStandoff - ny*tang
		ty = ey + ny*autoStandoff + nx*tang
	} else if wdx, wdy, ok := g.wallFollowDir(); ok {
		tx, ty = g.x+wdx*wallLook, g.y+wdy*wallLook // no target in sight: trace the walls to roam
	} else {
		g.creditsWanderCD--
		if g.creditsWanderCD <= 0 || math.Hypot(g.creditsWanderX-g.x, g.creditsWanderY-g.y) < 45 {
			g.pickWanderTarget()
		}
		tx, ty = g.creditsWanderX, g.creditsWanderY
	}

	dx, dy := tx-g.x, ty-g.y
	d := math.Hypot(dx, dy)
	moved := false
	if d > 12 {
		g.vx += dx / d * thrust * 0.55
		g.vy += dy / d * thrust * 0.55
		moved = true
	}

	fx, fy := g.vx, g.vy // face travel while roaming...
	if hunting {
		fx, fy = ex-g.x, ey-g.y // ...but face the enemy to aim while fighting
	}
	if math.Hypot(fx, fy) > 0.3 {
		g.angle = turnToward(g.angle, math.Atan2(fy, fx)*180/math.Pi, creditsTurnRate*1.6)
	}
	// Fire the nose gun only at an enemy in range that we are aimed at — like a real
	// player, not a constant stream into empty space. The aimed mounts auto-fire on
	// their own (autoFire is on for the demo), also range-gated.
	if hunting {
		tdx, tdy := ex-g.x, ey-g.y
		if tdx*tdx+tdy*tdy <= autoFireRange*autoFireRange && g.facingWithin(tdx, tdy, autoAimConeDeg) {
			g.fireWeaponSlot(0)
		}
	}
	return moved
}

// facingWithin reports whether the ship's heading is within cone degrees of the
// direction (dx,dy) — so the demo only fires forward when it is actually pointed at
// its target.
func (g *Game) facingWithin(dx, dy, cone float64) bool {
	if dx == 0 && dy == 0 {
		return true
	}
	want := math.Atan2(dy, dx) * 180 / math.Pi
	diff := math.Mod(math.Abs(want-g.angle), 360)
	if diff > 180 {
		diff = 360 - diff
	}
	return diff <= cone
}

// wallFollowDir returns a unit direction that follows the nearest wall — tangent to it, correcting
// toward or away to hold wallHug — so a roaming demo traces the cave's contours and covers the map
// instead of milling in one spot. False in open space with no wall within reach (caller wanders).
// It reads the clearance field's gradient: the field rises AWAY from rock, so its gradient points
// away from the nearest wall and the perpendicular runs along it.
func (g *Game) wallFollowDir() (float64, float64, bool) {
	if g.flood == nil {
		return 0, 0, false
	}
	const eps = 6.0
	gx := g.flood.clearanceAt(g.x+eps, g.y) - g.flood.clearanceAt(g.x-eps, g.y)
	gy := g.flood.clearanceAt(g.x, g.y+eps) - g.flood.clearanceAt(g.x, g.y-eps)
	gm := math.Hypot(gx, gy)
	if gm < 1e-3 {
		return 0, 0, false // flat clearance (open space): no wall to follow
	}
	gx, gy = gx/gm, gy/gm // unit, pointing AWAY from the nearest wall
	tx, ty := gy, -gx     // tangent: circle the wall in a consistent direction
	err := (g.flood.clearanceAt(g.x, g.y) - wallHug) / wallHug
	dx := tx - gx*err*wallCorrect // too far -> steer toward the wall; too close -> away
	dy := ty - gy*err*wallCorrect
	m := math.Hypot(dx, dy)
	if m < 1e-6 {
		return tx, ty, true
	}
	return dx / m, dy / m, true
}

// autoHuntTarget returns the nearest enemy the demo has a clear line of sight to (something to
// engage), or false when none is in sight.
func (g *Game) autoHuntTarget() (float64, float64, bool) {
	best := math.MaxFloat64
	var bx, by float64
	found := false
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		dd := (e.x-g.x)*(e.x-g.x) + (e.y-g.y)*(e.y-g.y)
		if dd < best && g.lineOfSight(g.x, g.y, e.x, e.y) {
			best, bx, by, found = dd, e.x, e.y, true
		}
	}
	return bx, by, found
}

// pickWanderTarget chooses a fresh roam point: a random spot anywhere in the map the ship has a
// clear path to (so it explores instead of pushing a wall). Falls back to the level start.
func (g *Game) pickWanderTarget() {
	g.creditsWanderCD = creditsWanderFrames
	b := g.bounds
	for range 8 { // a clear-path point ANYWHERE in the map, so the demo explores the whole arena
		cx, cy := b.minX+randFloat()*b.w(), b.minY+randFloat()*b.h()
		if g.clearPath(g.x, g.y, cx, cy, g.radius) {
			g.creditsWanderX, g.creditsWanderY = cx, cy
			return
		}
	}
	if g.level != nil { // fall back to the start when no clear point was found
		g.creditsWanderX, g.creditsWanderY = g.level.PlayerStart.X, g.level.PlayerStart.Y
	}
}

// drawCredits dims the running demo and draws the rising credits over it, through
// the same logical-space overlay the HUD uses.
func (g *Game) drawCredits(screen *ebiten.Image) {
	g.presentOverlay(screen, g.drawCreditsContent)
}

func (g *Game) drawCreditsContent(dst *ebiten.Image) {
	if g.debugHUD || webDebug {
		g.drawDebug(dst) // ?debug (or F3): telemetry over the attract too, so a device can watch it idle
	}
	if g.titleMode {
		g.drawTitle(dst)
		return
	}
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := float64(w) / 2
	// Easter egg unlocked: the demo becomes a playable toy — drop the dim and the
	// credits entirely, leaving just a small exit hint.
	if g.creditsPlayable {
		g.titleText(dst, "PLAYER CONTROL   ESC: BACK", cx, float64(h)-24, 1.2)
		return
	}
	lines := creditsLines()
	const lineH = 26.0
	total := float64(h) + float64(len(lines))*lineH
	off := math.Mod(g.creditsScroll, total)
	for i, s := range lines {
		if s == "" {
			continue // a blank spacer line draws nothing (no stray box)
		}
		y := float64(h) - off + float64(i)*lineH
		if y < -lineH || y > float64(h) {
			continue
		}
		g.titleText(dst, s, cx, y, 1.5)
	}
	g.titleText(dst, "ESC: BACK", cx, float64(h)-24, 1.2)
}
