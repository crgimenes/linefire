package game

import (
	"io/fs"
	"math"

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
//   - no crosshair: the mouse belongs to whatever the user is actually doing;
//   - silent unless asked: a desktop toy does not talk first.
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
