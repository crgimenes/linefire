package effects

import "testing"

// fixedRandom returns a source cycling through the given values, so a test can
// pin exactly what a burst comes out as.
func fixedRandom(values ...float64) func() float64 {
	i := 0
	return func() float64 {
		v := values[i%len(values)]
		i++
		return v
	}
}

func TestBurstSpawnsWithinItsRanges(t *testing.T) {
	p := New(DefaultMax, fixedRandom(0, 0.5, 0.999))
	b := Burst{N: 20, Style: StyleDot, SpeedMin: 1, SpeedMax: 5, LifeMin: 10, LifeMax: 20, Drag: 0.9, Size: 2}
	p.Burst(100, 200, b)

	if p.Len() != b.N {
		t.Fatalf("burst of %d spawned %d particles", b.N, p.Len())
	}
	for _, pt := range p.Particles() {
		if pt.X != 100 || pt.Y != 200 || pt.PX != 100 || pt.PY != 200 {
			t.Errorf("particle started at %.1f,%.1f (previous %.1f,%.1f), want the burst origin", pt.X, pt.Y, pt.PX, pt.PY)
		}
		if pt.Life < b.LifeMin || pt.Life > b.LifeMax {
			t.Errorf("life %d outside [%d, %d]", pt.Life, b.LifeMin, b.LifeMax)
		}
		if pt.Life != pt.MaxLife {
			t.Errorf("fresh particle has life %d but max life %d", pt.Life, pt.MaxLife)
		}
		if speed := pt.VX*pt.VX + pt.VY*pt.VY; speed < b.SpeedMin*b.SpeedMin-1e-9 || speed > b.SpeedMax*b.SpeedMax+1e-9 {
			t.Errorf("speed^2 %.3f outside [%.3f, %.3f]", speed, b.SpeedMin*b.SpeedMin, b.SpeedMax*b.SpeedMax)
		}
	}
}

// The cap is a backstop against a pile-up of explosions growing the slice without
// bound; particles are cosmetic, so going over it drops rather than blocks.
func TestPoolStopsAtItsCap(t *testing.T) {
	p := New(10, fixedRandom(0.5))
	p.Burst(0, 0, Burst{N: 100, LifeMin: 5, LifeMax: 5, Drag: 1})
	if p.Len() != 10 {
		t.Fatalf("pool holds %d particles, cap is 10", p.Len())
	}
}

func TestStepMovesAndRetiresParticles(t *testing.T) {
	p := New(DefaultMax, fixedRandom(0.5))
	p.Emit(Particle{X: 0, Y: 0, VX: 2, VY: 0, Drag: 0.5, Life: 2, MaxLife: 2})

	p.Step()
	pt := p.Particles()[0]
	if pt.X != 2 || pt.PX != 0 {
		t.Errorf("after one step: at %.1f, previous %.1f; want 2 and 0", pt.X, pt.PX)
	}
	if pt.VX != 1 {
		t.Errorf("drag left velocity %.2f, want 1", pt.VX)
	}

	p.Step()
	if p.Len() != 0 {
		t.Fatalf("a particle with 2 frames of life survived 2 steps: %d left", p.Len())
	}
}

func TestExplosionPlaysBothPresets(t *testing.T) {
	p := New(DefaultMax, fixedRandom(0.25))
	p.Explosion(0, 0)
	if want := ExplosionStreaks.N + ExplosionChunks.N; p.Len() != want {
		t.Fatalf("explosion spawned %d particles, want %d", p.Len(), want)
	}
}

// A pool is handed its randomness so callers that need reproducible effects get
// them; the same source has to replay the same burst.
func TestSameSourceGivesTheSameBurst(t *testing.T) {
	values := []float64{0.1, 0.7, 0.33, 0.9, 0.05}
	a, b := New(DefaultMax, fixedRandom(values...)), New(DefaultMax, fixedRandom(values...))
	a.Explosion(10, 20)
	b.Explosion(10, 20)
	for i, pa := range a.Particles() {
		if pb := b.Particles()[i]; pa != pb {
			t.Fatalf("particle %d differs: %+v vs %+v", i, pa, pb)
		}
	}
}
