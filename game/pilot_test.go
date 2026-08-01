package game

import (
	"math"
	"testing"
)

// pilotGame builds a bare skirmish world with one piloted ship (faction 1) and
// whatever else a test drops in, flying the given program.
func pilotGame(t *testing.T, src string) *Game {
	t.Helper()
	g := &Game{}
	g.skirmishMode = true
	g.bounds = bounds{minX: 0, minY: 0, maxX: 1000, maxY: 1000}
	g.filoEng = newPilotEngine()
	ais, err := compilePilots(g.filoEng, []string{src})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.factionAIs = ais
	g.entities = []entity{{kind: kindEnemy, x: 500, y: 500, radius: 8, hp: 3, faction: 1, radar: 400}}
	g.entities[0].pilot = g.pilotFor(1)
	return g
}

// (move) steers the ship: the order is a direction, the engine applies the
// hull's own speed and physics.
func TestPilotMoveSteersTheShip(t *testing.T) {
	g := pilotGame(t, `(move 1 0)`)
	g.updateEnemies()
	e := &g.entities[0]
	if e.x <= 500 {
		t.Fatalf("ship ordered east stayed at x=%v", e.x)
	}
	if e.y != 500 {
		t.Fatalf("ship ordered east drifted to y=%v", e.y)
	}
}

// (fire x y) shoots a faction bolt at the point, and the COOLDOWN is the
// engine's: a program spamming fire still shoots at the hull's rate.
func TestPilotFireRespectsTheCooldown(t *testing.T) {
	g := pilotGame(t, `(fire 900 500)`)
	g.updateEnemies()
	if len(g.enemyShots) != 1 {
		t.Fatalf("one fire order made %d shots", len(g.enemyShots))
	}
	if got := g.enemyShots[0].faction; got != 1 {
		t.Fatalf("the bolt carries faction %d, want the shooter's 1", got)
	}
	if g.enemyShots[0].vx <= 0 {
		t.Fatal("the bolt should fly east, toward the ordered point")
	}

	g.updateEnemies() // next tick: still on cooldown
	if len(g.enemyShots) != 1 {
		t.Fatalf("the cooldown let a second shot out (%d shots)", len(g.enemyShots))
	}
}

// The sensors report only what the ship can SEE: inside the radar and in line
// of sight — and (head enemies) is the nearest, which is the whole reason the
// lists are sorted.
func TestPilotSensorsRespectRadarWallsAndOrder(t *testing.T) {
	// The program remembers how many enemies it saw, so the test can read it back.
	g := pilotGame(t, `(def seen (length enemies))`)
	g.entities = append(g.entities,
		entity{kind: kindEnemy, x: 700, y: 500, radius: 8, hp: 3, faction: 2}, // visible
		entity{kind: kindEnemy, x: 500, y: 100, radius: 8, hp: 3, faction: 2}, // in range, behind the wall
		entity{kind: kindEnemy, x: 500, y: 950, radius: 8, hp: 3, faction: 2}, // beyond the radar (450 > 400)
	)
	g.segs = []segment{{ax: 400, ay: 300, bx: 600, by: 300}} // a wall north of the pilot

	g.updateEnemies()
	mem := g.entities[0].pilot.mem
	if got := mem["seen"].Num; got != 1 {
		t.Fatalf("the ship saw %v enemies; only the clear, in-range one is visible", got)
	}

	allies, enemies := g.sensorContacts(&g.entities[0])
	if len(allies) != 0 || len(enemies) != 1 {
		t.Fatalf("contacts = %d allies, %d enemies; want 0 and 1", len(allies), len(enemies))
	}
}

// Memory: what the program defines survives to the next tick, and first-tick
// guards initialisation (the program re-runs whole every tick).
func TestPilotMemorySurvivesTicks(t *testing.T) {
	g := pilotGame(t, `
		(if first-tick (def n 0) (def n (+ n 1)))
	`)
	for range 5 {
		g.updateEnemies()
	}
	if got := g.entities[0].pilot.mem["n"].Num; got != 4 {
		t.Fatalf("n = %v after 5 ticks, want 4 (0 on the first, +1 on each after)", got)
	}
}

// A broken program must not kill the ship or the game: the error is reported
// once and the hull falls back to the house brain, which just patrols here.
func TestPilotErrorFallsBackToTheHouseBrain(t *testing.T) {
	g := pilotGame(t, `(this-builtin-does-not-exist)`)
	g.updateEnemies() // must not panic
	if !g.factionAIs[1].errShown {
		t.Fatal("the script error was not reported")
	}
	bx, by := g.entities[0].x, g.entities[0].y
	for range 30 {
		g.updateEnemies()
	}
	e := &g.entities[0]
	if e.x == bx && e.y == by {
		t.Fatal("the fallback house brain should at least patrol")
	}
}

// The step budget is enforced: an infinite loop costs its ship one tick, not
// the game.
func TestPilotStepLimitStopsRunaways(t *testing.T) {
	g := pilotGame(t, `(fold (fn (a b) (+ a b)) 0 (range 0 1000000))`)
	g.updateEnemies() // must return promptly, not spin a million steps
	if !g.factionAIs[1].errShown {
		t.Fatal("the runaway program was not cut off by the step limit")
	}
}

// bearing is the didactic workhorse: from (0,0) to (0,10) is straight down the
// screen, +90 in the game's degrees.
func TestPilotMathBuiltins(t *testing.T) {
	g := pilotGame(t, `
		(def b (bearing 0 0 0 10))
		(def d (dist 3 0 0 4))
		(def c (clamp 15 0 10))
	`)
	g.updateEnemies()
	mem := g.entities[0].pilot.mem
	if got := mem["b"].Num; math.Abs(got-90) > 1e-9 {
		t.Errorf("bearing straight down = %v, want 90", got)
	}
	if got := mem["d"].Num; math.Abs(got-5) > 1e-9 {
		t.Errorf("dist(3,0 -> 0,4) = %v, want 5", got)
	}
	if got := mem["c"].Num; got != 10 {
		t.Errorf("clamp(15, 0, 10) = %v, want 10", got)
	}
}
