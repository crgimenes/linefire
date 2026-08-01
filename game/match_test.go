package game

import (
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// A match validates its mode at creation: a typo fails at launch, not mid
// battle.
func TestMatchModeIsValidated(t *testing.T) {
	if _, err := newMatch("brawl", 0, 0, 2); err == nil {
		t.Fatal("an unknown mode should be rejected")
	}
	m, err := newMatch("", 0, 0, 2)
	if err != nil || m.Mode != MatchEndless {
		t.Fatalf("the empty mode should be the aquarium, got %v (%v)", m, err)
	}
}

// The aquarium never ends, whatever happens on the field.
func TestEndlessNeverEnds(t *testing.T) {
	m, _ := newMatch(MatchEndless, 4, 100, 2)
	for range 1000 {
		m.step(1, 2) // one faction left the whole time
	}
	if _, over := m.Over(); over {
		t.Fatal("the endless mode decided a battle; nothing ever ends in the aquarium")
	}
}

// Lastfleet is annihilation: it ends exactly when one faction remains — and a
// fleet's last hull still inside its vortex has not lost yet.
func TestLastFleetEndsOnAnnihilation(t *testing.T) {
	m, _ := newMatch(MatchLastFleet, 4, 0, 2)
	m.step(2, 0)
	if _, over := m.Over(); over {
		t.Fatal("two factions present: the battle is not decided")
	}
	m.step(1, 2)
	winner, over := m.Over()
	if !over || winner != 2 {
		t.Fatalf("one faction present: faction 2 should prevail, got %d/%v", winner, over)
	}
}

// The vortex grace, on the real counter: presentFactions counts arrivals as
// presence, so a faction is not annihilated while its ships materialise.
func TestPresentFactionsCountsArrivals(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, hp: 3, faction: 1}}
	g.arrivals = []arrival{{faction: 2, left: 10}}
	present, last := g.presentFactions()
	if present != 2 || last != 0 {
		t.Fatalf("a materialising fleet still counts: present=%d last=%d", present, last)
	}
}

// Timed judges by kills at the deadline; a dead-even score is a draw.
func TestTimedJudgesByKillsAtTheDeadline(t *testing.T) {
	m, _ := newMatch(MatchTimed, 4, 100, 2)
	m.recordKill(1, 2)
	m.recordKill(1, 2)
	m.recordKill(2, 1)
	for range 100 {
		m.step(2, 0)
	}
	winner, over := m.Over()
	if !over || winner != 1 {
		t.Fatalf("faction 1 leads 2-1 at the deadline, got winner=%d over=%v", winner, over)
	}

	tie, _ := newMatch(MatchTimed, 4, 10, 2)
	tie.recordKill(1, 2)
	tie.recordKill(2, 1)
	for range 10 {
		tie.step(2, 0)
	}
	winner, over = tie.Over()
	if !over || winner != 0 {
		t.Fatalf("a dead-even deadline is a draw, got winner=%d over=%v", winner, over)
	}
}

// The scoreboard is fed by the real fight: in a battle to annihilation, one
// side's kills are the other side's losses, and both equal the dead.
func TestBattleScoreboardReconciles(t *testing.T) {
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
	res, err := RunBattle(filoio.OSFS(), "../gameassets", BattleOptions{
		Programs: []string{hunter, `#t`},
		Ships:    4,
		Seed:     7,
	})
	if err != nil {
		t.Fatalf("RunBattle: %v", err)
	}
	if res.Stats == nil {
		t.Fatal("the result carries no scoreboard")
	}
	s1, s2 := res.Stats[1], res.Stats[2]
	if s1.Kills != 4 || s2.Losses != 4 {
		t.Fatalf("the idle fleet of 4 should be wiped: kills=%d losses=%d", s1.Kills, s2.Losses)
	}
	if s1.Shots == 0 {
		t.Fatal("the aggressor fired no recorded shots")
	}
	if s2.Kills != s1.Losses {
		t.Fatalf("scoreboard does not reconcile: %d kills vs %d losses", s2.Kills, s1.Losses)
	}
}
