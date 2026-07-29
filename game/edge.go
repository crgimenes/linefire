package game

import (
	"fmt"

	"github.com/crgimenes/linefire/procgen"
)

// Digging past the edge: an authored or generated flood map is a finite grid of rock (the padding
// beyond the walls, floodmap.go gridPad). Rather than let a tunnel dead-end at that boundary, when
// the ship digs to within edgeMargin of the grid edge we WARP it into a freshly generated, harder
// procedural screen (procgen.GenEdgeRoom) entered on the matching side — so the tunnel reads as
// continuing and the map never truly ends. Each breach digs one level deeper and one notch harder.
// This reuses the whole bonus-room path (a full destructible-rock screen with fog + persistence);
// the only new work is the detector and the transition.

// edgeDigDepth is how far PAST the map's authored bounds the ship must dig to trigger the warp. It
// is well short of the padded grid edge (gridPad ~420), so the ship is still surrounded by undug,
// fog-covered rock when it warps — the player is whisked away mid-tunnel, before the map's void
// edge is ever exposed. Digging "far ahead" means being teleported by surprise, not reaching an end.
const edgeDigDepth = 120.0

// diggingPastEdge reports whether the ship has dug this far past the authored map bounds, and which
// side. Only flood maps have rock to dig through (procedural @proc maps have no flood field); on a
// fresh map the ship starts inside the bounds, so this cannot false-fire on arrival.
func (g *Game) diggingPastEdge() (int, bool) {
	if g.flood == nil {
		return 0, false
	}
	b := g.bounds
	switch {
	case g.x < b.minX-edgeDigDepth:
		return procgen.EdgeLeft, true
	case g.x > b.maxX+edgeDigDepth:
		return procgen.EdgeRight, true
	case g.y < b.minY-edgeDigDepth:
		return procgen.EdgeTop, true
	case g.y > b.maxY+edgeDigDepth:
		return procgen.EdgeBottom, true
	}
	return 0, false
}

// oppositeEdge maps the breached side to the side the ship should ARRIVE on in the next screen, so
// it continues in the same direction (breach the right edge -> arrive on the new screen's left).
func oppositeEdge(side int) int {
	switch side {
	case procgen.EdgeLeft:
		return procgen.EdgeRight
	case procgen.EdgeRight:
		return procgen.EdgeLeft
	case procgen.EdgeTop:
		return procgen.EdgeBottom
	default:
		return procgen.EdgeTop
	}
}

// advancePastEdge handles the ship digging to the map edge. In the Rift (endless) it opens the NEXT
// Rift screen — so a room whose forward portal never opened (an enemy hidden in the fog, so it never
// counted as "cleared") is never a dead end; you can always dig on. Otherwise it warps into a fresh
// edge room (the mid-campaign detour, which keeps a way back).
func (g *Game) advancePastEdge(side int) {
	if g.endless {
		g.enterRift() // stays one-way: enterRift adds no return portal
		return
	}
	g.extendPastEdge(side)
}

// extendPastEdge generates the next screen beyond the breached edge and warps into it. digDepth
// (carried across maps) both drives the difficulty and keeps successive screens distinct; the run
// clock varies the seed so a re-dug edge is a fresh cave.
func (g *Game) extendPastEdge(breachSide int) {
	g.digDepth++
	ret := g.edgeReturn
	if ret == "" {
		ret = g.startMap // never entered an authored map via a portal (dug straight off the first map)
	}
	seed := edgeSeed(g.mapName, g.digDepth, breachSide, g.runTicks)
	lvl := procgen.GenEdgeRoom(seed, oppositeEdge(breachSide), g.digDepth, ret)
	// Themeless on purpose: the song on the air plays on; the jukebox rotates it.
	g.logf(">>> BREACH  depth %d — the rock opens <<<", g.digDepth)
	g.enterMap(lvl, fmt.Sprintf("@edge%d", g.digDepth), "from_edge", false)
}

// edgeSeed hashes the source map, depth, breach side and run clock into a seed (FNV-1a), so each
// breach is a different cave while a given breach is reproducible within the frame.
func edgeSeed(name string, depth, side, nonce int) int64 {
	var h uint64 = 14695981039346656037
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	for i := 0; i < len(name); i++ {
		mix(uint64(name[i]))
	}
	mix(uint64(depth)) // #nosec G115 -- hashing into a seed; wraparound intended
	mix(uint64(side))  // #nosec G115
	mix(uint64(nonce)) // #nosec G115
	return int64(h)    // #nosec G115
}
