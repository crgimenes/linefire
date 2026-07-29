package filoio

import (
	"reflect"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/config"
)

// TestConfigRoundTrips: settings must survive model -> Filo -> model, best times and all.
func TestConfigRoundTrips(t *testing.T) {
	want := config.Config{
		Volume:    0.35,
		Muted:     true,
		HighScore: 42800,
		BestTimes: map[string]int{
			"map0002": 5102,
			"map0001": 3421,
		},
	}
	src := EmitConfig(want)
	got, err := ParseConfig(src)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round trip lost data\n%s\nwant %+v\ngot  %+v", src, want, got)
	}
}

// TestConfigEmitIsStable: best times are written in sorted order, so rewriting the file
// after a run produces a minimal diff instead of shuffling lines.
func TestConfigEmitIsStable(t *testing.T) {
	c := config.Config{Volume: 0.5, BestTimes: map[string]int{"z": 1, "a": 2, "m": 3}}
	out := EmitConfig(c)
	ai, mi, zi := strings.Index(out, `"a"`), strings.Index(out, `"m"`), strings.Index(out, `"z"`)
	if ai >= mi || mi >= zi {
		t.Fatalf("best times must be sorted:\n%s", out)
	}
	// A zero-volume, unmuted, no-records config is still a valid single line.
	if EmitConfig(config.Config{}) == "" {
		t.Fatal("even an empty config must emit a (config ...) form")
	}
}

// TestConfigGrammarByHand pins the surface a user editing the file by hand sees.
func TestConfigGrammarByHand(t *testing.T) {
	c, err := ParseConfig(`(config (volume 0.8) (muted) (best-time "arena" 900))`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Volume != 0.8 || !c.Muted || c.BestTimes["arena"] != 900 {
		t.Fatalf("hand-written config wrong: %+v", c)
	}
	// muted is a presence flag: absent means false.
	c, err = ParseConfig(`(config (volume 0.8))`)
	if err != nil || c.Muted {
		t.Fatalf("absent (muted) must read as false: %+v %v", c, err)
	}
}
