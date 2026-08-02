package game

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
)

// The match model: a battle as a first-class object, Core Wars style. The
// ~20-second arena regeneration the overlay inherited from the attract screen
// lives ONLY in the endless mode (the aquarium); a real match has a mode, an
// end condition, a winner and a scoreboard.
//
// Modes:
//
//	MatchEndless   the aquarium: reinforcements forever, the arena regenerates,
//	               nothing ever ends. Stats still count — the eventual panel
//	               reads them — and survive arena regenerations.
//	MatchLastFleet annihilation: each faction lands one fleet, no
//	               reinforcements, the last faction with ships on the field
//	               (or in a vortex) prevails. A single faction is solo
//	               practice — there is nobody to annihilate, so it flies on.
//	MatchTimed     a deadline: fleets land, no reinforcements, and when time
//	               runs out the most kills prevail (a wipe still ends it
//	               early). A dead-even score is a draw.
const (
	MatchEndless   = "endless"
	MatchLastFleet = "lastfleet"
	MatchTimed     = "timed"
)

// matchCooldownTicks is how long a finished battle stays on screen — the
// survivors flying their victory lap — before the next one begins.
const matchCooldownTicks = 300 // 5s

// FleetStats is one faction's scoreboard.
type FleetStats struct {
	Kills    int // enemy ships destroyed
	Losses   int // own ships destroyed
	Shots    int // bolts fired (with Kills, an accuracy read)
	Powerups int // reserved: ships cannot collect loot yet (the Filo loop will)
}

// Match is the battle being fought: its rules, its clock and its scoreboard.
type Match struct {
	Mode     string
	Ships    int // fleet size per faction (lastfleet/timed)
	Duration int // deadline in ticks (timed)
	Factions int // teams dealt in: one is solo practice, with nobody to annihilate

	tick     int
	over     bool
	winner   int // faction that prevailed; 0 = draw (or not over yet)
	cooldown int
	Stats    map[int]*FleetStats

	// A battle is staged once the view has stopped changing size: a window
	// settles into its real dimensions over the first frames (the requested
	// size, then what the platform actually gave), and fleets dealt onto an
	// arena that is about to be rebuilt are dealt twice for nothing.
	staged    bool
	stableW   float64
	stableH   float64
	stableFor int
}

// stageStableTicks is how long the view size must hold still before the first
// battle of a session is staged.
const stageStableTicks = 10

// newMatch validates and builds a match. An empty mode is the aquarium.
func newMatch(mode string, ships, duration, factions int) (*Match, error) {
	switch mode {
	case "":
		mode = MatchEndless
	case MatchEndless, MatchLastFleet, MatchTimed:
	default:
		return nil, fmt.Errorf("unknown match mode %q (want %q, %q or %q)",
			mode, MatchEndless, MatchLastFleet, MatchTimed)
	}
	if ships <= 0 {
		ships = battleDefaultShips
	}
	if duration <= 0 {
		duration = battleDefaultTicks
	}
	m := &Match{Mode: mode, Ships: ships, Duration: duration, Factions: factions, Stats: map[int]*FleetStats{}}
	for f := 1; f <= factions; f++ {
		m.Stats[f] = &FleetStats{}
	}
	return m, nil
}

// stats returns the faction's counters, creating them on first sight (a
// campaign-side faction 0 never registers; skirmish factions always exist).
func (m *Match) stats(faction int) *FleetStats {
	if m == nil || faction == 0 {
		return nil
	}
	s := m.Stats[faction]
	if s == nil {
		s = &FleetStats{}
		m.Stats[faction] = s
	}
	return s
}

// recordShot and recordKill are nil-safe so the campaign (no match) pays one
// comparison and nothing else.
func (m *Match) recordShot(faction int) {
	if s := m.stats(faction); s != nil {
		s.Shots++
	}
}

func (m *Match) recordKill(by, of int) {
	if s := m.stats(by); s != nil {
		s.Kills++
	}
	if s := m.stats(of); s != nil {
		s.Losses++
	}
}

// Over reports whether the battle has been decided, and for whom (0 = draw).
func (m *Match) Over() (winner int, over bool) {
	if m == nil {
		return 0, false
	}
	return m.winner, m.over
}

// step advances the match one tick and decides it when its condition is met.
// present is how many factions still have ships on the field or in a vortex;
// lastAlive is the one present faction when present == 1.
func (m *Match) step(present, lastAlive int) {
	if m == nil || m.over || m.Mode == MatchEndless {
		return
	}
	m.tick++
	switch {
	case present == 0:
		m.winner, m.over = 0, true // mutual destruction: nobody prevails
	case present == 1 && m.Factions > 1:
		m.winner, m.over = lastAlive, true // annihilation ends any mode early
	case m.Mode == MatchTimed && m.tick >= m.Duration:
		m.winner, m.over = m.topKills(), true
	}
}

