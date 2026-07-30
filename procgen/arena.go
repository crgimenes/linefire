package procgen

import (
	"fmt"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// Arena dimensions, in world units. Big enough that a fight spreads out and the
// camera has somewhere to travel, walled so nothing can leave.
const (
	arenaW = 2400.0
	arenaH = 1600.0
)

// GenArena builds an open field: a walled rectangle and nothing else inside it.
//
// It is the counterpart to GenCaveRoom, for a battle that is NOT meant to happen
// in a cave. The difference is not only how it looks: a cave's wall layer is
// thousands of little segments, which the game can afford because it draws caves
// as negative space — solid fills with the corridors carved out — and never
// strokes those segments. Anything that has to draw walls as LINES wants a wall
// layer with four of them, and that is what this is.
//
// Spawns are left to the caller: an arena is a place to fight, not a stage.
func GenArena(seed int64) *level.Level {
	lvl := level.New()
	lvl.Name = fmt.Sprintf("arena %d", seed)
	lvl.Title = "Open Field"
	lvl.Tags = []string{"stage", "arena"}
	lvl.Size = asset.Size{W: arenaW, H: arenaH}
	lvl.PlayerStart = level.Start{X: arenaW / 2, Y: arenaH / 2, Angle: -90}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Stroke: "#80ffff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8,
		Paths: []asset.Path{arenaBounds()},
	}}
	lvl.Entries = []level.Entry{{Name: "arena", X: arenaW / 2, Y: arenaH / 2, Angle: -90}}
	lvl.Spawns = []level.Spawn{}
	return lvl
}

// arenaBounds is the perimeter, as one closed path.
func arenaBounds() asset.Path {
	return asset.Path{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0},
		{Op: asset.OpLineTo, X: arenaW, Y: 0},
		{Op: asset.OpLineTo, X: arenaW, Y: arenaH},
		{Op: asset.OpLineTo, X: 0, Y: arenaH},
		{Op: asset.OpClose},
	}}
}
