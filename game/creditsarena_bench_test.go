package game

import (
	"testing"

	"linefire/filoio"
	"linefire/procgen"
)

// BenchmarkCreditsArenaSwap measures the attract backdrop regen — the synchronous
// chunk that runs every ~20s inside one Update and starves the wasm audio pump.
func BenchmarkCreditsArenaSwap(b *testing.B) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		b.Fatalf("load player: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		lvl := procgen.GenCaveRoom(int64(randIntN(1<<30)), "")
		newWithContent(filoio.OSFS(), player, lvl, "../gameassets", false)
	}
}
