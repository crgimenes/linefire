package game

import (
	"strconv"
	"strings"

	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/procgen"
)

// procDir is where generated chunks are persisted (and reused on revisit). It is
// working-directory relative for now; a server will replace it later.
const procDir = "procmaps"

// enterProcedural builds and enters the procedural map for the seed encoded in the
// portal target ("@proc" or "@proc:<seed>"): it generates the 3x3 neighbourhood
// around chunk (0,0) and starts the player at its center.
func (g *Game) enterProcedural(target string) {
	w := procgen.NewWorld(procDir, procSeed(target))
	err := w.Ensure(0, 0)
	if err != nil {
		return // generation failed: leave the portal inert this frame
	}
	lvl, err := w.Assemble(0, 0)
	if err != nil {
		return
	}
	g.enterMap(lvl, procMapName, "", true)
	g.procWorld = w
	g.procCX, g.procCY = 0, 0
}

// procMapName is the reserved map name the runtime uses while in the procedural
// world (it never names a file).
const procMapName = "@proc"

// procSeed extracts the world seed from a portal target ("@proc" -> 1, "@proc:42"
// -> 42, malformed -> 1).
func procSeed(target string) int64 {
	_, after, ok := strings.Cut(target, ":")
	if !ok {
		return 1
	}
	n, err := strconv.ParseInt(after, 10, 64)
	if err != nil {
		return 1
	}
	return n
}

// streamProcedural keeps the loaded region centered on the player: when the ship
// crosses into a new chunk it regenerates the 3x3 neighbourhood around that chunk
// and rebuilds the world, keeping the player's position, velocity, loadout and run
// progress. It reports whether it rebuilt, so Update can stop using stale state.
//
// Limitation (v1): the rebuild re-instantiates spawns, so enemies in the still-
// loaded chunks respawn on a crossing and the fog resets. Per-chunk entity/fog
// persistence is the planned fix.
func (g *Game) streamProcedural() bool {
	if g.procWorld == nil {
		return false
	}
	cx, cy := procgen.ChunkOf(g.x, g.y)
	if cx == g.procCX && cy == g.procCY {
		return false
	}
	err := g.procWorld.Ensure(cx, cy)
	if err != nil {
		return false
	}
	lvl, err := g.procWorld.Assemble(cx, cy)
	if err != nil {
		return false
	}
	g.rebuildProcedural(lvl, cx, cy)
	return true
}

// rebuildProcedural swaps in a freshly assembled procedural level while carrying
// over the run (health/shield/lives/score), the loadout, the combat-computer state,
// the window/DPI layout and — crucially — the player's position and velocity, so a
// chunk crossing is seamless rather than a teleport.
func (g *Game) rebuildProcedural(lvl *level.Level, cx, cy int) {
	health, shield, lives, score := g.health, g.shield, g.lives, g.score
	dpr, sw, sh, winW, winH := g.dpr, g.sw, g.sh, g.winW, g.winH
	overlay := g.overlay
	arsenal, slotArsIdx := g.arsenal, g.slotArsIdx
	autoFire, hasComputer, fireLevel := g.autoFire, g.hasComputer, g.fireLevel
	rateLevel, damageLevel, seekLevel := g.rateLevel, g.damageLevel, g.seekLevel
	bubbleTime, reflectTime := g.bubbleTime, g.reflectTime
	godMode := g.godMode
	allies := g.allies
	cp := g.checkpoint
	runTicks := g.runTicks
	sfx := g.sfx
	mapDir := g.mapDir // the horde still spawns assets from the game's asset directory
	startMap := g.startMap
	runLog := g.runLog
	world := g.procWorld
	px, py, ang := g.x, g.y, g.angle
	vx, vy := g.vx, g.vy

	g.releaseTransientImages()
	*g = *newWithContent(g.content, g.player, lvl, mapDir, true)

	g.health, g.shield, g.lives, g.score = health, shield, lives, score
	g.dpr, g.sw, g.sh, g.winW, g.winH = dpr, sw, sh, winW, winH
	g.overlay = overlay
	g.arsenal, g.slotArsIdx = arsenal, slotArsIdx
	g.syncSlots()
	g.autoFire, g.hasComputer, g.fireLevel = autoFire, hasComputer, fireLevel
	g.rateLevel, g.damageLevel, g.seekLevel = rateLevel, damageLevel, seekLevel
	g.bubbleTime, g.reflectTime = bubbleTime, reflectTime
	g.godMode = godMode
	g.allies = allies
	g.checkpoint = cp
	g.runTicks = runTicks
	g.levelStartTicks = runTicks
	g.sfx = sfx
	g.procWorld = world
	g.procCX, g.procCY = cx, cy
	g.startMap = startMap
	g.runLog = runLog
	g.mapName = procMapName
	g.x, g.y, g.angle = px, py, ang
	g.vx, g.vy = vx, vy
	g.prevX, g.prevY, g.prevAngle = px, py, ang
	g.snapAlliesToShip() // re-form companions at the new chunk position
	g.portalGrace = portalGraceFrames
}
