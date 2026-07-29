package game

import (
	"io/fs"

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

	g.enterCredits() // the autonomous demo: autopilot, endless horde, indestructible ship
	g.skirmishMode = true
	g.transparent = true
	g.floodView = false
	g.debugHUD = opts.Debug
	return g, nil
}

// stepSkirmishMeta is skirmish's slice of stepCreditsMeta: only the periodic
// arena regeneration. No scroll, no Konami code, no Esc/R — the window is
// click-through, so there is no player to press them.
func (g *Game) stepSkirmishMeta() {
	g.creditsRegenCD--
	if g.creditsRegenCD <= 0 {
		g.buildCreditsArena() // a fresh cave to keep flying and fighting in
	}
}
