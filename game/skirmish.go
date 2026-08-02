package game

import (
	"fmt"
	"image/color"
	"io"
	"io/fs"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/effects"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/procgen"
	"github.com/crgimenes/linefire/weapon"
)

// Linefire Skirmish is the game running itself on a transparent, undecorated,
// click-through window over the desktop: two or more FACTIONS of ships fighting
// each other in an arena. There is no player ship at all — every hull is an
// entity on a team, hunting the nearest ship of another team (enemyTarget), in
// its faction's color (factionSkin), firing its faction's bolts. This file is
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
//   - no player HUD: bars, pips, score and log describe a single ship at a
//     keyboard; only the developer overlay remains, behind Debug;
//   - no keys: skirmish reads no input at all — ships answer to their programs,
//     the program answers to its floating panel, and every keystroke belongs to
//     the desktop underneath (see Update);
//   - no arena border: the open field's perimeter is collision only (GenArena
//     declares no stroke), so the screen edge — or the window frame — is the
//     visible border;
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

// factionPalette is the bright half of the VGA palette: the eight colors that
// tell up to eight simultaneous teams apart at a glance. The hue carries the
// faction; the ship art keeps its own saturation and value (render.Retinted),
// so every hull still reads as linefire art. The achromatic pair (white, grey)
// desaturates its fleet instead, which is its own look.
var factionPalette = [...]color.RGBA{
	{0xff, 0x55, 0x55, 0xff}, // light red
	{0x55, 0xff, 0xff, 0xff}, // light cyan
	{0x55, 0xff, 0x55, 0xff}, // light green
	{0xff, 0x55, 0xff, 0xff}, // light magenta
	{0xff, 0xff, 0x55, 0xff}, // yellow
	{0x55, 0x55, 0xff, 0xff}, // light blue
	{0xff, 0xff, 0xff, 0xff}, // white
	{0x55, 0x55, 0x55, 0xff}, // dark grey
}

// maxFactions is the practical ceiling crg set: more than eight teams turns the
// screen to mush, and eight is exactly what the bright VGA colors can name.
const maxFactions = len(factionPalette)

// factionColor is the team's color; factions count from 1.
func factionColor(f int) color.RGBA {
	return factionPalette[(f-1)%maxFactions]
}

// factionShotColors is the faction's bolt: the same hot-core/colored-halo
// relationship the horde's orange shot has, in the faction's hue.
func factionShotColors(f int) (core, glow color.RGBA) {
	glow = factionColor(f)
	lighten := func(v uint8) uint8 {
		return uint8(int(v) + (255-int(v))*2/3) // #nosec G115 -- v plus 2/3 of its headroom to 255 cannot exceed 255
	}
	core = color.RGBA{R: lighten(glow.R), G: lighten(glow.G), B: lighten(glow.B), A: 0xff}
	return core, glow
}

// factionSkinKey caches one ship kind retinted for one team.
type factionSkinKey struct {
	kind    string
	faction int
}

// arrival is a ship on its way in: what it will be, whose it will be, where it
// will appear, and how long the vortex has left to run.
type arrival struct {
	kind    string
	faction int
	x, y    float64
	left    int
}

// Skirmish map modes: the open field is the default show; the maze is the cave
// generator fitted to the screen, where rock blocks sight lines and shots and
// the fight happens around corners.
const (
	SkirmishMapArena = "arena"
	SkirmishMapMaze  = "maze"
)

