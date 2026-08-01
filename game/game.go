// Package game is the Linefire runtime: it loads a player asset and a level and
// runs a minimal ship-in-a-world prototype. The ship stays centered on screen
// always pointing up; the world rotates and translates around it (the camera
// follows the ship's position and heading).
package game

import (
	"image"
	"image/color"
	"io/fs"
	"math"
	"math/rand/v2"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/effects"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/procgen"
	"github.com/crgimenes/linefire/render"
	"github.com/crgimenes/linefire/weapon"

	ui "github.com/crgimenes/minigui"
)

const (
	// winInitW/winInitH are the INITIAL window size (points); the window stays resizable and
	// Layout adapts everything to whatever size it becomes. screenW/screenH below are the
	// design/reference resolution (camera + HUD defaults before Layout runs), not the window.
	winInitW = 1080
	winInitH = 1920

	screenW       = 800
	screenH       = 800
	camScale      = render.CamScale // world units -> logical screen pixels (see render.CamScale)
	turnSpeed     = 3.0             // degrees per frame
	thrust        = 0.18            // forward acceleration per frame (W/Up)
	strafeThrust  = 0.14            // lateral acceleration per frame (Q/E strafe)
	reverseThrust = 0.09            // backward acceleration per frame (S); ~half of thrust, so reverse tops out near half speed
	friction      = 0.96            // velocity retained per frame
	maxSpeed      = 6.0             // world units per frame (caps tunnelling)

	// blurStepPx is the target on-screen travel per accumulation sub-frame: the
	// motion-blur sample count is sized so the fastest-moving point steps at most
	// this many device pixels, which is what hides the stroboscopic doubling.
	blurStepPx = 2.5
	// maxBlurSamples caps the per-frame sub-frame count (and thus the cost).
	maxBlurSamples = 8

	maxHealth     = 100 // player health at full
	startLives    = 3   // lives before game over
	respawnInvuln = 90  // invulnerability frames after taking a hit or respawning

	shakeDecay = 0.86 // screen-shake magnitude retained per frame
	hitShake   = 5.0  // shake added when the player is hit (logical px)
	deathShake = 7.0  // shake added when an enemy is destroyed (logical px)

	wallDamageMinSpeed = 3.0 // below this impact speed, bumping a wall does no damage
	wallDamageScale    = 8.0 // health lost per unit/frame of impact above the floor
	contactDamage      = 25  // health lost when the player rams an enemy
	ramEnemyDamage     = 1   // hull damage the rammed enemy takes
	contactInvuln      = 30  // brief i-frames after a ram so it does not drain every frame
)

// End-screen pacing: death and the finale win used to snap up their screen instantly, cutting
// off the final effects (the DEVOURER collapse, the blow that killed you). Instead the outcome is
// QUEUED and the world keeps simulating its effects for a beat before the screen appears.
const (
	endNone  = iota // nothing queued
	endDeath        // game over waits behind the delay
	endWin          // the victory screen waits behind the delay
)

// endDelayFrames is how long the aftermath plays before the queued end screen shows (~2.5s).
const endDelayFrames = 150

var (
	colorBg       = color.RGBA{0x06, 0x08, 0x0c, 0xff}
	colorEnemy    = color.RGBA{0xff, 0x60, 0x60, 0xff}
	colorPower    = color.RGBA{0x60, 0xff, 0x90, 0xff}
	goalColor     = color.RGBA{0xff, 0xd0, 0x40, 0xff} // objective (exit) zone marker
	goalDoneColor = color.RGBA{0x60, 0xff, 0x90, 0xff} // objective met
)

// segment is a wall line segment in world coordinates.
type segment struct {
	ax, ay, bx, by float64
}

