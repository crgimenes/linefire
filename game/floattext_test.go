package game

import "testing"

func TestDamageNumbersRiseAndExpire(t *testing.T) {
	g := &Game{}
	g.spawnDamageNumber(10, 20, 5, damageColor)
	if len(g.floaters) != 1 {
		t.Fatalf("floaters = %d, want 1", len(g.floaters))
	}
	if g.floaters[0].text != "5" {
		t.Fatalf("floater text = %q, want \"5\"", g.floaters[0].text)
	}

	y0 := g.floaters[0].y
	g.stepFloaters()
	if g.floaters[0].y >= y0 {
		t.Fatalf("number should rise (y decreases in this y-down space): %v -> %v", y0, g.floaters[0].y)
	}

	for range floaterLife {
		g.stepFloaters()
	}
	if len(g.floaters) != 0 {
		t.Fatalf("numbers should expire, %d left", len(g.floaters))
	}
}

func TestDamageNumberPoolIsBounded(t *testing.T) {
	g := &Game{}
	for range maxFloaters * 3 {
		g.spawnDamageNumber(0, 0, 1, damageColor)
	}
	if len(g.floaters) > maxFloaters {
		t.Fatalf("floater pool grew past the cap: %d > %d", len(g.floaters), maxFloaters)
	}
}
