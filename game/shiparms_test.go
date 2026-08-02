package game

import "testing"

// armedGame is one armed ship of faction 1 and whatever else a test places.
func armedGame(t *testing.T, key string) *Game {
	t.Helper()
	g := salvageGame(t)
	g.entities[0].weaponKey = key
	return g
}

func foe(id int, x, y float64, hp int) entity {
	return entity{kind: kindEnemy, id: id, x: x, y: y, radius: 12, hp: hp, hpMax: hp, faction: 2}
}

// A carried weapon replaces the archetype bolt: the missile flies with the
// catalog's own speed and look, and carries a blast.
func TestCarriedMissileReplacesTheBolt(t *testing.T) {
	g := armedGame(t, "missile")
	g.enemyFire(&g.entities[0], 900, 500)

	if len(g.enemyShots) != 1 {
		t.Fatalf("one missile, got %d shots", len(g.enemyShots))
	}
	s := g.enemyShots[0]
	if s.aoe <= 0 {
		t.Fatal("a missile must carry a blast radius")
	}
	if s.hullDmg != shipMissileHullDamage {
		t.Errorf("missile hull damage %d, want %d", s.hullDmg, shipMissileHullDamage)
	}
	if s.faction != 1 || s.sid != 1 {
		t.Errorf("the missile lost its owner: %+v", s)
	}
}

// The blast does not ask whose ship it is. That is what makes an area weapon a
// decision: it kills the foe it was aimed at AND the ally standing too close,
// and the shooter's fleet is not credited with killing its own.
func TestBlastHurtsEveryoneInReach(t *testing.T) {
	g := armedGame(t, "missile")
	ally := foe(2, 620, 500, 1)
	ally.faction = 1 // the shooter's own fleet, right beside the target
	g.entities = append(g.entities, foe(3, 600, 500, 1), ally)

	g.enemyShots = []projectile{{
		x: 590, y: 500, px: 590, py: 500, vx: 20, vy: 0, life: 10,
		hullDmg: shipMissileHullDamage, aoe: 70, faction: 1, sid: 1,
	}}
	g.stepEnemyShots()

	for _, id := range []int{2, 3} {
		for i := range g.entities {
			if g.entities[i].id == id {
				t.Fatalf("ship %d survived a blast it was standing in", id)
			}
		}
	}
	if got := g.match.Stats[1].Kills; got != 1 {
		t.Errorf("faction 1 is credited %d kills; only the FOE counts", got)
	}
	if got := g.match.Stats[1].Losses; got != 1 {
		t.Errorf("faction 1 recorded %d losses; blowing up its own still costs it", got)
	}
}

// The laser is the prize: instant, and it burns every foe on the firing line —
// a fleet that lines up against one loses a row at a time.
func TestBeamBurnsEveryFoeOnTheLine(t *testing.T) {
	g := armedGame(t, "laser")
	g.entities = append(g.entities,
		foe(2, 600, 500, 1), // on the line
		foe(3, 700, 500, 1), // also on the line, behind the first
		foe(4, 700, 700, 1), // off the line
	)
	ally := foe(5, 650, 500, 1)
	ally.faction = 1 // an ally ON the line: a beam is aimed, it does not burn its own
	g.entities = append(g.entities, ally)

	g.enemyFire(&g.entities[0], 900, 500)

	alive := map[int]bool{}
	for i := range g.entities {
		alive[g.entities[i].id] = true
	}
	if alive[2] || alive[3] {
		t.Fatalf("both foes on the line should be burned: %v", alive)
	}
	if !alive[4] {
		t.Error("a ship off the firing line must not be hit")
	}
	if !alive[5] {
		t.Error("the beam burned its own fleet; only the blast does that")
	}
	if len(g.enemyShots) != 0 {
		t.Error("a beam is instant: it must not leave a projectile in flight")
	}
	if len(g.beams) != 1 {
		t.Fatalf("the burn should be drawn, got %d beams", len(g.beams))
	}
	if got := g.match.Stats[1].Kills; got != 2 {
		t.Errorf("faction 1 credited %d kills, want 2", got)
	}
}

// Rock stops a beam: what is behind a wall is not on the firing line.
func TestBeamIsStoppedByRock(t *testing.T) {
	g := armedGame(t, "laser")
	g.entities = append(g.entities, foe(2, 700, 500, 1))
	g.segs = []segment{{ax: 600, ay: 400, bx: 600, by: 600}}

	g.enemyFire(&g.entities[0], 900, 500)
	for i := range g.entities {
		if g.entities[i].id == 2 {
			return // survived behind the wall, as it should
		}
	}
	t.Fatal("the beam burned a ship standing behind rock")
}

// The beams fade rather than lingering forever.
func TestBeamsFadeOut(t *testing.T) {
	g := armedGame(t, "laser")
	g.enemyFire(&g.entities[0], 900, 500)
	for range shipBeamFrames + 1 {
		g.stepShipBeams()
	}
	if len(g.beams) != 0 {
		t.Fatalf("%d beams still drawn after they should have faded", len(g.beams))
	}
}
