package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// Maneuver tests: known tactics written in Filo, with the outcome asserted.
// This is how the language is validated — not "does it parse" but "does a
// program that SAYS strafe actually circle its target".

// maneuverGame is one piloted ship (faction 1) at (200,500) and one immobile,
// nearly-harmless turret target (faction 2) at (600,500), in an open field.
func maneuverGame(t *testing.T, src string) *Game {
	t.Helper()
	g := pilotGame(t, src)
	e := &g.entities[0]
	e.x, e.y = 200, 500
	e.hp = 999
	e.radar = 10000 // the maneuver is the subject, not the sensors
	g.entities = append(g.entities, entity{
		kind: kindEnemy, x: 600, y: 500, radius: 8, hp: 999, faction: 2,
		stationary: true, fireEvery: 1 << 30, radar: 10000,
	})
	return g
}

// dist to the target for the piloted ship.
func maneuverDist(g *Game) float64 {
	e, o := &g.entities[0], &g.entities[1]
	return math.Hypot(e.x-o.x, e.y-o.y)
}

// A standoff program — approach when far, back off when close — must settle
// near its chosen range and stay there.
func TestManeuverStandoffHoldsItsRange(t *testing.T) {
	g := maneuverGame(t, `
		(if (is-empty enemies)
		    (move 1 0)
		    (let ((e (head enemies)))
		      (let ((tx (nth e 1)) (ty (nth e 2)) (d (nth e 4)))
		        (if (> d 200)
		            (move (- tx self-x) (- ty self-y))
		            (move (- self-x tx) (- self-y ty))))))
	`)
	for range 600 {
		g.updateEnemies()
	}
	d := maneuverDist(g)
	if math.Abs(d-200) > 60 {
		t.Fatalf("standoff-at-200 program settled at %.0f", d)
	}
}

// A strafing program must actually ORBIT: the bearing from the target to the
// ship keeps advancing while the range stays in a band.
func TestManeuverStrafeCircles(t *testing.T) {
	g := maneuverGame(t, `
		(if (is-empty enemies)
		    (move 1 0)
		    (let ((e (head enemies)))
		      (let ((tx (nth e 1)) (ty (nth e 2)) (d (nth e 4)))
		        (if (> d 240)
		            (move (- tx self-x) (- ty self-y))
		            (move (- self-y ty) (- tx self-x))))))
	`)
	for range 300 {
		g.updateEnemies() // reach the fighting range first
	}
	o := &g.entities[1]
	prev := math.Atan2(g.entities[0].y-o.y, g.entities[0].x-o.x)
	swept := 0.0
	for range 400 {
		g.updateEnemies()
		cur := math.Atan2(g.entities[0].y-o.y, g.entities[0].x-o.x)
		delta := cur - prev
		for delta > math.Pi {
			delta -= 2 * math.Pi
		}
		for delta < -math.Pi {
			delta += 2 * math.Pi
		}
		swept += delta
		prev = cur
	}
	if math.Abs(swept) < math.Pi/2 {
		t.Fatalf("strafe program swept only %.0f degrees around its target in 400 ticks", swept*180/math.Pi)
	}
	if d := maneuverDist(g); d > 400 {
		t.Fatalf("strafe program drifted to range %.0f; an orbit holds its band", d)
	}
}

// A fleeing program opens the range: pure kite, no fight.
func TestManeuverFleeOpensTheRange(t *testing.T) {
	g := maneuverGame(t, `
		(if (is-empty enemies)
		    (move -1 0)
		    (let ((e (head enemies)))
		      (move (- self-x (nth e 1)) (- self-y (nth e 2)))))
	`)
	start := maneuverDist(g)
	for range 300 {
		g.updateEnemies()
	}
	if d := maneuverDist(g); d <= start+100 {
		t.Fatalf("fleeing program went from range %.0f to %.0f; it should run", start, d)
	}
}

// A program that drives straight into a wall stops at the wall — the engine
// slides hulls along rock, it never teleports them through. (crg watched
// piloted ships do exactly this against maze walls: that is the program's
// blindness, not an engine fault, and this test pins the physics.)
func TestManeuverWallStopsTheBlindPilot(t *testing.T) {
	g := pilotGame(t, `(move 0 -1)`)                         // due north, forever
	g.segs = []segment{{ax: 300, ay: 300, bx: 700, by: 300}} // a wall north of the ship
	for range 300 {
		g.updateEnemies()
	}
	e := &g.entities[0]
	if e.y < 300 {
		t.Fatalf("the ship passed through the wall to y=%.0f", e.y)
	}
	if e.y > 320 {
		t.Fatalf("the ship should be pressed against the wall, got y=%.0f", e.y)
	}
}

// The headless battle runner, end to end: hunter-style aggression against a
// program that does nothing must end in extermination, well under the budget,
// at full CPU speed. (An earlier version pitted the hunter against a FLEEING
// program and timed out — which is not a runner bug but emergent tactics:
// with equal hull speeds, kiting out of radar range breaks pursuit. The
// runner exists precisely to surface that kind of truth in seconds.)
func TestBattleAggressorWipesTheIdle(t *testing.T) {
	// Sensors are finite (radar 320 in a 2400x1600 field), so a hunter that
	// parks never finds anything: it has to SWEEP when blind. The first
	// version of this test parked at the center and proved exactly that.
	hunter := `
		(if first-tick (def sweep (* self-x 0.7)))
		(if (is-empty enemies)
		    (do
		      (def sweep (norm-angle (+ sweep 0.3)))
		      (move (cos sweep) (sin sweep)))
		    (let ((e (head enemies)))
		      (let ((tx (nth e 1)) (ty (nth e 2)))
		        (do
		          (face (bearing self-x self-y tx ty))
		          (move (- tx self-x) (- ty self-y))
		          (fire tx ty)))))
	`
	idle := `#t` // a program that orders nothing: the ship just sits there
	res, err := RunBattle(filoio.OSFS(), "../gameassets", BattleOptions{
		Programs: []string{hunter, idle},
		Ships:    4,
		Seed:     7,
	})
	if err != nil {
		t.Fatalf("RunBattle: %v", err)
	}
	if res.Winner != 1 {
		t.Fatalf("the aggressor did not win: winner=%d alive=%v after %d ticks", res.Winner, res.Alive, res.Ticks)
	}
	if res.Ticks >= battleDefaultTicks {
		t.Fatal("the battle timed out instead of being won")
	}
}
