package game

import (
	"math"

	"linefire/level"
)

// bounds is a world-space axis-aligned box. The fog, nav and flood grids are
// sized to it (with an origin) instead of to the level's nominal Size, so a map
// can extend anywhere — including negative coordinates and far beyond the editor
// Size hint — limited only by RAM.
type bounds struct {
	minX, minY, maxX, maxY float64
}

func (b bounds) w() float64        { return b.maxX - b.minX }
func (b bounds) h() float64        { return b.maxY - b.minY }
func (b bounds) diagonal() float64 { return math.Hypot(b.w(), b.h()) }

// mapBounds returns the world bounding box of all map content (walls, the player
// start and spawns) plus a margin. Falls back to the Size hint for an empty map.
func mapBounds(lvl *level.Level, segs []segment) bounds {
	b := bounds{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
	upd := func(x, y float64) {
		b.minX = math.Min(b.minX, x)
		b.minY = math.Min(b.minY, y)
		b.maxX = math.Max(b.maxX, x)
		b.maxY = math.Max(b.maxY, y)
	}
	for _, s := range segs {
		upd(s.ax, s.ay)
		upd(s.bx, s.by)
	}
	upd(lvl.PlayerStart.X, lvl.PlayerStart.Y)
	for _, s := range lvl.Spawns {
		upd(s.X, s.Y)
	}
	for _, e := range lvl.Entries {
		upd(e.X, e.Y)
	}
	if math.IsInf(b.minX, 1) {
		return bounds{0, 0, lvl.Size.W, lvl.Size.H}
	}
	const margin = 80.0
	return bounds{b.minX - margin, b.minY - margin, b.maxX + margin, b.maxY + margin}
}
