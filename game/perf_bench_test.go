package game

import (
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
