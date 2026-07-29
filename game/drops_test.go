package game

import "testing"

// The table and its bands are the loot package's; what these cover is the
// death -> roll -> spawn wiring in this world.

// TestMaybeDropLootDrops: over enough kills a tank (78% drop) leaves loot on the map — proves
// the death -> roll -> spawn wiring (spawnReward appends the pickup even when its asset is
// absent, which it is with no mapDir).
func TestMaybeDropLootDrops(t *testing.T) {
	g := &Game{rng: newDropRNG()}
	for range 40 {
		g.maybeDropLoot("tank", 0, 0)
	}
	if len(g.entities) == 0 {
		t.Fatal("a tank drops ~78% of the time; 40 kills should leave loot on the map")
	}
}

// TestMaybeDropLootNilRNGSafe: a bare Game (no RNG, how unit tests build one) never drops or
// panics.
func TestMaybeDropLootNilRNGSafe(t *testing.T) {
	g := &Game{}
	g.maybeDropLoot("tank", 0, 0)
	if len(g.entities) != 0 {
		t.Fatal("without an RNG nothing should drop")
	}
}
