package game

import (
	"math/rand/v2"

	"github.com/crgimenes/linefire/loot"
)

// Loot drops: a slain enemy rolls a per-archetype table and, on a hit, drops a power-up (or
// a weapon) at its wreck — so combat feeds the player instead of only authored pickups. The
// roll is deterministic from a seeded stream so a run is reproducible.

// The table itself lives in the loot package, shared with Linefire Skirmish: the
// drop rates have to be the same wherever a ship dies. What stays here is the
// stream the roll comes from and what a dropped ref becomes in this world.

// newDropRNG builds the loot RNG: a fixed stream, so drops are deterministic across a run
// (fair for speedruns) while kill order still varies what actually falls.
func newDropRNG() *rand.Rand {
	// #nosec G404 -- loot variety, not a security boundary
	return rand.New(rand.NewPCG(1, 0x44524f5053)) // stream = "DROPS"
}

// maybeDropLoot rolls the drop table for a slain enemy of the given archetype and drops the
// loot at (x,y) on a hit. A nil RNG (a bare test Game) drops nothing.
func (g *Game) maybeDropLoot(kind string, x, y float64) {
	if g.rng == nil {
		return
	}
	ref, ok := loot.Roll(kind, g.rng.Float64())
	if ok {
		g.spawnReward(ref, x, y)
	}
}
