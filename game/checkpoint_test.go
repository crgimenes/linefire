package game

import (
	"testing"

	"linefire/asset"
	"linefire/level"
)

func TestSaveCheckpointClonesRun(t *testing.T) {
	g := &Game{mapName: "m1", health: 42, fireLevel: 2, lives: 3}
	g.arsenal = []int{catFront, catLaser}
	z := level.Zone{Kind: level.ZoneCircle, Points: []asset.Point{{X: 100, Y: 200}}, Radius: 10, Trigger: triggerCheckpoint}

	g.saveCheckpoint(z)
	cp := g.checkpoint
	if !cp.valid || cp.x != 100 || cp.y != 200 {
		t.Fatalf("checkpoint should snapshot the zone center, got valid=%v (%.0f,%.0f)", cp.valid, cp.x, cp.y)
	}
	if cp.health != 42 || cp.fireLevel != 2 || cp.lives != 3 {
		t.Fatal("checkpoint should snapshot vitals and mods")
	}

	// Mutating the live run must not change the stored snapshot (it is a clone).
	g.arsenal[0] = catMine
	g.fireLevel = 4
	if g.checkpoint.arsenal[0] != catFront {
		t.Fatal("checkpoint arsenal must be a clone, not a shared slice")
	}
	if g.checkpoint.fireLevel != 2 {
		t.Fatal("checkpoint fireLevel is a value snapshot")
	}
}

// TestRespawnKeepsPosition: losing a life is a setback, not a checkpoint reset — the ship stays
// where it fell and only gains brief invulnerability. (A GAME OVER is what returns to the
// checkpoint, via restoreCheckpoint.)
func TestRespawnKeepsPosition(t *testing.T) {
	g := &Game{level: &level.Level{}, mapName: "m1", x: 300, y: 400, angle: 90}
	g.checkpoint = checkpoint{valid: true, mapName: "m1", x: 700, y: 800, angle: 0}

	g.respawn()

	if g.x != 300 || g.y != 400 {
		t.Fatalf("losing a life must not teleport the ship, even with a checkpoint set, got (%.0f,%.0f)", g.x, g.y)
	}
	if g.invuln <= 0 {
		t.Fatal("respawn should grant brief invulnerability")
	}
}

func TestCheckpointSnapshotsMapProgress(t *testing.T) {
	g := &Game{mapName: "m1"}
	g.mapStates = map[string]*mapState{
		"m1": {consumed: map[int]bool{3: true}, reached: map[string]bool{}, resolved: map[int]bool{}},
	}
	z := level.Zone{Kind: level.ZoneCircle, Points: []asset.Point{{X: 0, Y: 0}}, Radius: 5, Trigger: triggerCheckpoint}
	g.saveCheckpoint(z)

	// The snapshot must be a deep copy: consuming more spawns after the checkpoint
	// must NOT leak into the saved state (that is what caused powerups to accumulate).
	g.mapStates["m1"].consumed[7] = true
	cp := g.checkpoint.mapStates["m1"]
	if !cp.consumed[3] {
		t.Fatal("checkpoint should remember spawns consumed before it")
	}
	if cp.consumed[7] {
		t.Fatal("a spawn consumed AFTER the checkpoint must not be in the snapshot")
	}
}

func TestFormatRunTime(t *testing.T) {
	// At the default 60 TPS: 0, 1s, 1m1.5s.
	if got := formatRunTime(0); got != "0:00.0" {
		t.Fatalf("zero should be 0:00.0, got %s", got)
	}
	if got := formatRunTime(60); got != "0:01.0" {
		t.Fatalf("60 ticks should be 0:01.0, got %s", got)
	}
	if got := formatRunTime(61*60 + 30); got != "1:01.5" {
		t.Fatalf("61.5s should be 1:01.5, got %s", got)
	}
}
