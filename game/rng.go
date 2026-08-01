package game

import "math/rand/v2"

// Game randomness drives visual effects and AI variety only — it is not
// security sensitive, so the fast non-cryptographic generator is the right tool.
// Centralizing it keeps the gosec G404 suppression in one place.

// randFloat returns a pseudo-random float in [0, 1).
func randFloat() float64 {
	// #nosec G404 -- visual/gameplay variety, not cryptographic
	return rand.Float64()
}

// randIntN returns a pseudo-random int in [0, n).
func randIntN(n int) int {
	// #nosec G404 -- visual/gameplay variety, not cryptographic
	return rand.IntN(n)
}

// simFloat and simIntN are the SIMULATION's dice: identical to the globals in
// normal play, but drawn from the battle's own seeded generator when one is
// set (the headless runner), so a seeded battle replays byte for byte — a
// trace becomes a reproducible artifact, not a one-off observation.
func (g *Game) simFloat() float64 {
	if g.simRand != nil {
		return g.simRand.Float64()
	}
	return randFloat()
}

func (g *Game) simIntN(n int) int {
	if g.simRand != nil {
		return g.simRand.IntN(n)
	}
	return randIntN(n)
}