// Game is the ebiten.Game runtime.
type Game struct {
	player    *asset.Asset
	level     *level.Level
	content   fs.FS  // where game data lives: the embedded bundle, or a directory on disk
	mapDir    string // subdirectory of content holding the maps, so portals can load sibling files
	mapName   string // current map's file stem, for same-map portal detection
	startMap  string // the campaign's first map, so a victory or credits exit can replay from the start
	segs      []segment
	bounds    bounds // world bounding box of the map (sizes the nav/fog/flood grids)
	entities  []entity
	nav       *navgrid   // A* grid for enemy pursuit around walls
	disc      *discovery // fog-of-war memory: cells the player has ever seen
	flood     *floodmap  // negative-space map: reachable interior + boundary glow
	floodView bool       // render the flood-fill glow map instead of line walls (F2)
	simpleMap bool       // skip flood + fog (procedural map): fast render, cheap rebuild
	debugHUD  bool       // show the developer overlay (F3): FPS/TPS, counts, mode
	memLine   string     // cached memory readout for the debug overlay
	memTick   int        // frames until the memory readout refreshes
	mapOpen   bool       // the full-screen automap is open (M)
	mapZoom   float64    // automap zoom factor
	roundView bool       // mask the view to a circle ("porthole") (F4)
	glow      *render.Glow
	pglow     *render.Glow  // separate bloom for the centered ship (never reprojected)
	frame     *ebiten.Image // the moving world, rendered ONCE per frame then reprojected
	camPad    float64       // extra centering translation while rendering into the padded frame
	fogMask   *ebiten.Image // device-res buffer reused for the fog-of-war overlay
	fogTex    *ebiten.Image // world-space cleared-area texture (fog-of-war brush)

	navPath *vector.Path // flattened wall outline in world space (built once; even-odd fill = corridors)
	navWork vector.Path  // per-frame scratch: navPath transformed by the camera

	wallMesh   *render.Mesh // walls in world coordinates (built once)
	wallGlow   *render.Mesh // wall strokes scaled by glow, for the bloom
	playerMesh *render.Mesh // player asset geometry
	playerGlow *render.Mesh // player glowing strokes, for the bloom
	radius     float64      // ship collision radius

	dpr        float64 // device pixel ratio (set by Layout): logical px -> device px
	sw, sh     int     // screen size in device pixels
	winW, winH int     // window size in logical points (drives HUD/overlay layout)

	x, y   float64 // ship world position
	angle  float64 // ship heading, degrees (-90 = up)
	vx, vy float64 // velocity, world units per frame

	discX, discY float64 // ship position at the last fog-discovery scan (skip re-scan when unmoved)
	discTick     int     // frame counter for the stationary re-scan cadence

	fogStampX, fogStampY float64     // ship position at the last fog-texture stamp (skip when unmoved)
	fogStamped           bool        // at least one stamp has landed (position (0,0) is a valid start)
	visSegs              []segment   // scratch: walls within the visibility radius, reused per stamp
	visHits              []visHit    // scratch: ray hits, reused per stamp
	visPoly              []vec2      // scratch: the returned polygon, reused per stamp
	fogPath              vector.Path // scratch: the polygon stamped into fogTex, reused per stamp

	miniSpans    [][4]float64 // cached revealed wall spans (world coords) for the minimap
	miniSpansRev int          // discovery revision the cache was built at (0 = never)

	boostFuel  float64 // afterburner fuel remaining (0..boostMax)
	boosting   bool    // the booster is engaged this frame (raises the speed cap)
	boostArmed bool    // false while locked out after emptying, until the fuel recharges past the re-arm threshold

	projectiles  []projectile             // live player projectiles (all weapons share one pool)
	enemyShots   []projectile             // live enemy bullets in world space
	mines        []mine                   // live deployed land mines
	devourer     *devourer                // the live black hole (superweapon), or nil
	devourerAmmo int                      // remaining DEVOURER charges (run-wide, carries across maps)
	digDepth     int                      // how many map edges the player has dug past (drives edge/rift difficulty)
	edgeReturn   string                   // the last authored map, so edge rooms know where their return portal leads
	endless      bool                     // fell into "The Rift" via the boss portal: one-way, no return, until death
	arsenal      []int                    // catalog indices collected, in pickup order (arsenal.go)
	slotArsIdx   [numSlots]int            // which arsenal weapon each slot points at (-1 = empty)
	slots        [numSlots]weaponSlot     // the live weapon + cooldown in each slot
	fx           *effects.Pool            // live cosmetic particles (explosions, pickups)
	shocks       []shockwave              // live expanding blast rings
	floaters     []floatText              // live floating damage numbers
	numCache     map[string]*ebiten.Image // rendered text per string (lazy, reused)
	muzzleFlash  int                      // frames the muzzle flash still shows
	score        int                      // enemies destroyed
	rng          *rand.Rand               // loot-drop RNG (see drops.go)
	runTicks     int                      // frames of active play this run (the speedrun clock; RTA — not rewound by death)

	levelStartTicks  int           // runTicks when this level began (per-level clear time = runTicks - this)
	levelCleared     bool          // objectives met at least once (latch, so the clear note shows once)
	clearBannerTicks int           // frames left to show the non-blocking level-clear banner (0 = hidden)
	titleBannerTicks int           // frames left to flash the stage title on entry (0 = hidden)
	gameWon          bool          // the campaign is finished: the victory screen is up (terminal, sim frozen)
	runLog           []levelResult // per-stage summary of this run, shown on the victory screen
	clearTicks       int           // the level clear time (frames) shown on the banner
	bestTicks        int           // the best clear time for this map (this run's or the stored record)
	newRecord        bool          // this clear beat the stored best
	endKind          int           // endNone/endDeath/endWin: the queued end screen, delayed so effects play out
	endTicks         int           // frames left before the queued end screen appears

	titleMode    bool      // the front-door title screen is up (attract demo + "press space to start")
	creditsMode  bool      // the credits attract screen is running (autonomous demo + scroll)
	skirmishMode bool      // the attract demo as a desktop overlay: no title/credits text, no meta keys (see skirmish.go)
	transparent  bool      // the screen alpha is real (a transparent window): no background fill, no fog
	arenaCam     bool      // the camera is a fixed view of the whole arena (see camPose)
	arrivals     []arrival // vortices about to deliver a ship (skirmish; see skirmish.go)
	factions     int       // skirmish: how many teams share the arena (0 outside skirmish)
	nextFaction  int       // skirmish: round-robin cursor so arrivals keep the teams even

	factionSkins    map[factionSkinKey]hordeAsset // skirmish: per-(kind, faction) retinted meshes
	creditsPlayable bool                          // the Konami code handed control to the player
	creditsScroll   float64                       // credits vertical scroll offset (logical px)
	konamiN         int                           // progress through the Konami sequence
	creditsWanderX  float64                       // autopilot roam target (world x)
	creditsWanderY  float64                       // autopilot roam target (world y)
	creditsWanderCD int                           // frames until the autopilot picks a new roam target
	creditsRegenCD  int                           // frames until the attract backdrop is regenerated (a fresh map)
	attractRecords  []string                      // the HI score + best times, shown on the title's records page (rebuilt per backdrop)
	screenReturn    *Game                         // the non-playable screen the credits were opened from, so Esc backs out to it (nil = none)

	// Secondary-weapon aiming. In locked mode the aim is frozen in world space
	// (via aimRefAngle) so rotating the ship does not swing it.
	aimLocked   bool
	aimRefAngle float64
	fireBlocked bool // ignore the held mouse button until released (e.g. after a menu click)

	godMode   bool                 // iddqd: the ship takes no damage (test far maps without dying)
	cheatBuf  string               // rolling buffer of recently typed letters, matched against the cheat codes
	iconCache map[string]*iconMesh // lazily-loaded pickup meshes for the HUD slot strip

	shakeMag       float64 // current screen-shake magnitude (logical px)
	shakeX, shakeY float64 // shake offset for this frame (device px)

	laserOn          bool    // the laser beam is firing this frame (drives the visual)
	laserX1, laserY1 float64 // laser beam endpoint in world space
	laserHitRock     bool    // the beam terminated on rock (so its tick melts it)

	hasComputer        bool        // the combat computer was picked up (unlocks auto-fire)
	energy             weapon.Pool // ammunition: every weapon but the plain front gun draws on it
	fireLevel          int         // multishot upgrade level (fire-power pickups); fans direct shots
	rateLevel          int         // fire-rate upgrade level (rate pickups); shortens cooldowns
	damageLevel        int         // damage upgrade level (damage pickups); strengthens shots
	seekLevel          int         // homing upgrade level (seek pickups); curves shots to enemies
	allies             []ally      // friendly companions: escorts (formation) + drones (orbit)
	orbitPhase         float64     // shared drone orbit angle, advanced each frame
	bubbleTime         int         // frames of full damage immunity left (bubble shield mod)
	reflectTime        int         // frames of shot reflection left (reflector shield mod)
	checkpoint         checkpoint  // last saved run snapshot (game-over R restores it)
	autoFire           bool        // combat computer engaged (auto-aim + auto-fire aimed weapons)
	autoAimX, autoAimY float64     // auto-fire target direction (unit) this frame
	autoAimOK          bool        // a target was found this frame

	// Laser heat: firing builds heat; at the cap the laser shuts off (laserHot)
	// until it cools back to zero, so sitting on auto-fire is not a free win.
	laserHeat int
	laserHot  bool

	health       int  // player health, 0..maxHealth
	shield       int  // shield points; absorb damage before health
	lives        int  // remaining lives
	invuln       int  // invulnerability frames remaining
	portalGrace  int  // frames after entering a map during which portals do not trigger
	over         bool // game over: all lives lost
	newHighScore bool // this run beat the stored HI score (shown on the game-over/victory screen)
	overIdle     int  // frames the game-over screen has sat idle, so it can auto-return to attract

	horde *horde // optional time-based enemy spawner (level.Horde)

	// Level objectives derived from the map (clear enemies, reach exit zones).
	objectives    []objective
	inZones       map[string]bool // zones the player is currently inside (enter edges)
	firedTriggers map[string]bool // zone triggers that have fired once

	// Resolutions (condition -> routine, fired once per map). hadEnemies gates the
	// "cleared" condition to maps that spawned any; cameFrom is where a "return"
	// routine goes back to (the previous map's name, empty at the run start).
	hadEnemies bool
	cameFrom   string

	// Per-map progress across the run, so revisiting a map through a portal keeps
	// cleared enemies / collected pickups gone. Preserved across enterMap; a full
	// restart drops it.
	mapStates map[string]*mapState

	// Procedural map: non-nil when the player is in the chunked procedural world.
	// procCX/procCY is the chunk the 3x3 loaded neighbourhood is centered on.
	procWorld      *procgen.World
	procCX, procCY int

	// camera state at the previous Draw; the gap to the current state is the
	// motion the accumulation blur spreads across, killing the rotation strobe.
	prevX, prevY, prevAngle float64

	// ESC pause menu (built with the ui toolkit).
	paused bool
	menu   ui.Context

	sfx *soundBank // gion-synthesized sound effects; nil until Run initializes audio

	log []logLine // terminal-style event feed at the bottom of the screen

	// overlay is a logical-size buffer reused for HUD and menu: content is drawn
	// at logical sizes then scaled up by the device ratio, so text stays the same
	// physical size on every monitor instead of shrinking at high DPI.
	overlay *ebiten.Image
}

// presentOverlay draws content into the reused logical overlay and blits it onto
// the device-resolution screen scaled by the DPI, so UI text and shapes keep a
// consistent physical size.
func (g *Game) presentOverlay(screen *ebiten.Image, draw func(dst *ebiten.Image)) {
	if g.deviceScale() == 1 {
		// At 1× (the web profile, plain monitors) the overlay is exactly the screen:
		// its callbacks only ADD content, so drawing straight onto the screen is the
		// same composite minus a fullscreen buffer, its Clear and a fullscreen blit —
		// per overlay, per frame (HUD, map, menu, credits).
		draw(screen)
		return
	}
	lw, lh := g.logicalSize()
	if g.overlay == nil || g.overlay.Bounds().Dx() != lw || g.overlay.Bounds().Dy() != lh {
		g.overlay = ebiten.NewImage(lw, lh)
	}
	g.overlay.Clear()
	draw(g.overlay)

	var op ebiten.DrawImageOptions
	op.GeoM.Scale(g.deviceScale(), g.deviceScale())
	op.Filter = ebiten.FilterLinear
	screen.DrawImage(g.overlay, &op)
}

