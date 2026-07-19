package game

import (
	"runtime"
	"testing"

	"linefire/filoio"
)

// TestSoakHeapStable guards against gradual accumulation: crg's iOS playtest reported
// the web build starting fast, slowing over minutes, and being killed by the OS — the
// signature of something growing without bound (and wasm linear memory never shrinks,
// so even freed spikes ratchet the ceiling). This soaks the ATTRACT sim — the game
// playing itself: autopilot, an endless horde spawning and dying, particles, shots,
// and a full procgen arena rebuild every ~20s — for thousands of frames and requires
// the Go heap to settle rather than climb. It bounds the GAME's side of the story;
// browser-side growth (audio players, textures) is measured on-device via the debug
// HUD's heap/sys readout.
func TestSoakHeapStable(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Fatalf("player: %v", err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		t.Fatalf("level: %v", err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.enterCredits() // the autonomous demo: the busiest steady-state the game has

	// Warm up caches (meshes, pools, the first arena regens), then baseline.
	for range 2000 {
		if err := g.Update(); err != nil {
			t.Fatalf("warmup update: %v", err)
		}
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// A minute of sim time, including three full arena rebuilds.
	for range 3600 {
		if err := g.Update(); err != nil {
			t.Fatalf("soak update: %v", err)
		}
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if after.HeapAlloc > before.HeapAlloc {
		growth := after.HeapAlloc - before.HeapAlloc
		// The pools breathe (particles, projectiles, horde entities), so allow real
		// slack — the failure mode being hunted is unbounded growth, not jitter.
		const maxGrowth = 12 << 20
		if growth > maxGrowth {
			t.Fatalf("heap grew %d MB across the soak (%.1f -> %.1f MB live): something is accumulating",
				growth>>20, float64(before.HeapAlloc)/(1<<20), float64(after.HeapAlloc)/(1<<20))
		}
	}
}
