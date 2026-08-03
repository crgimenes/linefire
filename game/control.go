package game

import (
	"log"
	"sync"
)

// The control plane: how the OUTSIDE talks to a running skirmish. One surface
// serves them all — a terminal, the garage UI, an agent: battle events flow
// OUT as JSONL (the same stream the headless trace writes: spawns, shots,
// hits, deaths, battle staged, battle decided), and match commands flow IN.
//
// Commands, one JSON object per line:
//
//	{"op":"restart"}                                  stage the battle again
//	{"op":"match","mode":"timed","ships":6,"duration":120}   reconfigure and stage
//	{"op":"quit"}                                     end the battle and exit
//
// Anything invalid is reported and ignored — the show goes on.
//
// Quitting used to be left to the transport owner, on the reasoning that
// ending a process it started is a launcher's own business. crg turned that
// around: there is already a channel here, so ASKING costs nothing, while a
// signal costs portability — the caller ends up writing per-platform process
// handling for something the protocol says in seven bytes. Asking is also the
// better of the two, because a game that is asked gets to end on its own
// terms instead of being cut down mid-frame.
type MatchCommand struct {
	Op       string `json:"op"`
	Mode     string `json:"mode,omitempty"`
	Ships    int    `json:"ships,omitempty"`
	Duration int    `json:"duration,omitempty"` // seconds
}

// controlInbox is the mailbox between the transport goroutine and the game
// loop. It is a shared POINTER on Game so it survives the wholesale *g swap
// of an arena rebuild while another goroutine posts into it.
type controlInbox struct {
	mu  sync.Mutex
	cmd *MatchCommand // the pending command; the last one posted wins
	// quit lives HERE, and not on Game, for the same reason the inbox does: an
	// arena rebuild replaces *g wholesale, and a flag it forgot to carry over
	// would be a quit that silently never happened. Behind this pointer it
	// cannot be dropped.
	quit bool
}

// PostCommand queues a control command for the game loop. Safe to call from
// any goroutine; a nil inbox (the campaign, or no control plane) ignores it.
func (g *Game) PostCommand(c MatchCommand) {
	if g.ctrl == nil {
		return
	}
	g.ctrl.mu.Lock()
	g.ctrl.cmd = &c
	g.ctrl.mu.Unlock()
}

// consumeCommand applies the pending command, on the game loop.
func (g *Game) consumeCommand() {
	if g.ctrl == nil {
		return
	}
	g.ctrl.mu.Lock()
	cmd := g.ctrl.cmd
	g.ctrl.cmd = nil
	g.ctrl.mu.Unlock()
	if cmd == nil {
		return
	}

	switch cmd.Op {
	case "restart":
		g.startMatch()
	case "quit":
		g.ctrl.mu.Lock()
		g.ctrl.quit = true
		g.ctrl.mu.Unlock()
	case "match":
		m, err := newMatch(cmd.Mode, cmd.Ships, cmd.Duration*60, g.factions)
		if err != nil {
			log.Printf("control: %v (command ignored)", err)
			return
		}
		g.match = m
		g.startMatch()
	default:
		log.Printf("control: unknown op %q (command ignored)", cmd.Op)
	}
}

// quitRequested reports whether the control plane asked the game to end. Only
// the game loop may actually end it — Update returns ebiten.Termination — so
// the command sets a flag and the loop reads it on the next tick.
func (g *Game) quitRequested() bool {
	if g.ctrl == nil {
		return false
	}
	g.ctrl.mu.Lock()
	defer g.ctrl.mu.Unlock()
	return g.ctrl.quit
}

// emitBattleEvent announces the battle now staged: the garage's cue to reset
// its scoreboard. Same stream and register as the headless runner's header.
func (g *Game) emitBattleEvent() {
	if g.trace == nil {
		return
	}
	g.trace.emit(map[string]any{
		"ev": "battle", "mode": g.match.Mode, "map": g.level.Title,
		"w": int(g.level.Size.W), "h": int(g.level.Size.H),
		"factions": g.factions, "ships": g.match.Ships, "seed": g.arenaSeed,
	})
}

// emitResultEvent reports the decided battle with its full scoreboard.
func (g *Game) emitResultEvent() {
	if g.trace == nil {
		return
	}
	winner, _ := g.match.Over()
	g.trace.emit(map[string]any{
		"ev": "result", "winner": winner, "ticks": g.match.tick,
		"alive": g.aliveByFaction(), "stats": g.match.Stats,
	})
}
