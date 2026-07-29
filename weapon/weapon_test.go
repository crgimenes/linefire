package weapon

import "testing"

func TestForFallsBackToTheGun(t *testing.T) {
	if got := For(CatMissile); got.Name != "Missile" {
		t.Errorf("For(CatMissile) returned %q", got.Name)
	}
	if got := For(-1); got.Name != Catalog[CatFront].Name {
		t.Errorf("a negative index gave %q, want the front gun", got.Name)
	}
	if got := For(len(Catalog)); got.Name != Catalog[CatFront].Name {
		t.Errorf("an index past the catalogue gave %q, want the front gun", got.Name)
	}
}

// Every entry has to be usable: a weapon with no cooldown fires every frame, and
// one with no name cannot be shown to a player picking it up.
func TestCatalogEntriesAreUsable(t *testing.T) {
	for i, w := range Catalog {
		switch {
		case w.Name == "":
			t.Errorf("catalogue entry %d has no name", i)
		case w.Cooldown <= 0:
			t.Errorf("%s has no cooldown", w.Name)
		case w.Damage <= 0:
			t.Errorf("%s does no damage", w.Name)
		}
	}
}

// The kinds carry their own parameters, and a missing one is a weapon that does
// nothing when fired.
func TestEachKindCarriesItsParameters(t *testing.T) {
	for _, w := range Catalog {
		switch w.Kind {
		case KindProjectile:
			if w.Speed <= 0 || w.Life <= 0 {
				t.Errorf("%s is a projectile with speed %.1f and life %d", w.Name, w.Speed, w.Life)
			}
		case KindMine:
			if w.TriggerRadius <= 0 || w.MaxLive <= 0 || w.AOE <= 0 {
				t.Errorf("%s is a mine that cannot trigger, stack or blast", w.Name)
			}
		case KindLaser:
			if w.Reach <= 0 {
				t.Errorf("%s is a beam with no reach", w.Name)
			}
		}
	}
}
