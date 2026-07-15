package game

import (
	"testing"

	"linefire/procgen"
)

func TestProceduralLevelIsNavigable(t *testing.T) {
	// Across several seeds, an assembled procedural level must let the player reach
	// at least some enemies from the start (blocks never box the ship in).
	for seed := int64(1); seed <= 5; seed++ {
		w := procgen.NewWorld(t.TempDir(), seed)
		lvl, err := w.Assemble(0, 0)
		if err != nil {
			t.Fatalf("seed %d: assemble: %v", seed, err)
		}
		segs := wallSegments(lvl)
		nav := buildNavgrid(segs, mapBounds(lvl, segs))
		ps := lvl.PlayerStart
		reached, enemies := 0, 0
		for _, s := range lvl.Spawns {
			if spawnEntityKind(s.Kind) != kindEnemy {
				continue
			}
			enemies++
			path := nav.findPath(ps.X, ps.Y, s.X, s.Y)
			if len(path) > 0 {
				reached++
			}
		}
		if enemies > 0 && reached == 0 {
			t.Fatalf("seed %d: no enemies reachable from the start (player boxed in)", seed)
		}
	}
}

func TestProcSeedParsing(t *testing.T) {
	cases := map[string]int64{
		"@proc":     1,
		"@proc:42":  42,
		"@proc:-7":  -7,
		"@proc:bad": 1,
		"@proc:":    1,
	}
	for target, want := range cases {
		got := procSeed(target)
		if got != want {
			t.Fatalf("procSeed(%q) = %d, want %d", target, got, want)
		}
	}
}

func TestStreamProceduralNoopWithoutWorld(t *testing.T) {
	g := &Game{} // not in the procedural world
	if g.streamProcedural() {
		t.Fatal("streaming should be a no-op when procWorld is nil")
	}
}