// topKills is the timed mode's judge: most kills prevails, a tie is a draw.
func (m *Match) topKills() int {
	best, bestKills, tied := 0, -1, false
	for f, s := range m.Stats {
		switch {
		case s.Kills > bestKills:
			best, bestKills, tied = f, s.Kills, false
		case s.Kills == bestKills:
			tied = true
		}
	}
	if tied {
		return 0
	}
	return best
}

// result is the battle's outcome as one report, in campaign register.
func (m *Match) result() string {
	verdict := "the battle ends in a draw"
	if m.winner != 0 {
		verdict = fmt.Sprintf("faction %d prevails", m.winner)
	}
	var factions []int
	for f := range m.Stats {
		factions = append(factions, f)
	}
	sort.Ints(factions)
	var lines []string
	for _, f := range factions {
		s := m.Stats[f]
		lines = append(lines, fmt.Sprintf("faction %d: %d kills, %d losses, %d shots", f, s.Kills, s.Losses, s.Shots))
	}
	return fmt.Sprintf("%s after %.1fs — %s", verdict, float64(m.tick)/60, strings.Join(lines, "; "))
}

// presentFactions counts the factions still in the battle: ships alive on the
// field, plus ships still materialising — a fleet's last hull inside its
// vortex has not lost yet.
func (g *Game) presentFactions() (present, lastAlive int) {
	seen := map[int]bool{}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindEnemy && e.hp > 0 && !seen[e.faction] {
			seen[e.faction] = true
		}
	}
	for i := range g.arrivals {
		if !seen[g.arrivals[i].faction] {
			seen[g.arrivals[i].faction] = true
		}
	}
	for f := range seen {
		lastAlive = f
	}
	if len(seen) != 1 {
		lastAlive = 0
	}
	return len(seen), lastAlive
}

// stepMatchMeta is the skirmish meta-loop for REAL matches (the endless
// aquarium keeps stepSkirmishMeta's regeneration): tick the match, and when it
// is decided let the survivors fly their lap, then stage the next battle on a
// fresh field. A match owns its arena for its whole length, so a window
// resized mid-battle is honoured by the NEXT battle.
func (g *Game) stepMatchMeta() {
	m := g.match
	if !m.staged {
		g.stageWhenViewSettles()
		return
	}
	if m.over {
		m.cooldown--
		if m.cooldown <= 0 {
			g.startMatch()
		}
		return
	}
	present, last := g.presentFactions()
	m.step(present, last)
	_, over := m.Over()
	if over {
		m.cooldown = matchCooldownTicks
		log.Print("battle over: " + m.result())
		g.emitResultEvent()
	}
}

// stageWhenViewSettles holds the opening battle until the window has stopped
// changing size, then stages it. Without this the boot sequence staged a
// battle onto the pre-Layout default arena, then again for each size the
// window passed through on its way to its real one — three fleets dealt and
// two of them thrown away, which reads on screen as ships materialising and
// vanishing before the battle finally begins.
// A world with no view at all — a headless test — never reports a size; it
// simply waits out the same count and stages on whatever arena it has, so
// nothing can hang waiting for a window that will never exist.
func (g *Game) stageWhenViewSettles() {
	m := g.match
	m.stableFor++
	w, h := g.arenaWorldSize()
	if w > 0 && h > 0 && (math.Abs(w-m.stableW) > 1 || math.Abs(h-m.stableH) > 1) {
		m.stableW, m.stableH, m.stableFor = w, h, 0
		return
	}
	if m.stableFor >= stageStableTicks {
		g.startMatch()
	}
}

// startMatch stages a battle: a fresh field, a fresh scoreboard, and each
// faction's fleet dealt into its vortices.
func (g *Game) startMatch() {
	g.buildCreditsArena()
	m, err := newMatch(g.match.Mode, g.match.Ships, g.match.Duration, g.factions)
	if err != nil {
		return // unreachable: the running mode was validated at launch
	}
	m.staged = true // whatever startMatch deals is a battle in progress
	g.match = m
	g.dealFleets()
	g.emitBattleEvent()
}

// dealFleets opens each faction's vortices for a real match; the endless
// aquarium is fed by the horde instead. The horde object stays either way for
// its asset cache, but never reinforces a battle — the fleets that land are
// the fleets there are.
func (g *Game) dealFleets() {
	if g.match.Mode == MatchEndless || g.horde == nil {
		return // the aquarium drips; bare test worlds have no spawner to deal with
	}
	for i := 0; i < g.match.Ships*g.factions; i++ {
		g.spawnArrival(battleKinds[i%len(battleKinds)], g.horde.rng)
	}
}
