package game

import "math/rand/v2"

// Loot drops: a slain enemy rolls a per-archetype table and, on a hit, drops a power-up (or
// a weapon) at its wreck — so combat feeds the player instead of only authored pickups. The
// roll is deterministic from a seeded stream so a run is reproducible.

// dropBand is one slice of an archetype's drop roll: a roll below `upto` drops `ref`. Bands
// are cumulative upper bounds in [0,1); the gap up to 1.0 is "nothing".
type dropBand struct {
	upto float64
	ref  string // the dropped asset's name (a power-up like "firepower", or a weapon "wpn_*")
}

// dropTable maps an enemy archetype (its asset Kind) to its cumulative drop bands. Tougher
// enemies drop more, and better — a tank is worth killing. Keyed by asset Kind; an unknown
// kind drops nothing.
var dropTable = map[string][]dropBand{
	"enemy":  {{0.12, "powerup"}},                                                                   // grunt: 12% heal
	"turret": {{0.15, "powerup"}, {0.25, "ratepower"}},                                              // 15% heal, +10% rate
	"rusher": {{0.15, "powerup"}, {0.23, "firepower"}},                                              // 15% heal, +8% fire
	"sniper": {{0.18, "powerup"}, {0.32, "damagepower"}},                                            // 18% heal, +14% damage
	"tank":   {{0.25, "shield"}, {0.45, "firepower"}, {0.65, "damagepower"}, {0.78, "wpn_missile"}}, // 78% drop, incl. a weapon
}

// rollDrop returns the asset a slain enemy of the given kind drops for a roll in [0,1), and
// whether anything dropped. Pure, so the table is testable without the RNG.
func rollDrop(kind string, roll float64) (string, bool) {
	for _, b := range dropTable[kind] {
		if roll < b.upto {
			return b.ref, true
		}
	}
	return "", false
}

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
	ref, ok := rollDrop(kind, g.rng.Float64())
	if ok {
		g.spawnReward(ref, x, y)
	}
}
