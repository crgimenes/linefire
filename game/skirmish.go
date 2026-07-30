package game

import (
	"image/color"
	"io/fs"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/effects"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/weapon"
)

// Linefire Skirmish is the attract demo as a product: the game running itself on
// a transparent, undecorated, click-through window over the desktop. This file is
// the whole of skirmish mode — everything else is the game as it already is,
// which is the point. The window itself belongs to the caller (linefire-skirmish
// sets it up, NeoFrame-style); what the mode owns is the parts of the game that
// assume an opaque screen or a player at the keyboard:
//
//   - no title and no credits text: the demo IS the show, not a backdrop for one;
//   - no background fill and no fog of war: the desktop is the floor, so the
//     world draws only its lines and lights (see drawWorld and drawBrushFog);
//   - line walls, not the flood view: the cave look is built out of solid fills,
//     which an alpha screen cannot carry;
//   - a fixed arena camera: several ships a side are fighting, so the view cannot
//     ride one of them (see camPose), and the arena is sized to fill it;
//   - ships materialise anywhere on the field instead of walking in from a ring
//     around the player, which is a notion an arena does not have;
//   - no minimap: it maps a cave you cannot see all of, and here you can see all
//     of the arena — it is the whole screen;
//   - no screen shake: the camera is the arena, and jolting the whole field
//     because one ship of sixteen took a hit reads as a fault, not as impact;
//   - no crosshair: the mouse belongs to whatever the user is actually doing;
//   - silent unless asked: a desktop toy does not talk first.
//
// The arrival effect: linefire's tunnel-to-the-next-stage vortex, retuned so it
// reads as a ship materialising rather than as a hole opening.
//
// The vortex lives exactly as long as the wait for the ship, so it finishes and
// vanishes on the frame the hull appears — the motes fall in, the portal goes, and
// the ship is there. Anything that overlapped would read as a ship sliding out of
// a hole that is still open.
const (
	materialiseTicks = 30  // how long the vortex runs before the ship exists (0.5s)
	arrivalRadius    = 48  // rim, world units: linefire's portal circle, opened out
	arrivalMoteSpeed = 6.0 // several full rim-to-centre trips inside that half second
	arrivalMoteScale = 2.0 / 3
	arrivalOpenSpeed = 4.0 // how much of its life it spends widening

	// Arrivals are placed at random — an arena has no "around the player" to spawn
	// on — but drawn from a few candidates, keeping the one furthest from anything
	// already flying. Pure random put ships on top of each other often enough to
	// look broken; this costs a handful of draws.
	arrivalCandidates = 8
	arrivalInset      = 80 // world units kept clear of the walls, so nothing lands in one
)

// arrivalColor is the vortex tint: the cool cyan the game marks navigation with.
var arrivalColor = color.RGBA{0x80, 0xff, 0xff, 0xff}

// arrival is a ship on its way in: what it will be, where it will appear, and how
// long the vortex has left to run.
type arrival struct {
	kind string
	x, y float64
	left int
}

type SkirmishOptions struct {
	Sound bool // create the audio context (default silent)
	Debug bool // start with the F3 debug HUD up
}

// NewSkirmish builds the attract demo for a transparent desktop window: the
// autopilot ship (indestructible, as the attract ship always is), an endless
// horde, and a fresh procedurally generated arena every ~20 seconds. The caller
// runs it with ebiten.RunGameWithOptions and ScreenTransparent.
func NewSkirmish(content fs.FS, mapDir string, opts SkirmishOptions) (*Game, error) {
	player, err := filoio.LoadAssetFS(content, mapDir, "player")
	if err != nil {
		return nil, err
	}
	// The opening level is a placeholder: enterCredits immediately replaces it
	// with a generated arena. Loading the campaign's first map just keeps New's
	// contract (a Game always has a level).
	lvl, err := filoio.LoadLevelFS(content, mapDir, "map0001")
	if err != nil {
		return nil, err
	}

	g := newWithContent(content, player, lvl, mapDir, false)
	g.mapName = creditsMapName
	g.startMap = creditsMapName
	if opts.Sound {
		g.sfx = newSoundBank()
		g.sfx.content = content
		// No cfgPath on purpose: skirmish must never write linefire's config.
		g.sfx.prewarm(weapon.Catalog)
		g.prewarmMusic()
	}

	g.skirmishMode = true // set before enterCredits, which builds the arena from it
	g.transparent = true
	g.arenaCam = true
	g.enterCredits() // the autonomous demo: autopilot, endless horde, indestructible ship
	g.floodView = false
	g.debugHUD = opts.Debug
	return g, nil
}

