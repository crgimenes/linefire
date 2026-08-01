package procgen

import (
	"fmt"
	"math/rand/v2"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// DefaultArenaW and DefaultArenaH are used when a caller passes no size. Big
// enough that a fight spreads out, walled so nothing can leave.
const (
	DefaultArenaW = 2400.0
	DefaultArenaH = 1600.0
)

// GenArena builds an open field of the given size in world units — zero or less
// takes the default: a walled rectangle and nothing else inside it.
//
// It is the counterpart to GenCaveRoom, for a battle that is NOT meant to happen
// in a cave. The perimeter exists for PHYSICS, not for the eye: it declares no
// stroke and no glow, so nothing is ever drawn for it. An open field has no
// fence — a caller that fits the arena to a screen gets the screen edge (or the
// window frame) as the visible border, which is the point. Collision does not
// care: wall segments are read from the paths regardless of how the layer looks.
//
// Spawns are left to the caller: an arena is a place to fight, not a stage.
func GenArena(seed int64, w, h float64) *level.Level {
	if w <= 0 || h <= 0 {
		w, h = DefaultArenaW, DefaultArenaH
	}
	lvl := level.New()
	lvl.Name = fmt.Sprintf("arena %d", seed)
	lvl.Title = "Open Field"
	lvl.Tags = []string{"stage", "arena"}
	lvl.Size = asset.Size{W: w, H: h}
	lvl.PlayerStart = level.Start{X: w / 2, Y: h / 2, Angle: -90}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Fill: "transparent",
		Paths: []asset.Path{arenaBounds(w, h)},
	}}
	lvl.Entries = []level.Entry{{Name: "arena", X: w / 2, Y: h / 2, Angle: -90}}
	lvl.Spawns = []level.Spawn{}
	return lvl
}

// arenaBounds is the perimeter, as one closed path.
func arenaBounds(w, h float64) asset.Path {
	return asset.Path{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0},
		{Op: asset.OpLineTo, X: w, Y: 0},
		{Op: asset.OpLineTo, X: w, Y: h},
		{Op: asset.OpLineTo, X: 0, Y: h},
		{Op: asset.OpClose},
	}}
}

// caveArenaMinCells keeps a degenerate request (a tiny window) from producing a
// cave with no room to carve: below ~12 cells a side the chambers and their
// sealed border leave nothing.
const caveArenaMinCells = 12

// CaveArenaSize is the size GenCaveArena would produce for a requested one: the
// cave is carved on a cell grid, so the arena snaps down to whole cells. A
// caller that fits the arena to a view compares against this, not against the
// raw request. Zero or less takes the cave generator's classic square.
func CaveArenaSize(w, h float64) (float64, float64) {
	rows, cols := caveArenaGrid(w, h)
	return float64(cols) * caveCell, float64(rows) * caveCell
}

// caveArenaGrid is the cell grid for a requested world size.
func caveArenaGrid(w, h float64) (rows, cols int) {
	if w <= 0 || h <= 0 {
		return caveRows, caveCols
	}
	return max(int(h/caveCell), caveArenaMinCells), max(int(w/caveCell), caveArenaMinCells)
}

// GenCaveArena builds a labyrinth arena close to the given size in world units
// (snapped down to whole cave cells — see CaveArenaSize): the cave generator
// fitted to a screen, for a battle among rock walls instead of in the open.
// Unlike GenArena's invisible fence, a maze IS its walls, so the layer keeps
// the cave's stroke and glow.
//
// Like GenArena it ships no spawns — an arena is a place to fight, not a stage.
// PlayerStart is the most open cell near the bottom of the carved region, so a
// caller that parks something there has it in reachable open space.
func GenCaveArena(seed int64, w, h float64) *level.Level {
	rows, cols := caveArenaGrid(w, h)
	// #nosec G404 G115 -- deterministic procedural generation (not crypto); the seed's bits are hashed on purpose
	rng := rand.New(rand.NewPCG(splitmix(uint64(seed)), 0x4d415a45)) // stream = "MAZE"
	grid, region := carveValidCave(rng, rows, cols)
	entry := bottomMost(grid, region)

	lvl := level.New()
	lvl.Name = fmt.Sprintf("maze %d", seed)
	lvl.Title = "Labyrinth"
	lvl.Tags = []string{"stage", "arena", "maze"}
	lvl.Size = asset.Size{W: float64(cols) * caveCell, H: float64(rows) * caveCell}
	lvl.PlayerStart = level.Start{X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Stroke: "#c8a0ff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8,
		Paths: caveWalls(grid),
	}}
	lvl.Entries = []level.Entry{{Name: "arena", X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}}
	lvl.Spawns = []level.Spawn{}
	return lvl
}