type SkirmishOptions struct {
	Sound    bool   // create the audio context (default silent)
	Debug    bool   // show the debug HUD (there is no F3 to toggle it: skirmish reads no keys)
	Factions int    // teams sharing the arena, clamped to 1..maxFactions (0 = 2; one is solo practice)
	Map      string // SkirmishMapArena (default) or SkirmishMapMaze

	// The match being fought (see match.go): MatchEndless (default) is the
	// aquarium; MatchLastFleet and MatchTimed are real battles — Ships hulls
	// per faction, no reinforcements, rematch on a fresh field after the
	// outcome. Duration is the timed mode's deadline in SECONDS.
	Mode     string
	Ships    int
	Duration int

	// Programs is one Filo SOURCE per faction (Programs[0] drives faction 1,
	// and so on); an empty entry leaves that faction on the house brain. The
	// contract a program flies under is pilot.go's doc. Reading files is the
	// caller's job: the game takes source text.
	Programs []string

	// IPC is one external driver per faction, same indexing (nil entry = none).
	// The caller owns the transport — usually a child process's stdio — and
	// builds each with NewIPCDriver; the protocol is ipc.go's doc. A faction
	// cannot have both a Program and a driver.
	IPC []*IPCDriver

	// Events is the control plane's outbound stream (JSONL: spawns, shots,
	// hits, deaths, battles staged and decided — see control.go and trace.go);
	// nil keeps the game silent. Commands come back via PostCommand.
	Events io.Writer
}

// NewSkirmish builds the faction battle for a transparent desktop window: an
// endless horde dealt round-robin across the teams, and a fresh procedurally
// generated arena every ~20 seconds. The caller runs it with
// ebiten.RunGameWithOptions and ScreenTransparent.
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
	// One faction is a legitimate setup — a fleet flying alone, which is how a
	// program is watched without an opponent — but an UNSET count still means
	// the usual two.
	g.factions = 2
	if opts.Factions > 0 {
		g.factions = min(opts.Factions, maxFactions)
	}
	switch opts.Map {
	case "", SkirmishMapArena:
		g.skirmishMap = SkirmishMapArena
	case SkirmishMapMaze:
		g.skirmishMap = SkirmishMapMaze
	default:
		return nil, fmt.Errorf("unknown skirmish map %q (want %q or %q)", opts.Map, SkirmishMapArena, SkirmishMapMaze)
	}
	if len(opts.Programs) > 0 {
		g.filoEng = newPilotEngine()
		g.factionAIs, err = compilePilots(g.filoEng, opts.Programs)
		if err != nil {
			return nil, err // a broken program fails at launch, not mid-battle
		}
	}
	for i, d := range opts.IPC {
		if d == nil {
			continue
		}
		if g.factionAIs[i+1] != nil {
			return nil, fmt.Errorf("faction %d has both a Filo program and an IPC driver; pick one", i+1)
		}
		d.faction = i + 1
		if g.ipcDrivers == nil {
			g.ipcDrivers = map[int]*IPCDriver{}
		}
		g.ipcDrivers[i+1] = d
	}
	g.match, err = newMatch(opts.Mode, opts.Ships, opts.Duration*60, g.factions)
	if err != nil {
		return nil, err
	}
	g.trace = newBattleTrace(opts.Events)
	g.ctrl = &controlInbox{}
	// The aquarium starts flowing at once; a real match waits for the window
	// to settle into its size before its opening battle is staged (see
	// stageWhenViewSettles).
	g.enterCredits()
	if g.match.Mode == MatchEndless {
		g.match.staged = true
		g.emitBattleEvent()
	}
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
// arena is built before any Layout) and if the monitor ever changes. The maze
// snaps to whole cave cells, so it is compared against what the generator would
// actually produce for this view, not against the raw view size.
func (g *Game) arenaFitsView() bool {
	w, h := g.arenaWorldSize()
	if w <= 0 || h <= 0 || g.level == nil {
		return true // nothing to compare against yet
	}
	if g.skirmishMap == SkirmishMapMaze {
		w, h = procgen.CaveArenaSize(w, h)
	}
	return math.Abs(g.level.Size.W-w) < 1 && math.Abs(g.level.Size.H-h) < 1
}

// stepSkirmishMeta is skirmish's slice of stepCreditsMeta: the periodic arena
// regeneration, plus a rebuild when the arena no longer fits the view. No scroll,
// no Konami code, no Esc/R — the window is click-through, so there is no player
// to press them.
func (g *Game) stepSkirmishMeta() {
	g.consumeCommand() // the control plane speaks between ticks, on this goroutine
	// A real match owns its arena for its whole length: no regeneration mid
	// battle. The periodic regen is the AQUARIUM's — attract-screen heritage,
	// kept only where nothing ever ends.
	if g.match != nil && g.match.Mode != MatchEndless {
		g.stepMatchMeta()
		return
	}
	g.creditsRegenCD--
	if g.creditsRegenCD <= 0 || !g.arenaFitsView() {
		g.buildCreditsArena() // a fresh field to keep flying and fighting in
	}
}

