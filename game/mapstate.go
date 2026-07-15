package game

import "github.com/hajimehoshi/ebiten/v2"

// mapState remembers what the player already did in a map within the current run,
// so returning through a portal does not respawn cleared enemies / collected
// pickups or reset finished objectives. It is keyed by map name and survives the
// per-map rebuild in enterMap; a full restart (R) drops it for a fresh run.
type mapState struct {
	consumed map[int]bool    // spawn index -> removed (enemy killed or pickup taken)
	reached  map[string]bool // exit zone name -> reach objective satisfied
	resolved map[int]bool    // resolution index -> already fired (outcomes happen once)

	// dug is the rock this map lost to the player. While the map is loaded it ALIASES
	// the live floodmap's slice: carve writes it in place, so the store is never stale
	// and there is no publish step to forget. cloneMapStates copies it, and that copy
	// is what freezes a checkpoint's terrain.
	// yagni: one byte per cell (~500 KB for map0001) is free in memory; pack it to a
	// bitset or RLE when the terrain has to go through a save file (T24).
	dug []bool

	// disc and fogTex are the fog of war this map has had lifted: the coarse logic grid
	// (entity culling, minimap) and the high-res world-space cleared texture. They are
	// held by POINTER and handed straight back on a revisit — nothing is copied.
	// yagni: fogTex is a full-map GPU image (34 MB for map0001, 7 MB for map0002), so
	// VRAM grows with the maps visited in a run. If that ever bites, drop the texture
	// for stale maps and re-stamp it from disc (6.9 KB) — the edge comes back blocky at
	// the fogCell grid until the player flies past it again.
	disc   *discovery
	fogTex *ebiten.Image
}

// bindMapState joins the live derived world to this map's entry in the store: the rock
// the player dug out, and the fog they lifted. New() rebuilds both from the level file,
// so without this a revisited map arrives sealed and unexplored.
//
// Idempotent and nil-safe. Called when a map is entered, when one is left, and when one
// is frozen into a checkpoint, so no path can lose a tunnel by forgetting to publish it.
func (g *Game) bindMapState() {
	s := g.curMapState()
	g.bindTerrain(s)
	g.bindFog(s)
}

// bindTerrain cuts this map's remembered excavation back into the freshly built field
// and re-derives the A* (so enemies still follow the tunnels). From then on the store
// aliases the field, and every later carve is recorded for free. Procedural maps have
// no region field, so there is nothing to bind.
func (g *Game) bindTerrain(s *mapState) {
	if g.flood == nil {
		return
	}
	if g.flood.applyDug(s.dug) {
		g.nav = buildNavgridFromFlood(g.flood)
	}
	s.dug = g.flood.dug
}

// bindFog hands a revisited map back the fog it had already lifted. Unlike the rock,
// fog is KNOWLEDGE rather than world state: it is deliberately NOT rolled back by a
// checkpoint restore (dying does not un-see a corridor), the same rule the RTA clock
// follows. A grid of another shape is refused, so a resized map starts dark.
func (g *Game) bindFog(s *mapState) {
	if g.disc == nil {
		return // procedural maps run without fog
	}
	if s.disc != nil && len(s.disc.seen) == len(g.disc.seen) {
		g.disc, g.fogTex = s.disc, s.fogTex
	}
	s.disc, s.fogTex = g.disc, g.fogTex
}

// curMapState returns the saved state for the current map, creating it (and the
// store) on first use.
func (g *Game) curMapState() *mapState {
	if g.mapStates == nil {
		g.mapStates = map[string]*mapState{}
	}
	s := g.mapStates[g.mapName]
	if s == nil {
		s = &mapState{consumed: map[int]bool{}, reached: map[string]bool{}, resolved: map[int]bool{}}
		g.mapStates[g.mapName] = s
	}
	return s
}

// markConsumed records that a spawn (a killed enemy or collected pickup) is gone
// for the current map, so it does not return on a revisit.
func (g *Game) markConsumed(spawnIdx int) {
	if spawnIdx < 0 {
		return
	}
	g.curMapState().consumed[spawnIdx] = true
}

// applyMapState restores what a map remembers after it is (re)built: the tunnels the
// player dug into it, the spawns they already consumed and the reach objectives they
// finished. A first-time map remembers nothing and is left as freshly built.
func (g *Game) applyMapState() {
	g.bindMapState()
	s := g.curMapState()
	kept := g.entities[:0]
	for i := range g.entities {
		if s.consumed[g.entities[i].spawn] {
			continue
		}
		kept = append(kept, g.entities[i])
	}
	g.entities = kept

	for i := range g.objectives {
		o := &g.objectives[i]
		if o.kind == objReach && s.reached[o.zone] {
			o.done = true
		}
	}
}