// New builds a game from a player asset and a level. simple skips the per-area
// flood negative-space map and the fog-of-war (line-wall view, no discovery grid /
// fog texture): the procedural map uses it so a large streamed region renders fast
// and rebuilds cheaply on a chunk crossing.
// New builds a game that reads its data straight from the operating system (the
// default for tests and dev, which pass a directory such as "../gameassets"). Run
// uses newWithContent to serve the same data from the embedded bundle instead.
func New(player *asset.Asset, lvl *level.Level, mapDir string, simple bool) *Game {
	return newWithContent(filoio.OSFS(), player, lvl, mapDir, simple)
}

// newWithContent is New with an explicit content FS, so the shipped binary serves
// assets, maps and music from an embed.FS and portal/restart transitions keep it.
func newWithContent(content fs.FS, player *asset.Asset, lvl *level.Level, mapDir string, simple bool) *Game {
	entities := buildEntities(content, lvl, mapDir)
	g := &Game{
		content:     content,
		player:      player,
		level:       lvl,
		simpleMap:   simple,
		segs:        wallSegments(lvl),
		entities:    entities,
		glow:        render.NewGlow(),
		pglow:       render.NewGlow(),
		wallMesh:    render.BuildLayersMesh(lvl.Walls),
		wallGlow:    render.BuildGlowMesh(lvl.Walls),
		playerMesh:  render.BuildLayersMesh(player.Layers),
		playerGlow:  render.BuildGlowMesh(player.Layers),
		radius:      assetRadius(player),
		dpr:         1,
		sw:          screenW,
		sh:          screenH,
		winW:        screenW,
		winH:        screenH,
		health:      maxHealth,
		lives:       startLives,
		portalGrace: portalGraceFrames,
		x:           lvl.PlayerStart.X,
		y:           lvl.PlayerStart.Y,
		angle:       lvl.PlayerStart.Angle,
		prevX:       lvl.PlayerStart.X,
		prevY:       lvl.PlayerStart.Y,
		prevAngle:   lvl.PlayerStart.Angle,
	}
	g.mapDir = mapDir
	g.rng = newDropRNG()
	g.bounds = mapBounds(lvl, g.segs)
	if !simple {
		// The flood map and fog grid are O(map area); only build them for hand-made
		// maps. The big procedural region runs in plain line-wall view with no fog.
		g.disc = newDiscovery(g.bounds)
		g.flood = buildFloodmap(g.segs, lvl.PlayerStart.X, lvl.PlayerStart.Y, g.bounds)
	}
	// SPIKE: where a region field exists it is the truth (it knows about dug tunnels);
	// the procedural world has none and keeps the segment-derived grid.
	if g.flood != nil {
		g.nav = buildNavgridFromFlood(g.flood)
	} else {
		g.nav = buildNavgrid(g.segs, g.bounds)
	}
	g.freeBuriedSpawns() // un-bury any guard a clipped cave cell left inside rock (else it's unkillable)
	g.objectives = deriveObjectives(lvl, g.entities)
	g.inZones = map[string]bool{}
	g.firedTriggers = map[string]bool{}
	g.hadEnemies = levelHasEnemies(lvl)
	if lvl.Horde != nil {
		g.horde = newHorde(*lvl.Horde)
	}
	g.initArsenal()
	g.boostFuel, g.boostArmed = boostMax, true // start with a full, ready afterburner
	g.numCache = map[string]*ebiten.Image{}
	g.floodView = !simple // negative-space map by default; line walls for procedural (F2)
	g.mapZoom = 1
	g.roundView = false // off by default (full map visible); F4 toggles the porthole
	return g
}

// deviceScale is the device pixel ratio, defaulting to 1 before Layout has run.
func (g *Game) deviceScale() float64 {
	if g.dpr <= 0 {
		return 1
	}
	return g.dpr
}

// screenSize is the drawable size in device pixels, defaulting to the logical
// resolution before Layout has run (keeps the camera testable without a window).
func (g *Game) screenSize() (float64, float64) {
	if g.sw <= 0 || g.sh <= 0 {
		return screenW, screenH
	}
	return float64(g.sw), float64(g.sh)
}

// logicalSize is the window size in logical points (the coordinate space the HUD,
// menu and minimap are laid out in), defaulting to the design resolution.
func (g *Game) logicalSize() (int, int) {
	if g.winW <= 0 || g.winH <= 0 {
		return screenW, screenH
	}
	return g.winW, g.winH
}

// camPixelScale is the world->device-pixel factor: the logical camera scale
// times the device pixel ratio so geometry keeps its size at any DPI.
func (g *Game) camPixelScale() float64 {
	return camScale * g.deviceScale()
}

// appendCameraAt adds the world->screen camera transform for an explicit camera
// pose: translate the ship to the origin, rotate so the heading points up, scale
// to device pixels, then center on screen. Taking the pose as parameters lets
// Draw sample intermediate poses between frames for the accumulation blur.
func (g *Game) appendCameraAt(m *ebiten.GeoM, x, y, angle float64) {
	alpha := (-90 - angle) * math.Pi / 180
	m.Translate(-x, -y)
	m.Rotate(alpha)
	m.Scale(g.camPixelScale(), g.camPixelScale())
	w, h := g.screenSize()
	// camPad centers the ship in the padded frame buffer while it is being rendered
	// (0 for the on-screen camera). The frame is bigger than the screen so reprojected
	// sub-frames never reveal unrendered corners during a turn.
	m.Translate(w/2+g.shakeX+g.camPad, h/2+g.shakeY+g.camPad)
}

// camPose is where the camera sits and how it is turned.
//
// Linefire's camera RIDES THE SHIP: the hull is pinned to the centre of the
// screen pointing up, and the world translates and rotates around it. That is the
// right camera for one ship you are flying — it is an arcade cabinet, and up is
// always where you are going.
//
// An arena camera does not move at all. It looks at the whole field from outside,
// because there is no "your ship" to ride: several ships per side are fighting,
// and a view that turned with one of them would make the other fifteen swing
// around the screen. So the pose is the middle of the map, held still.
//
// Angle -90 is what appendCameraAt turns into zero rotation (it computes
// -90 - angle), so a still camera needs no separate code path there.
func (g *Game) camPose() (x, y, angle float64) {
	if !g.arenaCam {
		return g.x, g.y, g.angle
	}
	return (g.bounds.minX + g.bounds.maxX) / 2, (g.bounds.minY + g.bounds.maxY) / 2, -90
}

// cameraGeoMAt is the world->screen transform for an explicit camera pose.
func (g *Game) cameraGeoMAt(x, y, angle float64) ebiten.GeoM {
	var m ebiten.GeoM
	g.appendCameraAt(&m, x, y, angle)
	return m
}

// cameraGeoM is the world->screen transform (pure, testable without a window).
func (g *Game) cameraGeoM() ebiten.GeoM {
	return g.cameraGeoMAt(g.camPose())
}

// entityGeoMAt places an entity's asset (asset coordinates) at its world position
// facing its heading, then applies the camera for an explicit camera pose. Assets
// are authored pointing up (-90), so they are turned by angle+90 to face heading.
func (g *Game) entityGeoMAt(e *entity, camX, camY, camAngle float64) ebiten.GeoM {
	var m ebiten.GeoM
	if e.a != nil {
		m.Translate(-e.a.Origin.X, -e.a.Origin.Y)
	}
	m.Rotate((e.angle + 90) * math.Pi / 180)
	m.Translate(e.x, e.y)
	g.appendCameraAt(&m, camX, camY, camAngle)
	return m
}

// entityGeoM places an entity through the current camera pose.
func (g *Game) entityGeoM(e *entity) ebiten.GeoM {
	return g.entityGeoMAt(e, g.x, g.y, g.angle)
}

// playerGeoM draws the player asset at the screen center, always pointing up.
// playerWorldGeoM places the player's hull at its world position facing its
// heading, through the camera — the way every other entity is placed. It is what
// an arena camera needs: with the camera still, the ship has to move and turn on
// screen, which is exactly what playerGeoM refuses to do.
func (g *Game) playerWorldGeoM(camX, camY, camAngle float64) ebiten.GeoM {
	var m ebiten.GeoM
	m.Translate(-g.player.Origin.X, -g.player.Origin.Y)
	m.Rotate((g.angle + 90) * math.Pi / 180)
	m.Translate(g.x, g.y)
	g.appendCameraAt(&m, camX, camY, camAngle)
	return m
}

