package game

import (
	"fmt"
	"strings"

	"linefire/procgen"
)

// "The Rift" — the endless trap. After the finale's boss dies a portal opens at the boss's fall
// (a DoPortal resolution, see gameassets/map0003.lfm); taking it drops the run into an UNBOUNDED
// chain of procedural Rift screens. Each is one-way: no return portal, a forward portal that opens
// only when the room is cleared, and a guard set that ramps with depth. There is no exit but death
// — an Atari-2600 kill screen you play for score. (Digging past a map's edge mid-campaign still
// enters the softer edge rooms, which DO have a way back; that path never sets g.endless.)

// riftMapName is the reserved portal target that (re)generates the next Rift screen.
const riftMapName = "@rift"

// enterRift generates and enters the next Rift screen, one step deeper. It latches g.endless, so
// from here every screen stays one-way and the whole run's earlier maps can be freed from memory.
func (g *Game) enterRift() {
	g.endless = true
	g.digDepth++ // the Rift depth: keeps successive screens distinct and paces the difficulty
	seed := edgeSeed(g.mapName, g.digDepth, 0, g.runTicks)
	// Difficulty ramps at HALF the depth, so the guard count and tough-pool climb slowly — the run
	// stays winnable deep in, a long tail to rack up score on.
	lvl := procgen.GenRiftRoom(seed, g.digDepth/2, riftMapName)
	lvl.Title = fmt.Sprintf("THE RIFT — depth %d", g.digDepth)
	// Themeless on purpose: the song on the air plays on; the jukebox rotates it.
	g.logf(">>> THE RIFT  depth %d — no way back <<<", g.digDepth)
	g.enterMap(lvl, fmt.Sprintf("@rift%d", g.digDepth), "from_rift", false)
}

// evictStaleProcMaps drops the remembered state of screens the run has left behind.
// Map states are CPU-side now (the fog texture is no longer retained per map — it is
// re-hydrated from the discovery grid on re-entry), so this is about not growing the
// store without bound. During the campaign only procedural ("@") screens are dropped —
// authored maps are a bounded set the player can revisit through portals. But once in
// the Rift (g.endless) the run can NEVER go back, so every map except the current one goes.
func (g *Game) evictStaleProcMaps(keep string) {
	for name := range g.mapStates {
		if name == keep {
			continue
		}
		if !g.endless && !strings.HasPrefix(name, "@") {
			continue // mid-campaign: keep authored maps (a revisit restores their fog/tunnels)
		}
		delete(g.mapStates, name)
	}
}
