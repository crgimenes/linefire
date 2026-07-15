package game

import "testing"

// TestRollDropTable pins the per-archetype loot bands: which roll drops what, and where the
// gap to 1.0 (no drop) sits.
func TestRollDropTable(t *testing.T) {
	cases := []struct {
		kind string
		roll float64
		want string
		ok   bool
	}{
		{"enemy", 0.05, "powerup", true}, // grunt: heal in the 12% band
		{"enemy", 0.50, "", false},       // and nothing above it
		{"tank", 0.10, "shield", true},   // tank: shield / fire / damage / weapon, then nothing
		{"tank", 0.40, "firepower", true},
		{"tank", 0.60, "damagepower", true},
		{"tank", 0.70, "wpn_missile", true},
		{"tank", 0.90, "", false}, // ~22% of tanks drop nothing
		{"sniper", 0.25, "damagepower", true},
		{"nobody", 0.01, "", false}, // unknown archetype drops nothing
	}
	for _, c := range cases {
		got, ok := rollDrop(c.kind, c.roll)
		if got != c.want || ok != c.ok {
			t.Errorf("rollDrop(%q, %.2f) = (%q,%v), want (%q,%v)", c.kind, c.roll, got, ok, c.want, c.ok)
		}
	}
}

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