func (g *Game) playerGeoM() ebiten.GeoM {
	var m ebiten.GeoM
	m.Translate(-g.player.Origin.X, -g.player.Origin.Y)
	m.Scale(g.camPixelScale(), g.camPixelScale())
	w, h := g.screenSize()
	m.Translate(w/2+g.shakeX, h/2+g.shakeY)
	return m
}

// Run opens the window and starts the game loop. content is where the game data
// lives (the embedded bundle, or a directory on disk); mapDir is content's map
// subdirectory and mapName is the current map's file stem, so portals can load
// sibling maps by name (and detect same-map targets).
func Run(content fs.FS, player *asset.Asset, lvl *level.Level, mapDir, mapName string) error {
	ebiten.SetWindowSize(winInitW, winInitH)
	ebiten.SetWindowTitle("Linefire")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(480, 360, -1, -1)
	g := newWithContent(content, player, lvl, mapDir, false)
	g.mapName = mapName
	g.startMap = mapName                  // remember where the campaign began, so a victory can replay it
	g.titleBannerTicks = stageTitleFrames // flash the opening stage's title
	webDebug = webFlag("debug")           // web A/B: the debug overlay everywhere, attract included
	if !webFlag("noaudio") {              // web A/B: no audio context at all — zero audio cost on the wasm main thread
		g.sfx = newSoundBank()  // create the audio context here (not in New), so tests stay headless
		g.sfx.content = content // serve music from the same bundle as the rest of the data
		cfgPath, err := filoio.ConfigPath()
		if err == nil {
			cfg := filoio.LoadConfig(cfgPath)
			g.sfx.cfgPath = cfgPath
			g.sfx.master = cfg.Volume
			g.sfx.muted = cfg.Muted
			g.sfx.highScore = cfg.HighScore
		}
		g.sfx.prewarm(weapon.Catalog)
		g.prewarmMusic()
	}
	g.logf("TIP  salvage a combat computer to unlock auto-fire (G)")
	g.logf("TIP  fly over a weapon to swap; your fire can break pickups")
	g.enterTitle() // open on the title screen (attract demo) rather than straight into play
	return ebiten.RunGame(g)
}

// Layout renders at the display's native pixel density: the offscreen the game
// draws into is sized in device pixels and mapped 1:1 to the framebuffer, so
// vector strokes are anti-aliased at full resolution with no resampling step.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := capScale(ebiten.Monitor().DeviceScaleFactor())
	if outsideWidth <= 0 || outsideHeight <= 0 {
		outsideWidth, outsideHeight = screenW, screenH
	}
	g.dpr = scale
	g.winW, g.winH = outsideWidth, outsideHeight
	g.sw = int(math.Round(float64(outsideWidth) * scale))
	g.sh = int(math.Round(float64(outsideHeight) * scale))
	return g.sw, g.sh
}

// Update steps input, physics and collision once.
// updateHotkeys handles the global toggle keys (render modes, overlays, window).
func (g *Game) updateHotkeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		g.floodView = !g.floodView
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.debugHUD = !g.debugHUD
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
		g.roundView = !g.roundView
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.mapOpen = !g.mapOpen
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) || inpututil.IsKeyJustPressed(ebiten.Key4) {
		if g.hasComputer {
			g.autoFire = !g.autoFire // combat computer on/off (slot 4)
			g.logf("AUTO-FIRE  %s", onOffWord(g.autoFire))
		} else {
			g.logf("AUTO-FIRE  this ship can't think yet... salvage it a brain")
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF6) && g.sfx != nil {
		g.sfx.muted = !g.sfx.muted
		if g.sfx.muted {
			g.sfx.stopLoops()
			g.sfx.stopMusic()
		}
		g.sfx.saveConfig() // the choice survives the session
		g.logf("AUDIO  %s", mutedWord(g.sfx.muted))
	}
	if g.mapOpen {
		if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
			g.zoomMap(mapZoomStep)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
			g.zoomMap(1 / mapZoomStep)
		}
	}
}

func (g *Game) Update() error {
	g.sfx.update()    // tick sound throttles + prune finished players (nil-safe)
	g.laserOn = false // re-asserted each frame the beam is actually held

	g.stepForcedGC() // web only: run finalizers so dead map/arena textures actually free
	g.updateTouch()  // track thumbs first, so every branch below sees fresh touch state

	// Skirmish reads no input at all: every key — and the system cursor itself —
	// belongs to whatever the user is doing under the overlay. Ships answer to
	// their programs, the program answers to its floating panel, never to the
	// keyboard. (Esc is gated below for the same reason.)
	if !g.skirmishMode {
		g.updateCursor()
		g.updateHotkeys()
	}

	// The credits attract screen: it owns Esc (exit) and the Konami code, then the
	// autonomous sim runs below (the input branch flies it on autopilot).
	switch {
	case g.skirmishMode:
		g.stepSkirmishMeta() // only the periodic regen: the overlay window has no player at the keys
	case g.creditsMode:
		if g.titleMode && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.sfx.saveConfig()
			return ebiten.Termination // Esc quits from the title screen
		}
		if g.stepCreditsMeta() {
			return nil // Esc dropped back to a fresh game
		}
	}

	// While paused, the ESC menu owns input and gameplay is frozen.
	if g.paused {
		g.sfx.stopLoops() // the engine/beam must not hum through the menu
		return g.updatePauseMenu()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) && !g.skirmishMode {
		g.paused = true
		return nil
	}

	// When the player is dead, only the restart key is live.
	if g.over {
		g.sfx.stopLoops()
		g.sfx.stopMusic() // silence reads as defeat
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			g.enterCreditsReturning() // roll the credits; Esc will come back to this game-over screen
			return nil
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyR) || touchJustTapped() {
			if g.checkpoint.valid && !shiftHeld() {
				g.restoreCheckpoint() // R (or a tap): back to the last checkpoint with the run intact
			} else {
				mapDir, mapName, simple := g.mapDir, g.mapName, g.simpleMap
				sfx := g.sfx // the audio context is a process singleton: carry it over
				g.releaseTransientImages()
				*g = *newWithContent(g.content, g.player, g.level, mapDir, simple)
				g.mapName, g.sfx = mapName, sfx // Shift+R (or no checkpoint): from scratch
			}
			return nil
		}
		// Left idle, the game-over screen returns to attract on its own — an arcade cabinet never
		// sits dead, it loops back to the demo to draw the next player in.
		g.overIdle++
		if g.overIdle >= attractIdleFrames {
			g.enterTitle()
		}
		return nil
	}

	// Campaign won: the victory screen is terminal and owns input.
	if g.gameWon {
		return g.updateVictory()
	}

	// A death or win is queued: play the aftermath (effects only, ship frozen) before its screen.
	if g.endKind != endNone {
		return g.stepEnding()
	}

	if !g.creditsMode {
		g.runTicks++   // the run clock: counts every frame of active play (not paused/over/won/credits)
		g.readCheats() // iddqd / idkfa: typed during play, not in the attract demo
	}

	if g.invuln > 0 {
		g.invuln--
	}
	if g.clearBannerTicks > 0 {
		g.clearBannerTicks-- // the level-clear note fades on its own; play never stops for it
	}
	if g.titleBannerTicks > 0 {
		g.titleBannerTicks-- // the stage-title flash fades too
	}
	g.stepShieldMods()

	// Pick the auto-fire target first, so manual aimed shots this frame point at
	// it too. In skirmish the player ship does not exist: nothing to fly, no
	// weapons to auto-fire — every hull in the arena is an entity, on a faction,
	// hunting the other factions (see enemyTarget).
	thrusting := false
	g.boosting = false // the demo never boosts; readManualInput re-sets this each frame
	switch {
	case g.skirmishMode:
		// the ghost stays parked: not flown, not drawn, not a target
	case g.creditsMode && !g.creditsPlayable:
		g.updateAutoTarget()
		thrusting = g.autoPilot()
		g.runAutoFire() // the combat computer auto-fires the aimed mounts (G toggles)
	default:
		g.updateAutoTarget()
		thrusting = g.readManualInput()
		g.runAutoFire()
	}

	// Every fire input has run, so laserOn is settled for this frame.
	g.stepLaserHeat()
	g.updateLoops(thrusting)
	g.updateMusic() // combat themes take over; the stage theme comes back after

	g.vx *= friction
	g.vy *= friction
	g.applyDevourerPull() // the black hole drags the ship in; full-burn flee to hold position
	g.clampSpeed()
	g.tryMove(g.vx, g.vy)
	if g.resolvePortals() {
		return nil // the world was swapped to another map; skip the rest this frame
	}
	if g.streamProcedural() {
		return nil // crossed a chunk boundary: the procedural world was rebuilt
	}
	if side, ok := g.diggingPastEdge(); ok {
		g.advancePastEdge(side) // dug to the map edge: warp into the next procedural screen
		return nil
	}
	g.updateDiscovery()
	g.stepHorde()
	g.energyPool().Tick() // the slow trickle back; a pickup is what refills it properly
	g.stepArrivals()      // vortices land the ships they were opened for (skirmish)
	g.stepProjectiles()
	g.stepAllies()
	g.updateEnemies()
	g.stepMines()
	g.stepDevourer()
	g.resolveContacts()
	g.resolvePickups()
	g.resolveWeaponPickups()
	g.stepEnemyShots()
	g.stepParticles()
	g.stepShockwaves()
	g.stepFloaters()
	g.stepLog()
	g.decayShake()
	g.updateObjectives()
	if g.updateResolutions() {
		return nil // a resolution warped the world; stop using stale state
	}
	g.checkLevelClear() // objectives all met -> freeze on the results overlay
	return nil
}

