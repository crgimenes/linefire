package asset

import "testing"

func TestDrawRadiusHugsTheDrawing(t *testing.T) {
	a := New()
	a.Origin = Point{X: 10, Y: 10}
	a.Collisions = []CollisionShape{{Kind: CollisionCircle, Points: []Point{{X: 10, Y: 10}}, Radius: 40}}
	a.Layers = []Layer{{Paths: []Path{{Commands: []Command{
		{Op: OpMoveTo, X: 10, Y: 4},
		{Op: OpLineTo, X: 14, Y: 10},
		{Op: OpClose},
	}}}}}
	if got := a.DrawRadius(); got != 6 {
		t.Errorf("DrawRadius = %v, want 6 (the furthest drawn point), not the 40 hitbox", got)
	}
	if got := a.Radius(); got != 40 {
		t.Errorf("Radius = %v, want the 40 hitbox", got)
	}

	// Nothing drawn: fall back to the hitbox rather than to zero.
	a.Layers = nil
	if got := a.DrawRadius(); got != 40 {
		t.Errorf("an asset with no paths has DrawRadius %v, want its radius", got)
	}
}
