package sfx

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/crgimenes/gion"
)

func TestStereo16DuplicatesChannelsLittleEndian(t *testing.T) {
	got := Stereo16([]int16{0x0102, -1})
	// Each mono sample becomes 4 bytes: LE left then LE right (same value).
	want := []byte{0x02, 0x01, 0x02, 0x01, 0xff, 0xff, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Fatalf("Stereo16 = % x, want % x", got, want)
	}
}

func TestVariationsDistinctAndUnknown(t *testing.T) {
	vs := Variations("explosion", 202)
	if len(vs) != VariationCount {
		t.Fatalf("got %d variations, want %d", len(vs), VariationCount)
	}
	for k, pcm := range vs {
		if len(pcm) == 0 {
			t.Fatalf("variation %d rendered empty", k)
		}
	}
	if bytes.Equal(vs[0], vs[1]) {
		t.Fatal("a mutation should differ from the base")
	}
	if Variations("not-a-preset", 0) != nil {
		t.Fatal("an unknown preset should render nil")
	}
}

func TestVariationsPinPitchOnTonalWaves(t *testing.T) {
	// A tonal sound's variations must keep the base pitch (a ±10% jump between
	// repeated shots reads as a wrong note) while still differing somewhere else.
	tonal := gion.Blip(101)
	for k := 1; k < VariationCount; k++ {
		v := variationParams(tonal, k)
		if v.Freq != tonal.Freq || v.FreqSlide != tonal.FreqSlide {
			t.Fatalf("variation %d moved the pitch: freq %v->%v slide %v->%v",
				k, tonal.Freq, v.Freq, tonal.FreqSlide, v.FreqSlide)
		}
	}

	// Noise keeps the full drift: its frequency is texture, not pitch. Across the
	// deterministic variations at least one must move it.
	noise := gion.Explosion(202)
	moved := false
	for k := 1; k <= 8; k++ {
		if variationParams(noise, k).Freq != noise.Freq {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("noise variations should keep frequency drift")
	}
}

func TestRenderCoversPresetsAndLoops(t *testing.T) {
	if len(Render("blip", 101)) == 0 {
		t.Fatal("a preset base should render")
	}
	if len(Render("engine", 700)) == 0 {
		t.Fatal("a loop base should render one pass")
	}
	if Render("nope", 0) != nil {
		t.Fatal("an unknown base should render nil")
	}
}

func TestLoopRecipesRenderSteadyAndSeamless(t *testing.T) {
	for _, name := range []string{"engine", "beam"} {
		s := LoopSamples(name, 700)
		if len(s) != int(LoopSeconds*gion.DefaultRate) {
			t.Fatalf("%s: rendered %d samples, want %d", name, len(s), int(LoopSeconds*gion.DefaultRate))
		}
		// The envelope must be constant (no fade-in/out), or the loop pumps: the
		// loudest stretch near the end should be in the same league as the start.
		head, tail := peak(s[:2000]), peak(s[len(s)-2000:])
		if head == 0 || tail == 0 {
			t.Fatalf("%s: silent head/tail (head=%d tail=%d)", name, head, tail)
		}
		if ratio := float64(head) / float64(tail); ratio > 2 || ratio < 0.5 {
			t.Fatalf("%s: envelope not steady (head=%d tail=%d)", name, head, tail)
		}
	}
}

func TestLoopRecipeIsDeterministicAndUnknownIsNil(t *testing.T) {
	a := LoopSamples("engine", 700)
	b := LoopSamples("engine", 700)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("engine render is not deterministic at sample %d", i)
		}
	}
	if LoopSamples("not-a-loop", 0) != nil {
		t.Fatal("an unknown loop base should render nil")
	}
}

func TestBeamLoopClosesPhase(t *testing.T) {
	// The beam's freq and vibrato are whole cycles per loop, so the last sample must
	// land close to the first (a seamless splice, no click).
	s := LoopSamples("beam", 150)
	gap := math.Abs(float64(s[0]) - float64(s[len(s)-1]))
	if gap > float64(peak(s))*0.15 {
		t.Fatalf("beam loop splice gap %v is audible (peak %d)", gap, peak(s))
	}
}

func TestTrackKnownAndUnknownMood(t *testing.T) {
	pcm := Track("battle", 7)
	if len(pcm) == 0 {
		t.Fatal("a known mood should render a track")
	}
	if Track("polka", 7) != nil {
		t.Fatal("an unknown mood should render nil (silence)")
	}
}

func TestLocalRecipesAreLowAndRender(t *testing.T) {
	// The local recipes exist so shots and pickups are not shrill: keep their base
	// pitch low and make sure each resolves/renders like a preset (Bases lists them,
	// Variations plays them).
	for _, m := range recipeMaps {
		for name, fn := range m {
			if f := fn(0).Freq; f > 500 {
				t.Fatalf("%s base freq %.0fHz is too high (should stay low/round)", name, f)
			}
			if !Known(name) {
				t.Fatalf("%s should be a known base", name)
			}
			if len(Variations(name, 7)) != VariationCount {
				t.Fatalf("%s should render %d variations", name, VariationCount)
			}
		}
	}
	for _, name := range []string{"shot_soft", "collect_soft", "arrival_low"} {
		if !slices.Contains(Bases(), name) {
			t.Fatalf("%s should appear in the editor's base list", name)
		}
	}
}

func TestIsMusicFile(t *testing.T) {
	if !IsMusicFile("music/One_Heart_Remaining.mp3") || !IsMusicFile("A.MP3") {
		t.Fatal("mp3 paths should be music files")
	}
	if IsMusicFile("dark") || IsMusicFile("battle") {
		t.Fatal("gion mood names are not files")
	}
}

// peak returns the largest absolute sample value.
func peak(s []int16) int {
	m := 0
	for _, v := range s {
		a := int(v)
		if a < 0 {
			a = -a
		}
		if a > m {
			m = a
		}
	}
	return m
}

// TestClearWarmRegistered guards the level-clear/finale sound: it must be a real (warm, low)
// recipe, not the shrill stock "powerup" preset it replaced.
func TestClearWarmRegistered(t *testing.T) {
	if !Known("clear_warm") {
		t.Fatal("clear_warm should be a registered recipe")
	}
	if len(Render("clear_warm", 707)) == 0 {
		t.Fatal("clear_warm should render audio")
	}
}