// readManualInput reads the keyboard/mouse and drives the ship: turn, thrust,
// strafe, brake, and fire each mount by its bound input. Returns whether the ship
// thrust this frame (for the engine loop). Split out so the credits demo can swap it
// for autoPilot.
func (g *Game) readManualInput() bool {
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		g.angle -= turnSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		g.angle += turnSpeed
	}
	thrusting := false
	fx, fy, rx, ry := g.headingDirs()
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		g.vx += fx * thrust
		g.vy += fy * thrust
		g.emitThruster(fx, fy)
		thrusting = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) {
		g.vx -= fx * reverseThrust
		g.vy -= fy * reverseThrust
		thrusting = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyQ) { // strafe left
		g.vx -= rx * strafeThrust
		g.vy -= ry * strafeThrust
		thrusting = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyE) { // strafe right
		g.vx += rx * strafeThrust
		g.vy += ry * strafeThrust
		thrusting = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.vx *= 0.90
		g.vy *= 0.90
	}
	if g.updateBoost(fx, fy) { // Left Shift: afterburner (extra forward thrust + higher speed cap)
		thrusting = true
	}
	// The mouse aims and fires the TURRET — the cursor-aimed mount (slot index 1, shown as
	// "2"): LEFT mouse fires it, the right button is unused. The forward NOSE gun (slot
	// index 0, "1") fires with Space. Keys 1 and 2 cycle each mount through the collected
	// arsenal (arsenal.go). The slot decides aim (forward / cursor); the weapon decides cadence.
	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		g.cycleSlot(0)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		g.cycleSlot(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.toggleAimMode()
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !g.fireBlocked {
			g.fireSlotInput(1, inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)) // left mouse fires the aimed turret
		}
	} else {
		g.fireBlocked = false // released: the next press may fire again
	}
	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.fireSlotInput(0, inpututil.IsKeyJustPressed(ebiten.KeySpace)) // Space fires the forward nose gun
	}
	if g.applyTouchControls() { // thumbs stack with the keys; a no-op with no touch
		thrusting = true
	}
	return thrusting
}

// fireSlotInput fires slot s from its held button. The DEVOURER is a discrete deploy — one
// black hole per PRESS (justPressed) — so a held button never spams "no charge" nor auto-burns
// a charge after each collapse; every other weapon keeps firing while the button is held.
func (g *Game) fireSlotInput(s int, justPressed bool) {
	if g.slots[s].filled && g.slots[s].w.Kind == weapon.KindDevourer {
		if justPressed {
			g.fireWeaponSlot(s)
		}
		return
	}
	g.fireWeaponSlot(s)
}

// hurtPlayer applies damage; on reaching zero health it costs a life and either
// ends the game or respawns the ship with brief invulnerability.
func (g *Game) hurtPlayer(dmg int) {
	if g.creditsMode || g.godMode {
		return // the attract-mode ship, or an iddqd ship, is indestructible
	}
	if g.invuln > 0 || g.over || g.endKind != endNone {
		return // dead already, or the death is already playing out
	}
	if g.bubbleTime > 0 {
		g.emitBurst(g.x, g.y, shieldSparks) // the bubble absorbs it whole
		return
	}
	g.addShake(hitShake)
	g.playEvent(g.player, "hit")
	// The shield absorbs damage first; only the overflow reaches health.
	if g.shield > 0 {
		g.emitBurst(g.x, g.y, shieldSparks)
		if g.shield >= dmg {
			g.shield -= dmg
			return
		}
		dmg -= g.shield
		g.shield = 0
	}
	g.health -= dmg
	if g.health > 0 {
		return
	}
	g.lives--
	if g.lives <= 0 {
		g.health = 0
		g.beginEnding(endDeath) // queue GAME OVER behind the death's aftermath
		return
	}
	g.health = maxHealth
	g.respawn()
}

// respawn recovers the ship after it loses a life WITHOUT teleporting it — losing a life is a
// setback, not a checkpoint reset. It stops the ship where it was hit, refills health, flashes a
// spark burst and grants brief invulnerability so it is not immediately hit again while it blinks
// back. (A game over still returns to the checkpoint via restoreCheckpoint.)
func (g *Game) respawn() {
	g.vx, g.vy = 0, 0
	g.invuln = respawnInvuln
	g.emitBurst(g.x, g.y, shieldSparks)
	g.addShake(hitShake)
}

// headingDirs returns the ship's forward and right unit vectors in world space.
// In this y-down space, right is the forward vector turned +90°.
func (g *Game) headingDirs() (fx, fy, rx, ry float64) {
	rad := g.angle * math.Pi / 180
	fx, fy = math.Cos(rad), math.Sin(rad)
	rx, ry = -fy, fx
	return
}

// updateCursor hides the OS cursor during active play (the crosshair is the
// pointer) and restores it when a menu or end screen needs clicking.
func (g *Game) updateCursor() {
	if g.paused || g.over {
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
		return
	}
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
}

// clampSpeed caps the velocity magnitude to keep collision reliable.
func (g *Game) clampSpeed() {
	lim := maxSpeed
	if g.boosting {
		lim = maxSpeed * boostSpeedMul // the afterburner tops out higher
	}
	speed := math.Hypot(g.vx, g.vy)
	if speed > lim {
		s := lim / speed
		g.vx *= s
		g.vy *= s
	}
}

// tryMove applies the move one axis at a time so the ship slides along walls
// instead of sticking, and never crosses one.
func (g *Game) tryMove(dx, dy float64) {
	steps := 1
	if d := math.Hypot(dx, dy); d > maxSpeed {
		steps = int(math.Ceil(d / maxSpeed)) // the afterburner can move > maxSpeed/frame; sub-step so it cannot tunnel
	}
	sx, sy := dx/float64(steps), dy/float64(steps)
	for range steps {
		if sx != 0 {
			if !g.collides(g.x+sx, g.y) {
				g.x += sx
			} else {
				g.wallImpact(g.vx)
				g.vx, sx = 0, 0
			}
		}
		if sy != 0 {
			if !g.collides(g.x, g.y+sy) {
				g.y += sy
			} else {
				g.wallImpact(g.vy)
				g.vy, sy = 0, 0
			}
		}
	}
}

// wallImpact damages the player for a hard collision with a wall, scaled by the
// impact speed; gentle contact and sliding do nothing.
func (g *Game) wallImpact(v float64) {
	speed := math.Abs(v)
	if speed < wallDamageMinSpeed {
		return
	}
	dmg := int((speed - wallDamageMinSpeed) * wallDamageScale)
	if dmg > 0 {
		g.hurtPlayer(dmg)
	}
}

