package game

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// The battle trace: the fight as TEXT, one JSON object per line, so behavior
// stops being something you watch and becomes an artifact you interrogate —
// with jq and awk, with the invariant checker (CheckTrace), with an AI reading
// it, or replayed byte for byte from the same seed. It is also the future
// record of a player-vs-player dispute: the trace is the match.
//
// Two granularities share the stream:
//
//   - EVENTS, sparse and semantic: {"t":481,"ev":"spawn","id":7,...} for
//     spawn, shot, hit, death. Every event names its tick and its ships by
//     stable id, and a hit names the shooter (by), so "who killed whom" needs
//     no correlation work.
//   - SNAPSHOTS, dense and rare: every snapEvery ticks, one "snap" line per
//     live ship with position, hull and mv — the distance moved since the
//     last snapshot. mv is what catches the stalled-ship class of bug without
//     watching a screen.
//
// A battle opens with {"ev":"battle",...} (seed, map, arena size, fleets) and
// closes with {"ev":"result",...}; several battles may share one file.
// Coordinates are rounded to whole world units: readable, diffable, and
// precise enough for any analysis that matters.
type battleTrace struct {
	enc  *json.Encoder
	fail error // the first write error; the trace goes quiet after reporting it
}

// snapEvery is the snapshot period in ticks: one per second of game time.
const snapEvery = 60

// traceEvent is every per-ship line in the stream; unused fields stay absent.
type traceEvent struct {
	T    int    `json:"t"`
	Ev   string `json:"ev"`
	ID   int    `json:"id,omitempty"`
	F    int    `json:"f,omitempty"`
	Kind string `json:"kind,omitempty"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	By   int    `json:"by,omitempty"`
	Dmg  int    `json:"dmg,omitempty"`
	HP   int    `json:"hp,omitempty"`
	Mv   int    `json:"mv"`
}

func newBattleTrace(w io.Writer) *battleTrace {
	if w == nil {
		return nil
	}
	return &battleTrace{enc: json.NewEncoder(w)}
}

// emit writes one line, going quiet after the first failure (a full disk must
// not take the battle down with it).
func (bt *battleTrace) emit(v any) {
	if bt == nil || bt.fail != nil {
		return
	}
	bt.fail = bt.enc.Encode(v)
	if bt.fail != nil {
		fmt.Printf("battle trace stopped: %v\n", bt.fail)
	}
}

// traceSpawn records a ship materialising.
func (g *Game) traceSpawn(e *entity) {
	if g.trace == nil {
		return
	}
	g.trace.emit(traceEvent{T: g.currentTick(), Ev: "spawn", ID: e.id, F: e.faction,
		Kind: e.kindName(), X: int(e.x), Y: int(e.y), HP: e.hp})
}

// traceShot records a bolt leaving a ship toward a point.
func (g *Game) traceShot(e *entity, tx, ty float64) {
	if g.trace == nil {
		return
	}
	g.trace.emit(traceEvent{T: g.currentTick(), Ev: "shot", ID: e.id, F: e.faction,
		X: int(tx), Y: int(ty)})
}

// traceHit and traceDeath record damage landing and a hull going down; by is
// the shooter's id (0 = the player, which skirmish does not have).
func (g *Game) traceHit(e *entity, by, dmg int) {
	if g.trace == nil {
		return
	}
	g.trace.emit(traceEvent{T: g.currentTick(), Ev: "hit", ID: e.id, F: e.faction,
		By: by, Dmg: dmg, X: int(e.x), Y: int(e.y), HP: e.hp})
}

func (g *Game) traceDeath(e *entity, by int) {
	if g.trace == nil {
		return
	}
	g.trace.emit(traceEvent{T: g.currentTick(), Ev: "death", ID: e.id, F: e.faction,
		By: by, X: int(e.x), Y: int(e.y)})
}

// traceSalvage records a ship taking something off the field; kind is what it
// took, so a trace shows which fleet lived off the battlefield.
func (g *Game) traceSalvage(e *entity, kind string) {
	if g.trace == nil {
		return
	}
	g.trace.emit(traceEvent{T: g.currentTick(), Ev: "salvage", ID: e.id, F: e.faction,
		Kind: kind, X: int(e.x), Y: int(e.y), HP: e.hp})
}

// traceSnapshot writes one "snap" line per live ship; last carries each ship's
// position at the previous snapshot so mv (distance covered since) is cheap.
func (g *Game) traceSnapshot(last map[int][2]float64) {
	if g.trace == nil {
		return
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 {
			continue
		}
		mv := 0
		if p, ok := last[e.id]; ok {
			mv = int(math.Hypot(e.x-p[0], e.y-p[1]))
		}
		last[e.id] = [2]float64{e.x, e.y}
		g.trace.emit(traceEvent{T: g.currentTick(), Ev: "snap", ID: e.id, F: e.faction,
			X: int(e.x), Y: int(e.y), HP: e.hp, Mv: mv})
	}
}
