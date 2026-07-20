package game

import (
	"image/color"
	"maps"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"linefire/level"
)

// triggerCheckpoint is the reserved zone Trigger that saves a run snapshot. Passing
// through a checkpoint zone captures everything a game-over would otherwise wipe, so
// death (R) drops the player back here with their arsenal instead of an empty ship.
const triggerCheckpoint = "checkpoint"

var checkpointColor = color.RGBA{0x50, 0xff, 0xa0, 0xff} // green-cyan ring

// checkpoint is a snapshot of the run at a checkpoint zone: the map and a spot on
// it, plus the full loadout, upgrades and vitals. Slices are cloned on save so a
// later loadout or mod change never mutates a stored checkpoint. The level pointer
// is shared (it is immutable, editor-owned data).
type checkpoint struct {
	valid       bool
	level       *level.Level
	mapName     string
	mapDir      string
	simpleMap   bool
	x, y, angle float64

	health, shield, lives, score int
	arsenal                      []int
	slotArsIdx                   [numSlots]int
	hasComputer                  bool
	autoFire                     bool
	fireLevel                    int
	rateLevel                    int
	damageLevel                  int
	seekLevel                    int
	bubbleTime                   int
	reflectTime                  int
	allies                       []ally

	// World progress, so a restore is the COMPLETE game state, not just the run:
	// mapStates records which spawns are already consumed (pickups collected, enemies
	// killed) and which rock is dug out, per map, so restoring neither respawns them
	// nor re-fills the tunnels — the fix for powerups accumulating when you re-collect
	// them after a death. Horde ramp scalars resume the difficulty (a fresh rng is
	// fine; spawns are procedural anyway).
	mapStates       map[string]*mapState
	levelStartTicks int // so the per-level clear time survives a death/restore
	hasHorde        bool
	hordeElapsed    int
	hordeInterval   int
	hordeMaxAlive   int
	hordeCD         int
}

// cloneMapStates deep-copies the per-map WORLD progress so a checkpoint is frozen at
// save time — later play marks more spawns consumed, and digs more rock, without
// touching the snapshot. Copying dug is what un-digs a tunnel bored after the
// checkpoint.
//
// The fog is deliberately SHARED rather than frozen: what the player has seen is
// knowledge, not world state, so a death never re-seals a corridor they already flew
// down — the same rule the RTA clock follows.
func cloneMapStates(m map[string]*mapState) map[string]*mapState {
	if m == nil {
		return nil
	}
	out := make(map[string]*mapState, len(m))
	for name, s := range m {
		out[name] = &mapState{
			consumed: maps.Clone(s.consumed),
			reached:  maps.Clone(s.reached),
			resolved: maps.Clone(s.resolved),
			dug:      slices.Clone(s.dug),
			disc:     s.disc,
		}
	}
	return out
}

// saveCheckpoint snapshots the current run at a checkpoint zone's center. Called on
// each enter-edge, so re-passing a checkpoint refreshes it with the better state.
func (g *Game) saveCheckpoint(z level.Zone) {
	g.bindMapState() // the store must hold this map's tunnels before we freeze a copy
	cx, cy := zoneCenter(z)
	g.checkpoint = checkpoint{
		valid:     true,
		level:     g.level,
		mapName:   g.mapName,
		mapDir:    g.mapDir,
		simpleMap: g.simpleMap,
		x:         cx,
		y:         cy,
		angle:     g.angle,

		health:          g.health,
		shield:          g.shield,
		lives:           g.lives,
		score:           g.score,
		arsenal:         append([]int(nil), g.arsenal...),
		slotArsIdx:      g.slotArsIdx,
		hasComputer:     g.hasComputer,
		autoFire:        g.autoFire,
		fireLevel:       g.fireLevel,
		rateLevel:       g.rateLevel,
		damageLevel:     g.damageLevel,
		seekLevel:       g.seekLevel,
		bubbleTime:      g.bubbleTime,
		reflectTime:     g.reflectTime,
		allies:          append([]ally(nil), g.allies...),
		mapStates:       cloneMapStates(g.mapStates),
		levelStartTicks: g.levelStartTicks,
	}
	if g.horde != nil {
		g.checkpoint.hasHorde = true
		g.checkpoint.hordeElapsed = g.horde.elapsed
		g.checkpoint.hordeInterval = g.horde.interval
		g.checkpoint.hordeMaxAlive = g.horde.maxAlive
		g.checkpoint.hordeCD = g.horde.cd
	}
	g.logf("CHECKPOINT  reached")
	g.sfx.play(soundReq{"arrival_low", 400, 0.6}) // a low affirming tone: progress secured
}