// Draw renders the world around the centered, upward-pointing ship. Because the
// world swings by a large per-frame angle during a turn, a single still frame on
// a sample-and-hold display reads as a doubled line (stroboscopic stepping). To
// fill the gap, the moving world is accumulated over several intermediate camera
// poses between the previous and current frame and averaged. The ship itself is
// fixed at the screen center, so it is drawn once, crisp, on top.
func (g *Game) Draw(screen *ebiten.Image) {
	g.refreshFlood() // SPIKE: a dig this frame moves the rock face and its glow
	g.updateShakeOffset()

	// The ship blinks while invulnerable (just took a life hit) as damage feedback; its
	// glow blinks with it. Once it is DESTROYED (the game-over aftermath is playing) it is
	// gone entirely — only its explosion remains. The visibility decision is made once here.
	// In skirmish the player ship does not exist — the arena is entities only.
	showPlayer := !g.skirmishMode && g.endKind != endDeath && (g.invuln <= 0 || (ebiten.Tick()/4)%2 == 0)

	screen.Clear()
	// With an arena camera the ship is part of the world — it moves and turns on
	// screen like everything else, so it goes in with the world layer.
	g.accumulateWorld(screen, showPlayer && g.arenaCam)
	// Fog of war: a gray layer over everything, cleared where the brush has passed.
	g.drawBrushFog(screen)

	// The ship: at the screen center, always pointing up, so it never strobes — it
	// is drawn once, crisp, with its own bloom (never reprojected with the world).
	if showPlayer && !g.arenaCam {
		g.pglow.Bloom(screen, image.Rect(0, 0, g.sw, g.sh), render.GlowOptions{
			Variant: render.GlowStable, Intensity: 1.2, Spread: 2.5 * g.dpr, Iterations: 2, AntiAlias: true,
		}, func(emissive *ebiten.Image) {
			if g.playerGlow != nil {
				g.playerGlow.Draw(emissive, g.playerGeoM(), true)
			}
		})
		g.playerMesh.Draw(screen, g.playerGeoM(), true)
		g.drawShieldRing(screen)
		g.drawTimedShields(screen)
	}

	// The credits screen dims the running demo and scrolls its text over it, in
	// place of the gameplay HUD.
	if g.creditsMode && !g.skirmishMode {
		g.drawCredits(screen)
		g.prevX, g.prevY, g.prevAngle = g.x, g.y, g.angle
		return
	}

	// The crosshair follows the mouse, and in skirmish the mouse belongs to
	// whatever the user is actually doing under the overlay.
	if !g.paused && !g.over && !g.mapOpen && !g.skirmishMode {
		g.drawCrosshair(screen)
	}

	// Round "porthole" mask: black out the corners so the view reads as a circle.
	// Drawn over the world but under the HUD, which stays in the black frame.
	if g.roundView {
		g.drawRoundMask(screen)
	}

	g.drawHUD(screen)
	g.drawTouchSticks(screen) // live thumbs + pause spot; invisible until a finger has landed

	if g.mapOpen {
		g.presentOverlay(screen, g.drawMapScreen)
	}
	if g.paused {
		g.drawPauseMenu(screen)
	}

	g.prevX, g.prevY, g.prevAngle = g.x, g.y, g.angle
}

// accumulateWorld draws the motion-blurred world into dst. The world is 2D, so a
// camera pose is a plain affine transform: instead of RE-RENDERING the scene at
// each intermediate pose (the old accumulation, N full renders + N blooms — the
// cost that spiked on a big screen while turning), it renders the world ONCE at the
// current pose into a padded frame and accumulates N exact affine REPROJECTIONS of
// that frame. One render + N cheap textured blits, so the cost barely grows with
// the turn rate. The reprojection is exact for 2D content up to resampling and the
// frame padding (sized so a turn never uncovers a corner). The player is not in the
// frame (it stays fixed at the screen center); Draw composites it separately.
func (g *Game) accumulateWorld(dst *ebiten.Image, showPlayer bool) {
	samples := g.blurSamples()
	m := g.frameMargin()
	camX, camY, camAngle := g.camPose()
	if samples == 1 {
		// Still or slow: one sample is one exact reprojection of the current pose —
		// identical to rendering straight to the screen. Skip the padded frame and
		// the fullscreen blit entirely (menus, the title's idle attract, hovering).
		// The bloom region keeps the PADDED size in both branches so the glow's
		// offscreen buffers never reallocate on a still<->moving transition.
		g.drawWorld(dst, image.Rect(0, 0, g.sw+2*m, g.sh+2*m), camX, camY, camAngle, showPlayer)
		return
	}

	g.ensureFrame()
	pad := float64(m)
	fw, fh := g.frame.Bounds().Dx(), g.frame.Bounds().Dy()

	// One render of the world at the current pose, centered in the padded frame.
	g.camPad = pad
	g.drawWorld(g.frame, image.Rect(0, 0, fw, fh), camX, camY, camAngle, showPlayer)
	g.camPad = 0

	weight := float32(1) / float32(samples)
	camFinal := g.cameraGeoM() // the pose the frame was rendered at (no pad)
	inv := camFinal
	inv.Invert()

	for s := range samples {
		t := (float64(s) + 0.5) / float64(samples)
		camS := g.cameraGeoMAt(lerp(g.prevX, g.x, t), lerp(g.prevY, g.y, t), lerp(g.prevAngle, g.angle, t))

		// frame pixel -> undo pad -> world (camFinal^-1) -> screen at pose s (camS).
		reproj := ebiten.GeoM{}
		reproj.Translate(-pad, -pad)
		reproj.Concat(inv)
		reproj.Concat(camS)

		op := &ebiten.DrawImageOptions{Blend: ebiten.BlendLighter, Filter: ebiten.FilterLinear}
		op.GeoM = reproj
		op.ColorScale.Scale(weight, weight, weight, weight)
		dst.DrawImage(g.frame, op)
	}
}

// drawWorld renders one opaque frame of the moving world for a single camera
// pose. Flood mode draws the negative-space map (glow fills everything, the
// navigable region is carved crisply to black); classic mode draws line walls.
func (g *Game) drawWorld(dst *ebiten.Image, region image.Rectangle, camX, camY, camAngle float64, showPlayer bool) {
	cam := g.cameraGeoMAt(camX, camY, camAngle)

	if g.transparent {
		// The desktop is the floor: clear to alpha and draw only lines and light.
		dst.Clear()
		g.bloomGlow(dst, region, func(emissive *ebiten.Image) {
			g.wallGlow.Draw(emissive, cam, true)
			g.drawEntityGlow(emissive, cam, camX, camY, camAngle, showPlayer)
		})
		g.wallMesh.Draw(dst, cam, true)
		g.drawGoalZones(dst, cam)
		g.drawEntityLayer(dst, cam, camX, camY, camAngle, showPlayer)
		return
	}

	if g.floodView {
		dst.Fill(floodFillColor) // intermediate base so the exterior never reaches black
		if g.flood != nil {
			g.drawFloodGlow(dst, cam) // glow fading from the silhouette down to the base
		}
		g.carveNavigable(dst, cam) // crisp black over the reachable corridors
		g.drawDug(dst, cam)        // SPIKE: rock the player blasted reads as open space
		// SPIKE: the wall LINES are omitted here — one would be drawn straight across
		// the mouth of a tunnel you just dug. The silhouette (black meeting glow)
		// defines the rock face on its own: the cave look.
	} else {
		dst.Fill(colorBg)
		// Glow behind the crisp strokes so the halo does not thicken the wall line.
		g.bloomGlow(dst, region, func(emissive *ebiten.Image) {
			g.wallGlow.Draw(emissive, cam, true)
			g.drawEntityGlow(emissive, cam, camX, camY, camAngle, showPlayer)
		})
		g.wallMesh.Draw(dst, cam, true)
		g.drawGoalZones(dst, cam)
		g.drawEntityLayer(dst, cam, camX, camY, camAngle, showPlayer)
		return
	}

	// Flood mode: entity glow + entities drawn after the carve so they sit on top
	// of the black corridors (the walls themselves stay crisp, without bloom).
	g.bloomGlow(dst, region, func(emissive *ebiten.Image) {
		g.drawEntityGlow(emissive, cam, camX, camY, camAngle, showPlayer)
	})
	g.drawGoalZones(dst, cam)
	g.drawEntityLayer(dst, cam, camX, camY, camAngle, showPlayer)
}

