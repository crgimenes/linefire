package ship

import (
	"slices"
	"testing"
)

func TestForFallsBackToTheGrunt(t *testing.T) {
	if got := For("sniper"); got.Kind != "sniper" {
		t.Errorf("For(sniper) returned %q", got.Kind)
	}
	if got := For("battlestar"); got.Kind != "enemy" {
		t.Errorf("an unknown hull got profile %q, want the grunt", got.Kind)
	}
}

// A battle fielding every type has to come out the same on every run, so the
// list must not inherit Go's map iteration order.
func TestKindsAreSorted(t *testing.T) {
	kinds := Kinds()
	if !slices.IsSorted(kinds) {
		t.Fatalf("Kinds() is not sorted: %v", kinds)
	}
	if !slices.Equal(kinds, Kinds()) {
		t.Fatal("Kinds() returned a different order on a second call")
	}
	if len(kinds) < 2 {
		t.Fatalf("only %d profiles registered", len(kinds))
	}
	for _, kind := range kinds {
		if For(kind).Kind != kind {
			t.Errorf("profile %q reports kind %q", kind, For(kind).Kind)
		}
	}
}

// Every profile has to be flyable: a zero here is a hull that cannot take a hit,
// cannot see, or never shoots.
func TestProfilesAreUsable(t *testing.T) {
	for _, kind := range Kinds() {
		p := For(kind)
		switch {
		case p.HP <= 0:
			t.Errorf("%s has no hull points", kind)
		case p.Radar <= 0:
			t.Errorf("%s cannot see", kind)
		case p.FireEvery <= 0:
			t.Errorf("%s never shoots", kind)
		case p.ShotSpeed <= 0:
			t.Errorf("%s fires shots that do not move", kind)
		case p.SpeedMul <= 0 && !p.Stationary:
			t.Errorf("%s cannot move but is not marked stationary", kind)
		}
	}
}
