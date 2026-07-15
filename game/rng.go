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
