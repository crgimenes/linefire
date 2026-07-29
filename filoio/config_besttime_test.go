package filoio

import (
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/config"
)

func TestRecordBestTimeAndPreserve(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.filo")
	best, rec, err := RecordBestTime(p, "m1", 500)
	if err != nil || !rec || best != 500 {
		t.Fatalf("first time should be a record: %v %v %d", err, rec, best)
	}
	if _, rec, _ := RecordBestTime(p, "m1", 600); rec {
		t.Fatal("a slower time is not a record")
	}
	if best, rec, _ := RecordBestTime(p, "m1", 400); !rec || best != 400 {
		t.Fatalf("a faster time is a record, got %v %d", rec, best)
	}
	// SaveAudio must not wipe the best times.
	if err := SaveAudio(p, 0.5, true); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if c.Volume != 0.5 || !c.Muted || c.BestTimes["m1"] != 400 {
		t.Fatalf("SaveAudio must preserve best times, got %+v", c)
	}
}

func TestRecordHighScoreAndPreserve(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.filo")
	best, rec, err := RecordHighScore(p, 1000)
	if err != nil || !rec || best != 1000 {
		t.Fatalf("first score should be a record: %v %v %d", err, rec, best)
	}
	if _, rec, _ := RecordHighScore(p, 800); rec {
		t.Fatal("a lower score is not a record")
	}
	if _, rec, _ := RecordHighScore(p, 1000); rec {
		t.Fatal("tying the score is not a record")
	}
	if best, rec, _ := RecordHighScore(p, 1500); !rec || best != 1500 {
		t.Fatalf("a higher score is a record, got %v %d", rec, best)
	}
	// A best-time write must not wipe the HI score, and vice-versa.
	if _, _, err := RecordBestTime(p, "m1", 400); err != nil {
		t.Fatal(err)
	}
	c := LoadConfig(p)
	if c.HighScore != 1500 || c.BestTimes["m1"] != 400 {
		t.Fatalf("high score and best times must coexist, got %+v", c)
	}
}

func TestRecordBestTimeRejectsImplausible(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.filo")
	best, rec, _ := RecordBestTime(p, "m1", 0) // the 0s-clear glitch
	if rec || best != 0 {
		t.Fatalf("a 0-tick clear must never be a record, got rec=%v best=%d", rec, best)
	}
	if _, ok := LoadConfig(p).BestTimes["m1"]; ok {
		t.Fatal("nothing should be stored for a sub-floor time")
	}
	// A legit time just over the floor still records.
	if _, rec, _ := RecordBestTime(p, "m1", config.MinBestTicks); !rec {
		t.Fatal("a plausible time at the floor should record")
	}
}
