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

// A single faction is solo practice: there is nobody to annihilate, so the
// battle flies on instead of declaring a winner on the first tick. It ends
// only if that fleet somehow ceases to exist.
func TestSoloPracticeDoesNotEndOnItsOwn(t *testing.T) {
	m, _ := newMatch(MatchLastFleet, 4, 0, 1)
	for range 600 {
		m.step(1, 1)
	}
	if _, over := m.Over(); over {
		t.Fatal("a lone fleet has nobody to defeat: the battle should not end")
	}
	m.step(0, 0)
	winner, over := m.Over()
	if !over || winner != 0 {
		t.Fatalf("an empty field is a draw, got winner=%d over=%v", winner, over)
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

// The opening battle waits for the window to settle. A window reports the
// requested size, then whatever the platform actually gave it, and a battle
// staged in between is dealt onto an arena about to be rebuilt — which is
// exactly what made the boot sequence stage three battles in a third of a
// second, ships materialising and vanishing twice before the real one began.
func TestOpeningBattleWaitsForTheViewToSettle(t *testing.T) {
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{
		Mode: MatchLastFleet, Ships: 2,
	})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	if g.match.staged {
		t.Fatal("the battle was staged before any view existed")
	}

	// A window still settling: the size changes every few ticks, and each
	// change must restart the wait.
	g.dpr = 1
	for i, size := range [][2]int{{800, 600}, {1280, 720}, {1600, 900}} {
		g.sw, g.sh = size[0], size[1]
		for range stageStableTicks - 1 {
			g.stepSkirmishMeta()
		}
		if g.match.staged {
			t.Fatalf("staged while the view was still changing (size %d)", i)
		}
	}

	// The size holds still: the battle is staged once, on the settled arena.
	for range stageStableTicks + 1 {
		g.stepSkirmishMeta()
	}
	if !g.match.staged {
		t.Fatal("the battle never staged on a settled view")
	}
	if !g.arenaFitsView() {
		t.Fatalf("staged on an arena that does not fit the view: %v vs %v", g.level.Size, [2]int{g.sw, g.sh})
	}
	if n := len(g.arrivals) + len(g.entities); n == 0 {
		t.Fatal("the fleets were never dealt")
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

// Every battle must open on a fresh deployment, not just a fresh map. The
// fleets are dealt from the HORDE's dice (dealFleets -> spawnArrival), and
// those were seeded from a constant — so the arena regenerated on every launch
// while the ships kept landing on exactly the same spots, session after
// session. crg caught it by eye: "as naves estão aparecendo no mesmo lugar
// quando eu peço o deploy novamente".
func TestEachArenaDealsItsFleetsSomewhereNew(t *testing.T) {
	deal := func() ([][2]float64, int64) {
		g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{
			Mode: MatchLastFleet, Ships: 3,
		})
		if err != nil {
			t.Fatalf("NewSkirmish: %v", err)
		}
		g.dealFleets()
		if len(g.arrivals) == 0 {
			t.Fatal("no fleet was dealt: the fixture proves nothing")
		}
		at := make([][2]float64, 0, len(g.arrivals))
		for _, a := range g.arrivals {
			at = append(at, [2]float64{a.x, a.y})
		}
		return at, g.arenaSeed
	}

	first, seedA := deal()
	second, seedB := deal()
	if seedA == seedB {
		t.Fatalf("two arenas were generated from the same seed %d", seedA)
	}
	if len(first) != len(second) {
		t.Fatalf("different fleet sizes dealt: %d and %d", len(first), len(second))
	}
	same := 0
	for i := range first {
		if first[i] == second[i] {
			same++
		}
	}
	if same == len(first) {
		t.Fatalf("both arenas dealt every ship to the identical spot: %v", first)
	}
}

// A won battle stays won. It used to count down and deal itself a fresh one,
// which is why a match watched to the end looked like it restarted — crg saw
// the winner flash past with a new field already landing. The verdict now
// holds until somebody asks for another battle.
func TestAWonBattleStaysWon(t *testing.T) {
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{
		Mode: MatchLastFleet, Ships: 2, Factions: 2,
	})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	g.match.staged = true
	g.match.winner, g.match.over = 3, true
	before := g.arenaSeed

	for range 60 * 30 { // half a minute of a decided battle
		g.stepMatchMeta()
	}

	winner, over := g.match.Over()
	if !over || winner != 3 {
		t.Fatalf("the verdict changed on its own: winner=%d over=%v", winner, over)
	}
	if g.arenaSeed != before {
		t.Error("a new arena was generated: the battle restarted itself")
	}
}

// The endless aquarium has no end to reach, so nothing above can freeze it.
func TestTheAquariumIsNeverOver(t *testing.T) {
	m, err := newMatch(MatchEndless, 4, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	m.step(1, 1) // one faction left would decide any real mode
	if _, over := m.Over(); over {
		t.Error("the endless mode declared a winner; it has no end condition")
	}
}
