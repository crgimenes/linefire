package loot

import (
	"slices"
	"testing"

	"github.com/crgimenes/linefire/ship"
)

// The bands are the drop rates: which roll drops what, and where the gap up to
// 1.0 — dropping nothing — sits.
func TestRollBands(t *testing.T) {
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
		{"nobody", 0.01, "", false}, // a kind nobody wrote a line for is not worth looting
	}
	for _, c := range cases {
		got, ok := Roll(c.kind, c.roll)
		if got != c.want || ok != c.ok {
			t.Errorf("Roll(%q, %.2f) = (%q,%v), want (%q,%v)", c.kind, c.roll, got, ok, c.want, c.ok)
		}
	}
}

// A tougher hull has to be worth killing, or the table is decoration.
func TestTougherShipsDropMore(t *testing.T) {
	chance := func(kind string) float64 {
		bands := Table[kind]
		if len(bands) == 0 {
			return 0
		}
		return bands[len(bands)-1].Upto
	}
	if chance("tank") <= chance("enemy") {
		t.Errorf("a tank drops %.2f of the time, a grunt %.2f", chance("tank"), chance("enemy"))
	}
}

// Refs is what a consumer loads its art from, so it has to be complete and it has
// to be stable — Go's map iteration order must not leak into it.
func TestRefsAreCompleteAndStable(t *testing.T) {
	refs := Refs()
	if !slices.Equal(refs, Refs()) {
		t.Fatal("Refs() returned a different order on a second call")
	}
	for _, kind := range ship.Kinds() {
		for _, b := range Table[kind] {
			if !slices.Contains(refs, b.Ref) {
				t.Errorf("%s can drop %q, which Refs() does not list", kind, b.Ref)
			}
		}
	}
	if len(refs) < 2 {
		t.Fatalf("only %d distinct drops in the table", len(refs))
	}
}

// Every kind the profiles field should have a line in the table, or a whole hull
// type is silently not worth killing.
func TestEveryShipKindHasABand(t *testing.T) {
	for _, kind := range ship.Kinds() {
		if len(Table[kind]) == 0 {
			t.Errorf("%s drops nothing at all", kind)
		}
	}
}