// spawnArrival opens a vortex somewhere on the field for a ship of the given
// kind, reporting whether it found a spot. The point is random — an arena has
// no "around the player" to spawn on — but chosen as the clearest of a few
// draws, so arrivals do not land on top of whatever is already fighting, and in
// a maze every candidate must sit in reachable open space: a vortex must never
// materialise a ship inside rock or in a sealed pocket. A cramped frame may
// reject every draw; the horde retries next tick. The team is dealt round-robin
// only when the spawn lands, so the factions stay even however long the battle
// runs.
func (g *Game) spawnArrival(kind string, rng *rand.Rand) bool {
	minX, minY := g.bounds.minX+arrivalInset, g.bounds.minY+arrivalInset
	spanX := max(g.bounds.maxX-arrivalInset-minX, 1)
	spanY := max(g.bounds.maxY-arrivalInset-minY, 1)

	// The clearance must cover the HULL of what is arriving, not a fixed number:
	// a ship delivered overlapping rock cannot move in any direction, ever. The
	// pad gives it room to actually leave, not just to exist.
	const arrivalHullPad = 8
	clearance := max(hordeSpawnClearance, assetRadius(g.hordeAssetFor(kind).a)+arrivalHullPad)

	bestX, bestY, bestClear := 0.0, 0.0, -1.0
	for range arrivalCandidates {
		x, y := minX+rng.Float64()*spanX, minY+rng.Float64()*spanY
		if !g.hordeReachable(x, y, clearance) {
			continue // inside rock, or cut off from the field
		}
		clear := g.clearanceAt(x, y)
		if clear <= bestClear {
			continue
		}
		bestX, bestY, bestClear = x, y, clear
	}
	if bestClear < 0 {
		return false
	}
	faction := g.nextFaction + 1
	g.nextFaction = (g.nextFaction + 1) % max(g.factions, 1)
	g.arrivals = append(g.arrivals, arrival{kind: kind, faction: faction, x: bestX, y: bestY, left: materialiseTicks})
	return true
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
	return clear
}

// stepArrivals runs the vortices down and lands the ships they were for, in
// their faction's colors.
func (g *Game) stepArrivals() {
	kept := g.arrivals[:0]
	for _, a := range g.arrivals {
		a.left--
		if a.left > 0 {
			kept = append(kept, a)
			continue
		}
		ha := g.factionSkin(a.kind, a.faction)
		e := g.enemyEntity(a.kind, ha.a, ha.mesh, ha.glow, a.x, a.y, 0)
		e.faction = a.faction
		e.pilot = g.pilotFor(a.faction) // a new ship is a new mind: fresh memory
		g.entities = append(g.entities, e)
		g.traceSpawn(&g.entities[len(g.entities)-1])
	}
	g.arrivals = kept
}

// factionSkin is hordeAssetFor in the team's colors: the same cached asset,
// its meshes retinted to the faction hue. The geometry is shared — a skin is
// only a new color table — and cached per (kind, faction) on the Game; a world
// rebuild drops the cache and it lazily refills.
func (g *Game) factionSkin(kind string, faction int) hordeAsset {
	key := factionSkinKey{kind: kind, faction: faction}
	ha, ok := g.factionSkins[key]
	if ok {
		return ha
	}
	base := g.hordeAssetFor(kind)
	col := factionColor(faction)
	ha = hordeAsset{a: base.a, mesh: base.mesh.Retinted(col), glow: base.glow.Retinted(col)}
	if g.factionSkins == nil {
		g.factionSkins = map[factionSkinKey]hordeAsset{}
	}
	g.factionSkins[key] = ha
	return ha
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
			Col:      factionColor(a.faction), // the vortex announces whose ship is coming
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