// drawGoalZones draws a pulsing outline at each "reach" objective's zone, so the
// player can see where the exit is. Hidden until the fog has revealed it; turns
// green once the objective is met.
func (g *Game) drawGoalZones(dst *ebiten.Image, cam ebiten.GeoM) {
	pulse := 0.5 + 0.5*math.Sin(float64(ebiten.Tick())/float64(ebiten.TPS())*3)
	for i := range g.objectives {
		o := g.objectives[i]
		if o.kind != objReach {
			continue
		}
		z, ok := g.zoneByName(o.zone)
		if !ok {
			continue
		}
		cx, cy := zoneCenter(z)
		if g.fogHidden(cx, cy) {
			continue
		}
		col := goalColor
		if o.done {
			col = goalDoneColor
		}
		col.A = uint8(0x50 + 0xa0*pulse)
		strokeZoneWorld(dst, cam, z, g.camPixelScale(), col)
	}
	g.drawCheckpointZones(dst, cam)
}

// strokeZoneWorld outlines a zone through the camera transform (a circle stays a
// circle; a rect is drawn as four transformed edges so it rotates with the world).
func strokeZoneWorld(dst *ebiten.Image, cam ebiten.GeoM, z level.Zone, scale float64, col color.RGBA) {
	switch z.Kind {
	case level.ZoneCircle:
		if len(z.Points) < 1 {
			return
		}
		sx, sy := cam.Apply(z.Points[0].X, z.Points[0].Y)
		vector.StrokeCircle(dst, float32(sx), float32(sy), float32(z.Radius*scale), 2, col, true)
	case level.ZoneRect:
		if len(z.Points) < 2 {
			return
		}
		corners := [4][2]float64{
			{z.Points[0].X, z.Points[0].Y}, {z.Points[1].X, z.Points[0].Y},
			{z.Points[1].X, z.Points[1].Y}, {z.Points[0].X, z.Points[1].Y},
		}
		for i := range 4 {
			ax, ay := cam.Apply(corners[i][0], corners[i][1])
			bx, by := cam.Apply(corners[(i+1)%4][0], corners[(i+1)%4][1])
			vector.StrokeLine(dst, float32(ax), float32(ay), float32(bx), float32(by), 2, col, true)
		}
	}
}

// bloomGlow runs the shared bloom pass: fill draws the emissive sources, which are
// blurred and added behind the crisp geometry.
func (g *Game) bloomGlow(dst *ebiten.Image, region image.Rectangle, fill func(*ebiten.Image)) {
	g.glow.Bloom(dst, region, render.GlowOptions{
		Variant:    render.GlowStable,
		Time:       float64(ebiten.Tick()) / float64(ebiten.TPS()),
		Intensity:  1.2,
		Spread:     2.5 * g.dpr,
		Iterations: 2,
		AntiAlias:  true,
	}, fill)
}

// fogHidden reports whether a world point lies under the fog of war (an area the
// ship's brush has not cleared yet), so entities there can be culled.
func (g *Game) fogHidden(wx, wy float64) bool {
	return g.disc != nil && !g.disc.discoveredAt(wx, wy)
}

// drawEntityGlow draws the emissive sources for enemies, the player, bullets and
// effects into the bloom buffer.
func (g *Game) drawEntityGlow(emissive *ebiten.Image, cam ebiten.GeoM, camX, camY, camAngle float64, showPlayer bool) {
	for i := range g.entities {
		e := &g.entities[i]
		if g.fogHidden(e.x, e.y) {
			continue // hidden in the fog: only visible where the brush has cleared
		}
		if e.kind == kindPortal {
			continue // the portal is drawn as ring + particles only; no asset glow circle
		}
		if e.glowMesh != nil && !e.glowMesh.Empty() {
			e.glowMesh.Draw(emissive, g.entityGeoMAt(e, camX, camY, camAngle), true)
		}
	}
	if showPlayer && g.playerGlow != nil && !g.playerGlow.Empty() {
		g.playerGlow.Draw(emissive, g.playerWorldGeoM(camX, camY, camAngle), true)
	}
	g.drawAllyGlow(emissive, camX, camY, camAngle)
	g.drawLaser(emissive, cam, true)
	g.drawBolts(emissive, cam, g.projectiles, true)
	g.drawBolts(emissive, cam, g.enemyShots, true)
	g.drawMines(emissive, cam, true)
	g.drawParticles(emissive, cam, true)
	g.drawMuzzleFlash(emissive, cam, muzzleColor, 9)
}

// drawEntityLayer draws the crisp foreground: enemies, bullets and effects.
func (g *Game) drawEntityLayer(dst *ebiten.Image, cam ebiten.GeoM, camX, camY, camAngle float64, showPlayer bool) {
	g.drawArrivals(dst, cam) // under everything: a ship comes OUT of its vortex
	// Emitted matter goes UNDER the hulls: exhaust and the muzzle flash pour out
	// at the emitting ship's own outline, and drawn on top they overpaint it —
	// under, the silhouette clips them and the plume reads as coming from behind
	// the ship. Bolts, beams and shockwaves stay above: those cross OTHER ships,
	// and a shot ducking under its target reads as a miss.
	g.drawParticles(dst, cam, false)
	g.drawMuzzleFlash(dst, cam, muzzleColor, 5)
	for i := range g.entities {
		if g.fogHidden(g.entities[i].x, g.entities[i].y) {
			continue // hidden in the fog: only visible where the brush has cleared
		}
		g.drawEntity(dst, cam, &g.entities[i], camX, camY, camAngle)
	}
	if showPlayer && g.playerMesh != nil && !g.playerMesh.Empty() {
		g.playerMesh.Draw(dst, g.playerWorldGeoM(camX, camY, camAngle), true)
	}
	g.drawAllies(dst, camX, camY, camAngle) // friendly companions, over the world
	g.drawLaser(dst, cam, false)
	g.drawBolts(dst, cam, g.projectiles, false)
	g.drawBolts(dst, cam, g.enemyShots, false)
	g.drawMines(dst, cam, false)
	g.drawDevourer(dst, cam)
	g.drawShockwaves(dst, cam)
	g.drawFloaters(dst, cam)
}

// blurSamples sizes the accumulation so the fastest-moving on-screen point steps
// at most blurStepPx per sub-frame. Peripheral travel during a turn is angle
// times the half-diagonal radius; translation adds its straight-line travel. The
// gap that reads as strobing is a PHYSICAL one, so the target step scales with the
// device pixel ratio: a high-density (Retina) display was paying twice the
// sub-frames for a gap the eye cannot resolve.
func (g *Game) blurSamples() int {
	if g.arenaCam {
		return 1 // the camera does not move, so there is no camera motion to smear
	}
	radius := 0.5 * math.Hypot(float64(g.sw), float64(g.sh))
	turn := math.Abs(g.angle-g.prevAngle) * math.Pi / 180
	travel := radius*turn + g.camPixelScale()*math.Hypot(g.x-g.prevX, g.y-g.prevY)

	step := blurStepPx * g.dpr
	if step <= 0 {
		step = blurStepPx
	}
	samples := int(math.Ceil(travel / step))
	if samples < 1 {
		return 1
	}
	if samples > blurSampleCap { // the platform profile's budget (native: maxBlurSamples)
		return blurSampleCap
	}
	return samples
}

// gcTick counts frames toward the platform profile's forced-GC cadence. Package level
// (not on Game) so world resets never restart the countdown.
var gcTick int

// webDebug forces the debug overlay on everywhere (play.html?debug), attract included —
// the on-device way to watch FPS and memory without a keyboard. Package level so world
// resets never drop it.
var webDebug bool

// stepForcedGC runs the Go GC on the platform profile's cadence (web only; a no-op
// natively). See perf_js.go for why: ebiten frees GPU textures via finalizers, and a
// small stable heap means automatic GC — and therefore the freeing — rarely happens.
func (g *Game) stepForcedGC() {
	if forceGCEvery <= 0 {
		return
	}
	gcTick++
	if gcTick >= forceGCEvery {
		gcTick = 0
		runtime.GC()
	}
}

// capScale clamps the OS device scale factor to the platform render profile: uncapped
// on native, 1× on the web (see perf_js.go). Pure, so the clamp is testable.
func capScale(scale float64) float64 {
	if scale <= 0 {
		scale = 1
	}
	if renderScaleCap > 0 && scale > renderScaleCap {
		return renderScaleCap
	}
	return scale
}

