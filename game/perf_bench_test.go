package game

import (
	"math"
	"testing"

	"linefire/filoio"
)

// benchGame loads the campaign's first map for render-path CPU benchmarks.
func benchGame(b *testing.B) *Game {
	b.Helper()
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		b.Fatalf("load player: %v", err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		b.Fatalf("load level: %v", err)
	}
	return New(player, lvl, "../gameassets", false)
}

// BenchmarkVisibilityPolygon measures the fog-of-war line-of-sight polygon, the
// per-frame CPU cost behind drawBrushFog's stamp.
func BenchmarkVisibilityPolygon(b *testing.B) {
	g := benchGame(b)
	b.ResetTimer()
	for b.Loop() {
		g.visibilityPolygon(g.x, g.y, g.bounds.diagonal())
	}
}

// BenchmarkVisibilityPolygonScreenReach is the same polygon bounded to the
// on-screen reach (what the overlay can actually show).
func BenchmarkVisibilityPolygonScreenReach(b *testing.B) {
	g := benchGame(b)
	b.ResetTimer()
	for b.Loop() {
		g.visibilityPolygon(g.x, g.y, g.discoveryReach())
	}
}

// BenchmarkFindPath measures one A* search across the first map — the per-repath
// cost every pursuing enemy and escort pays (up to the horde cap, every
// repathInterval frames).
func BenchmarkFindPath(b *testing.B) {
	g := benchGame(b)
	if g.nav == nil {
		b.Fatal("expected a navgrid")
	}
	// The farthest REACHABLE free cell from the start: a long search that must
	// actually succeed (an unreachable corner would return nil instantly).
	var tx, ty float64
	best := -1.0
	for cy := range g.nav.rows {
		for cx := range g.nav.cols {
			if g.nav.isBlocked(cx, cy) {
				continue
			}
			c := g.nav.center(cx, cy)
			d := math.Hypot(c.x-g.x, c.y-g.y)
			if d > best && len(g.nav.findPath(g.x, g.y, c.x, c.y)) > 0 {
				best, tx, ty = d, c.x, c.y
			}
		}
	}
	if best < 0 {
		b.Fatal("no reachable target found")
	}
	b.ResetTimer()
	for b.Loop() {
		if g.nav.findPath(g.x, g.y, tx, ty) == nil {
			b.Fatal("the benchmark path must resolve")
		}
	}
}
