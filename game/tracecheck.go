package game

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// CheckTrace reads a battle trace (one or many battles per stream) and returns
// every invariant violation it finds, in order. It is the deterministic half
// of trace analysis: it turns megabytes of events into a short list of
// findings that a human — or an AI with all the time in the world — can then
// explain. An empty result means the battle behaved.
//
// Invariants checked:
//
//   - every position (spawn, snap, hit, death) lies inside the arena;
//   - a ship's hull only ever goes down, and only hits move it;
//   - nothing happens to a ship after its death, and nobody is hit or killed
//     by a ship that never existed (a shooter may die while its bolt flies,
//     so a dead "by" is legal — an unborn one is not);
//   - the result's survivor counts match the spawns minus the deaths;
//   - STALL findings: a live ship that moved less than stallEps per snapshot
//     for stallSnaps consecutive snapshots while foes were alive. A stall is
//     a real finding, not necessarily a bug — a wall-blind program pressing
//     rock stalls honestly — which is exactly why it is worth reporting.
func CheckTrace(r io.Reader) ([]string, error) {
	var finds []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	st := newTraceState()
	battle := 0
	for line := 1; sc.Scan(); line++ {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev traceCheckEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			return finds, fmt.Errorf("trace line %d: %w", line, err)
		}
		switch ev.Ev {
		case "battle":
			battle = ev.N
			st = newTraceState()
			st.w, st.h = float64(ev.W), float64(ev.H)
		case "result":
			finds = append(finds, st.checkResult(battle, ev)...)
		default:
			finds = append(finds, st.checkEvent(battle, ev)...)
		}
	}
	if err := sc.Err(); err != nil {
		return finds, err
	}
	return finds, nil
}

// stall thresholds: fewer than stallEps world units covered per snapshot, for
// stallSnaps snapshots in a row (snapshots are one second apart).
const (
	stallEps   = 3
	stallSnaps = 5
)

// traceCheckEvent decodes any line of the stream (unused fields stay zero).
type traceCheckEvent struct {
	T    int    `json:"t"`
	Ev   string `json:"ev"`
	ID   int    `json:"id"`
	F    int    `json:"f"`
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	By   int    `json:"by"`
	Dmg  int    `json:"dmg"`
	HP   int    `json:"hp"`
	Mv   int    `json:"mv"`

	// battle / result fields
	N      int            `json:"n"`
	W      int            `json:"w"`
	H      int            `json:"h"`
	Winner int            `json:"winner"`
	Alive  map[string]int `json:"alive"`
}

// traceShipState is what the checker remembers about one hull.
type traceShipState struct {
	faction  int
	hp       int
	dead     bool
	stalled  int  // consecutive low-movement snapshots
	reported bool // one stall finding per ship, not one per snapshot
}

type traceState struct {
	w, h   float64
	ships  map[int]*traceShipState
	deaths map[int]int // per faction
	spawns map[int]int // per faction
}

func newTraceState() *traceState {
	return &traceState{ships: map[int]*traceShipState{}, deaths: map[int]int{}, spawns: map[int]int{}}
}

// foesAlive reports whether any two live ships belong to different factions —
// while true, a stalled ship has something it should be doing.
func (st *traceState) foesAlive() bool {
	seen := 0
	for _, s := range st.ships {
		if s.dead {
			continue
		}
		if seen != 0 && s.faction != seen {
			return true
		}
		seen = s.faction
	}
	return false
}

func (st *traceState) inBounds(x, y int) bool {
	if st.w <= 0 || st.h <= 0 {
		return true // headerless stream: nothing to compare against
	}
	return x >= 0 && y >= 0 && float64(x) <= st.w && float64(y) <= st.h
}

func (st *traceState) checkEvent(battle int, ev traceCheckEvent) []string {
	var finds []string
	find := func(format string, args ...any) {
		finds = append(finds, fmt.Sprintf("battle %d t=%d: ", battle, ev.T)+fmt.Sprintf(format, args...))
	}

	if !st.inBounds(ev.X, ev.Y) {
		find("%s of ship %d at %d,%d is outside the %vx%v arena", ev.Ev, ev.ID, ev.X, ev.Y, st.w, st.h)
	}

	s := st.ships[ev.ID]
	switch ev.Ev {
	case "spawn":
		if s != nil {
			find("ship %d spawned twice", ev.ID)
			return finds
		}
		st.ships[ev.ID] = &traceShipState{faction: ev.F, hp: ev.HP}
		st.spawns[ev.F]++
	case "shot":
		if s == nil {
			find("shot from ship %d, which never spawned", ev.ID)
		} else if s.dead {
			find("shot from ship %d, which is dead", ev.ID)
		}
	case "hit":
		if s == nil {
			find("hit on ship %d, which never spawned", ev.ID)
			return finds
		}
		if s.dead {
			find("hit on ship %d, which is dead", ev.ID)
		}
		if ev.By != 0 && st.ships[ev.By] == nil {
			find("ship %d hit by ship %d, which never spawned", ev.ID, ev.By)
		}
		if ev.HP >= s.hp {
			find("hit on ship %d RAISED its hull: %d -> %d", ev.ID, s.hp, ev.HP)
		}
		s.hp = ev.HP
	case "death":
		if s == nil {
			find("death of ship %d, which never spawned", ev.ID)
			return finds
		}
		if s.dead {
			find("ship %d died twice", ev.ID)
		}
		if ev.By != 0 && st.ships[ev.By] == nil {
			find("ship %d killed by ship %d, which never spawned", ev.ID, ev.By)
		}
		s.dead = true
		st.deaths[s.faction]++
	case "snap":
		if s == nil {
			find("snapshot of ship %d, which never spawned", ev.ID)
			return finds
		}
		if s.dead {
			find("snapshot of ship %d, which is dead", ev.ID)
		}
		if ev.HP > s.hp {
			find("ship %d's hull ROSE between snapshots: %d -> %d", ev.ID, s.hp, ev.HP)
		}
		s.hp = ev.HP
		if ev.Mv < stallEps && st.foesAlive() {
			s.stalled++
			if s.stalled >= stallSnaps && !s.reported {
				s.reported = true
				find("stall: ship %d (faction %d) has barely moved for %ds at %d,%d with foes alive",
					ev.ID, s.faction, s.stalled, ev.X, ev.Y)
			}
		} else {
			s.stalled = 0
		}
	}
	return finds
}

// checkResult reconciles the scoreboard with the events: survivors must equal
// spawns minus deaths, per faction.
func (st *traceState) checkResult(battle int, ev traceCheckEvent) []string {
	var finds []string
	for f, spawned := range st.spawns {
		want := spawned - st.deaths[f]
		got := ev.Alive[fmt.Sprintf("%d", f)]
		if got != want {
			finds = append(finds, fmt.Sprintf(
				"battle %d result: faction %d has %d survivors but %d spawned - %d died = %d",
				battle, f, got, spawned, st.deaths[f], want))
		}
	}
	return finds
}