// arenaWorldSize is how much world the fixed camera shows, which is exactly how
// big the arena should be: the whole field visible, its walls on the edges of the
// screen. Derived from the view rather than picked, so the arena fits whatever
// monitor it lands on.
func (g *Game) arenaWorldSize() (w, h float64) {
	scale := g.camPixelScale()
	if scale <= 0 {
		return 0, 0 // before the first Layout: let GenArena use its default
	}
	sw, sh := g.screenSize()
	return sw / scale, sh / scale
}

// arenaFitsView reports whether the current arena still matches what the camera
// shows. It stops matching the moment the window has a real size (the first
// arena is built before any Layout) and if the monitor ever changes.
func (g *Game) arenaFitsView() bool {
	w, h := g.arenaWorldSize()
	if w <= 0 || h <= 0 || g.level == nil {
		return true // nothing to compare against yet
	}
	return math.Abs(g.level.Size.W-w) < 1 && math.Abs(g.level.Size.H-h) < 1
}

// stepSkirmishMeta is skirmish's slice of stepCreditsMeta: the periodic arena
// regeneration, plus a rebuild when the arena no longer fits the view. No scroll,
// no Konami code, no Esc/R — the window is click-through, so there is no player
// to press them.
func (g *Game) stepSkirmishMeta() {
	g.creditsRegenCD--
	if g.creditsRegenCD <= 0 || !g.arenaFitsView() {
		g.buildCreditsArena() // a fresh field to keep flying and fighting in
	}
}

// spawnArrival opens a vortex somewhere on the field for a ship of the given
// kind. The point is random — an arena has no "around the player" to spawn on —
// but chosen as the clearest of a few draws, so arrivals do not land on top of
// whatever is already fighting.
func (g *Game) spawnArrival(kind string, rng *rand.Rand) {
	minX, minY := g.bounds.minX+arrivalInset, g.bounds.minY+arrivalInset
	spanX := max(g.bounds.maxX-arrivalInset-minX, 1)
	spanY := max(g.bounds.maxY-arrivalInset-minY, 1)

	bestX, bestY, bestClear := minX, minY, -1.0
	for range arrivalCandidates {
		x, y := minX+rng.Float64()*spanX, minY+rng.Float64()*spanY
		clear := g.clearanceAt(x, y)
		if clear <= bestClear {
			continue
		}
		bestX, bestY, bestClear = x, y, clear
	}
	g.arrivals = append(g.arrivals, arrival{kind: kind, x: bestX, y: bestY, left: materialiseTicks})
}

// clearanceAt is the distance from a point to the nearest thing already in the
// arena — a live entity, or another vortex about to deliver one.
func (g *Game) clearanceAt(x, y float64) float64 {
	clear := math.Inf(1)
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 {
			continue
		}
		clear = min(clear, math.Hypot(e.x-x, e.y-y))
	}
	for i := range g.arrivals {
		a := &g.arrivals[i]
		clear = min(clear, math.Hypot(a.x-x, a.y-y))
	}
	return min(clear, math.Hypot(g.x-x, g.y-y))
}

// stepArrivals runs the vortices down and lands the ships they were for.
func (g *Game) stepArrivals() {
	kept := g.arrivals[:0]
	for _, a := range g.arrivals {
		a.left--
		if a.left > 0 {
			kept = append(kept, a)
			continue
		}
		ha := g.hordeAssetFor(a.kind)
		g.entities = append(g.entities, enemyEntity(a.kind, ha.a, ha.mesh, ha.glow, a.x, a.y, 0))
	}
	g.arrivals = kept
}

// drawArrivals draws the open vortices. Under the entities, because a ship comes
// OUT of one.
func (g *Game) drawArrivals(dst *ebiten.Image, cam ebiten.GeoM) {
	scale := g.camPixelScale()
	seconds := float64(ebiten.Tick()) / float64(ebiten.TPS())
	for i := range g.arrivals {
		a := &g.arrivals[i]
		age := 1 - float64(a.left)/materialiseTicks // 0 as it opens, 1 as it closes
		x, y := cam.Apply(a.x, a.y)
		effects.DrawPortal(dst, effects.Portal{
			X: x, Y: y,
			Radius:   arrivalRadius * scale * min(age*arrivalOpenSpeed, 1),
			Col:      arrivalColor,
			Seconds:  seconds,
			Speed:    arrivalMoteSpeed,
			MoteSize: effects.DefaultMoteSize * arrivalMoteScale,
			HideRing: true, // a ship materialises; there is no hole here to fly into
			Fade:     1 - age*age,
			DPR:      g.dpr,
			Scale:    scale,
		})
	}
}
