// Package loot is the drop table: what a slain ship leaves behind, and how
// often. Combat feeds the fleet rather than only authored pickups, and a tougher
// hull is worth killing.
//
// The roll is pure — a table plus a number in [0,1) — so the rates are testable
// without an RNG and identical wherever they are rolled. Which stream that number
// comes from is the caller's business: Linefire wants a fixed loot stream so a run
// is reproducible, and a seeded simulation wants its own.
package loot

import "github.com/crgimenes/linefire/ship"

// Band is one slice of a drop roll: a roll below Upto drops Ref. Bands are
// cumulative upper bounds in [0,1), and the gap left up to 1.0 is "nothing".
type Band struct {
	Upto float64
	Ref  string // the dropped asset's name: a power-up like "firepower", or a weapon "wpn_*"
}

// Table maps a ship kind — its asset Kind, the same key the profiles use — to its
// cumulative bands. Tougher ships drop more, and better. An unknown kind drops
// nothing, so a hull nobody wrote a line for is simply not worth looting.
var Table = map[string][]Band{
	"enemy":  {{0.12, "powerup"}},                                                                   // grunt: 12% heal
	"turret": {{0.15, "powerup"}, {0.25, "ratepower"}},                                              // 15% heal, +10% rate
	"rusher": {{0.15, "powerup"}, {0.23, "firepower"}},                                              // 15% heal, +8% fire
	"sniper": {{0.18, "powerup"}, {0.32, "damagepower"}},                                            // 18% heal, +14% damage
	"tank":   {{0.25, "shield"}, {0.45, "firepower"}, {0.65, "damagepower"}, {0.78, "wpn_missile"}}, // 78% drop, incl. a weapon
}

// Roll returns what a slain ship of the given kind drops for a roll in [0,1), and
// whether anything dropped at all.
func Roll(kind string, roll float64) (string, bool) {
	for _, b := range Table[kind] {
		if roll < b.Upto {
			return b.Ref, true
		}
	}
	return "", false
}

// Refs lists every distinct thing the table can drop. A consumer uses it to load
// exactly the art it needs, instead of guessing or loading everything.
func Refs() []string {
	seen := map[string]bool{}
	var refs []string
	// ship.Kinds is sorted, so the result is stable: a caller loading art from it
	// must not depend on Go's map iteration order.
	for _, kind := range ship.Kinds() {
		for _, b := range Table[kind] {
			if seen[b.Ref] {
				continue
			}
			seen[b.Ref] = true
			refs = append(refs, b.Ref)
		}
	}
	return refs
}