// restoreCheckpoint rebuilds the run at the last checkpoint: the checkpoint's map
// fresh (enemies back for a clean replay), the ship at the checkpoint, and the saved
// arsenal, upgrades and vitals. Used by the game-over R when a checkpoint exists.
func (g *Game) restoreCheckpoint() {
	cp := g.checkpoint
	sfx := g.sfx           // the audio context is a process singleton: carry it over
	ticks := g.runTicks    // RTA speedrun clock: a death does not rewind it
	startMap := g.startMap // the campaign origin is not part of a checkpoint; it outlives one
	runLog := g.runLog     // the stages already summarized survive a death, like the RTA clock
	g.releaseTransientImages()
	*g = *newWithContent(g.content, g.player, cp.level, cp.mapDir, cp.simpleMap)
	g.sfx = sfx
	g.runTicks = ticks
	g.startMap = startMap
	g.runLog = runLog
	g.mapDir, g.mapName = cp.mapDir, cp.mapName
	g.checkpoint = cp // keep it for the next death

	// Restore the world progress so already-collected pickups and killed enemies stay
	// gone (no re-collecting a powerup for free), then apply it to the fresh map.
	g.mapStates = cloneMapStates(cp.mapStates)
	g.levelStartTicks = cp.levelStartTicks
	g.applyMapState()
	g.syncLevelCleared() // if the checkpoint's map was already cleared, don't re-fire results
	if g.horde != nil && cp.hasHorde {
		g.horde.elapsed = cp.hordeElapsed
		g.horde.interval = cp.hordeInterval
		g.horde.maxAlive = cp.hordeMaxAlive
		g.horde.cd = cp.hordeCD
	}

	g.health, g.shield, g.lives, g.score = cp.health, cp.shield, cp.lives, cp.score
	g.arsenal = append([]int(nil), cp.arsenal...)
	g.slotArsIdx = cp.slotArsIdx
	g.syncSlots()
	g.hasComputer, g.autoFire = cp.hasComputer, cp.autoFire
	g.fireLevel, g.rateLevel, g.damageLevel, g.seekLevel = cp.fireLevel, cp.rateLevel, cp.damageLevel, cp.seekLevel
	g.bubbleTime, g.reflectTime = cp.bubbleTime, cp.reflectTime
	g.allies = append([]ally(nil), cp.allies...)

	g.x, g.y, g.angle = cp.x, cp.y, cp.angle
	g.prevX, g.prevY, g.prevAngle = cp.x, cp.y, cp.angle
	g.snapAlliesToShip()
	g.invuln = respawnInvuln
	g.logf("RESTORE  checkpoint")
}

// shiftHeld reports whether either shift key is down, so game-over R can offer a
// full restart (Shift+R) alongside the checkpoint restore (R).
func shiftHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// drawCheckpointZones outlines each checkpoint zone; the saved one (this map) pulses
// brighter, so the player can see where a death would send them back to.
func (g *Game) drawCheckpointZones(dst *ebiten.Image, cam ebiten.GeoM) {
	pulse := 0.5 + 0.5*math.Sin(float64(ebiten.Tick())/float64(ebiten.TPS())*3)
	for _, z := range g.level.Zones {
		if z.Trigger != triggerCheckpoint {
			continue
		}
		cx, cy := zoneCenter(z)
		if g.fogHidden(cx, cy) {
			continue
		}
		col := checkpointColor
		active := g.checkpoint.valid && g.checkpoint.mapName == g.mapName &&
			g.checkpoint.x == cx && g.checkpoint.y == cy
		if active {
			col.A = uint8(0x80 + 0x7f*pulse) // the saved checkpoint glows
		} else {
			col.A = 0x60
		}
		strokeZoneWorld(dst, cam, z, g.camPixelScale(), col)
	}
}