// frameMargin is the device-pixel padding around the screen in the frame buffer:
// the farthest on-screen point (a corner, at the half-diagonal) sweeps this far
// when the world turns one frame's worth, so a reprojected sub-frame never reveals
// an unrendered edge. Sized to the worst case (max turn + max translation).
func (g *Game) frameMargin() int {
	radius := 0.5 * math.Hypot(float64(g.sw), float64(g.sh))
	m := radius*turnSpeed*math.Pi/180 + maxSpeed*g.camPixelScale() + 8
	return int(math.Ceil(m))
}

// releaseTransientImages hands the old world's GPU buffers back BEFORE a world
// reset drops the struct. Ebiten frees an image's texture via a FINALIZER, so
// without this every portal, restart and attract swap leaves several fullscreen
// buffers waiting on some future GC — the memory ratchet behind the iOS tab
// kills. No content is lost: everything here is redrawn from scratch (fogTex is
// re-hydrated from the discovery grid on the next map entry). Idempotent, so
// every reset route can call it without coordination.
func (g *Game) releaseTransientImages() {
	for _, img := range []*ebiten.Image{g.frame, g.fogMask, g.fogTex, g.overlay} {
		if img != nil {
			img.Deallocate()
		}
	}
	g.frame, g.fogMask, g.fogTex, g.overlay = nil, nil, nil, nil
	g.fogStamped = false // the cleared texture is gone; the next world must stamp anew
	g.glow.Deallocate()
	g.pglow.Deallocate()
	g.flood.releaseImages()
}

// ensureFrame (re)allocates the padded frame buffer to match the device resolution
// plus the reprojection margin.
func (g *Game) ensureFrame() {
	if g.sw <= 0 || g.sh <= 0 {
		return
	}
	m := g.frameMargin()
	fw, fh := g.sw+2*m, g.sh+2*m
	if g.frame != nil {
		b := g.frame.Bounds()
		if b.Dx() == fw && b.Dy() == fh {
			return
		}
	}
	g.frame = ebiten.NewImage(fw, fh)
}

// drawShieldRing draws a cyan ring around the centered ship while it has shield,
// brighter the more shield is left.
func (g *Game) drawShieldRing(screen *ebiten.Image) {
	if g.shield <= 0 {
		return
	}
	w, h := g.screenSize()
	cx, cy := w/2+g.shakeX, h/2+g.shakeY
	r := (g.radius + 8) * g.camPixelScale()
	frac := float64(g.shield) / float64(maxShield)
	col := color.RGBA{0x60, 0xd0, 0xff, uint8(0x50 + 0x90*frac)}
	vector.StrokeCircle(screen, float32(cx), float32(cy), float32(r), float32(2*g.dpr), col, true)
}

// drawBossShield strokes a pulsing double ring around a boss that is still protected by its
// escorts — a clear "you can't hurt this yet" cue that vanishes the instant the room is cleared.
func (g *Game) drawBossShield(dst *ebiten.Image, cam ebiten.GeoM, e *entity) {
	mx, my := cam.Apply(e.x, e.y)
	pulse := 0.5 + 0.5*math.Sin(float64(g.runTicks)*0.12)
	base := (e.radius + 10) * g.camPixelScale()
	col := color.RGBA{0x70, 0xd8, 0xff, uint8(0x60 + 0x80*pulse)} // shield cyan, breathing
	vector.StrokeCircle(dst, float32(mx), float32(my), float32(base), float32(2.5*g.dpr), col, true)
	outer := color.RGBA{0x70, 0xd8, 0xff, uint8(0x28 + 0x40*pulse)}
	vector.StrokeCircle(dst, float32(mx), float32(my), float32(base+4*g.dpr*pulse), float32(1.5*g.dpr), outer, true)
}

// drawEntity renders one runtime entity through the given camera pose, plus a
// brief white ring while it is flashing from a hit.
func (g *Game) drawEntity(dst *ebiten.Image, cam ebiten.GeoM, e *entity, camX, camY, camAngle float64) {
	if e.kind == kindPortal {
		g.drawPortal(dst, cam, e) // a portal is ONLY its ring + particles: no asset mesh, no hit ring
		return
	}
	if e.mesh != nil && !e.mesh.Empty() {
		e.mesh.Draw(dst, g.entityGeoMAt(e, camX, camY, camAngle), true)
	} else {
		col := colorEnemy
		if e.kind == kindPowerUp {
			col = colorPower
		}
		mx, my := cam.Apply(e.x, e.y)
		vector.StrokeCircle(dst, float32(mx), float32(my), float32(6*g.dpr), float32(1.5*g.dpr), col, true)
	}

	if e.hitFlash > 0 {
		frac := float64(e.hitFlash) / float64(hitFlashFrames)
		mx, my := cam.Apply(e.x, e.y)
		c := color.RGBA{0xff, 0xff, 0xff, uint8(255 * frac)}
		vector.StrokeCircle(dst, float32(mx), float32(my), float32(e.radius*g.camPixelScale()), float32(2*g.dpr), c, true)
	}

	if e.kind == kindEnemy && e.boss && g.escortsAlive() {
		g.drawBossShield(dst, cam, e) // the shielded boss reads as untouchable until the room is cleared
	}

	switch e.kind {
	case kindPowerUp:
		g.drawPulseRing(dst, cam, e, powerupColor(e.power))
	case kindWeapon:
		col := colorPower
		if cat := catForKey(e.power); cat >= 0 {
			col = weapon.Catalog[cat].Col // the ring says which weapon lies there
		}
		g.drawPulseRing(dst, cam, e, col)
	}
}

// drawPortal renders a portal with the shared vortex: a thin outer ring and motes
// falling straight down it to the centre. The accent colour comes from
// portalStyle, so an authored-stage portal and a bonus/procedural one are told
// apart.
func (g *Game) drawPortal(dst *ebiten.Image, cam ebiten.GeoM, e *entity) {
	col, _ := portalStyle(e.target)
	mx, my := cam.Apply(e.x, e.y)
	sc := g.camPixelScale()
	effects.DrawPortal(dst, effects.Portal{
		X: mx, Y: my,
		Radius:  (e.radius + 12) * sc,
		Col:     col,
		Seconds: float64(ebiten.Tick()) / float64(ebiten.TPS()),
		DPR:     g.dpr,
		Scale:   sc,
	})
}

// drawPulseRing draws the shared pulsing ring around an entity, in the given
// accent colour. Used for power-ups and portals.
func (g *Game) drawPulseRing(dst *ebiten.Image, cam ebiten.GeoM, e *entity, col color.RGBA) {
	mx, my := cam.Apply(e.x, e.y)
	sc := g.camPixelScale()
	effects.DrawPulseRing(dst, mx, my, e.radius*sc, col,
		float64(ebiten.Tick())/float64(ebiten.TPS()), sc, g.dpr)
}

// powerupColor is the accent color for a power-up effect.
func powerupColor(power string) color.RGBA {
	switch power {
	case powerShield:
		return color.RGBA{0x60, 0xd0, 0xff, 0xff}
	case powerScore:
		return color.RGBA{0xff, 0xe0, 0x60, 0xff}
	case powerComputer:
		return color.RGBA{0xc0, 0x80, 0xff, 0xff} // the combat computer's violet chip
	case powerFire:
		return color.RGBA{0xff, 0x80, 0x40, 0xff} // fire-power orange
	case powerRate:
		return color.RGBA{0x60, 0xff, 0xf0, 0xff} // fire-rate cyan
	case powerDamage:
		return color.RGBA{0xff, 0x50, 0x50, 0xff} // damage red
	case powerAlly:
		return color.RGBA{0x55, 0xa0, 0xff, 0xff} // ally blue (matches the map marker)
	case powerDrone:
		return color.RGBA{0x80, 0xc0, 0xff, 0xff} // drone light-blue
	case powerBubble:
		return color.RGBA{0x80, 0xf0, 0xff, 0xff} // bubble cyan-white
	case powerReflect:
		return color.RGBA{0xc0, 0x80, 0xff, 0xff} // reflector violet
	case powerSeek:
		return color.RGBA{0xff, 0xb0, 0x40, 0xff} // homing amber
	default: // heal
		return color.RGBA{0x60, 0xff, 0x90, 0xff}
	}
}

// lerp linearly interpolates from a to b by t in [0,1].
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}
