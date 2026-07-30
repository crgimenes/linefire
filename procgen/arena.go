package procgen

import (
	"fmt"

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
