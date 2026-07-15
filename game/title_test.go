package game

import (
	"path/filepath"
	"strings"
	"testing"

	"linefire/filoio"
)

// TestBuildAttractRecords: the attract records page lists the HI score and best times from the
// saved config, and is empty until something has been recorded (so the front door just shows the
// title until there is a record to brag about).
func TestBuildAttractRecords(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.filo")
	g := &Game{sfx: &soundBank{cfgPath: p}}

	if lines := g.buildAttractRecords(); lines != nil {
		t.Fatalf("no records yet should yield no page, got %v", lines)
	}

	if _, _, err := filoio.RecordHighScore(p, 4200); err != nil {
		t.Fatal(err)
	}
	if _, _, err := filoio.RecordBestTime(p, "map0001", 3421); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(g.buildAttractRecords(), "|")
	for _, want := range []string{"HIGH SCORE", "4200", "BEST TIMES", "map0001"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("records page missing %q, got %q", want, joined)
		}
	}

	// Headless (no sfx / no config path) yields no page rather than panicking.
	if lines := (&Game{}).buildAttractRecords(); lines != nil {
		t.Fatalf("no sfx should yield no page, got %v", lines)
	}
}
