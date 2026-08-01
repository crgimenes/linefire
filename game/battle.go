package game

import (
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/procgen"
)

// RunBattle fights one skirmish without a window: the ships are placed up
// front, there are no reinforcements, and the simulation steps at full CPU
// speed until one faction stands alone or the tick budget runs out. Hundreds
// of battles fit in a few seconds of processing, which is what makes program
// development honest — a change to an AI is measured over many fights, not
// eyeballed over one. The graphical skirmish and this runner step the same
// code: updateEnemies, the Filo pilots, the shots, the damage.
//
// A battle is DETERMINISTIC: placement, per-hull temperament and every patrol
// turn draw from the seed's own dice (simRand), so the same options replay the
// same fight byte for byte — a trace is a reproducible artifact, a regression
// can be bisected by seed, and a player-vs-player dispute has a verifiable
// record.
type BattleOptions struct {
	Programs []string  // one Filo source per faction, Programs[0] = faction 1 (empty = house brain)
	Factions int       // teams, clamped to 2..maxFactions (0 = 2)
	Ships    int       // hulls per faction (0 = 8)
	Map      string    // SkirmishMapArena (default) or SkirmishMapMaze
	Seed     int64     // arena generation and placement seed
	MaxTicks int       // battle length cap in ticks (0 = 7200: two minutes of game time)
	Trace    io.Writer // battle trace destination (JSONL; see trace.go) — nil = no trace
	Battle   int       // battle number stamped on the trace header, for multi-battle files
}

// BattleResult is how one fight ended.
type BattleResult struct {
	Winner int         // the faction left standing; 0 = draw (timeout, or mutual destruction)
	Ticks  int         // how long the fight ran
	Alive  map[int]int // live hulls per faction at the end
}

const (
	battleDefaultShips = 8
	battleDefaultTicks = 7200
	battleArenaW       = 2400.0
	battleArenaH       = 1600.0
)

// battleKinds is the mix of hulls each faction fields, dealt in order so every
// faction gets the same fleet.
var battleKinds = []string{"enemy", "rusher", "sniper", "tank"}

// RunBattle builds the battlefield, lands the fleets and fights to the end.
func RunBattle(content fs.FS, mapDir string, opts BattleOptions) (BattleResult, error) {
	player, err := filoio.LoadAssetFS(content, mapDir, "player")
	if err != nil {
		return BattleResult{}, err
	}
	ships := opts.Ships
	if ships <= 0 {
		ships = battleDefaultShips
	}
	maxTicks := opts.MaxTicks
	if maxTicks <= 0 {
		maxTicks = battleDefaultTicks
	}

	var lvl *level.Level
	switch opts.Map {
	case SkirmishMapMaze:
		lvl = procgen.GenCaveArena(opts.Seed, battleArenaW, battleArenaH)
	case "", SkirmishMapArena:
		lvl = procgen.GenArena(opts.Seed, battleArenaW, battleArenaH)
	default:
		return BattleResult{}, fmt.Errorf("unknown battle map %q (want %q or %q)", opts.Map, SkirmishMapArena, SkirmishMapMaze)
	}

	g := newWithContent(content, player, lvl, mapDir, false)
	g.skirmishMode = true
	g.arenaCam = true
	g.factions = min(max(opts.Factions, 2), maxFactions)
	// #nosec G404 -- deterministic battle simulation, not a security boundary
	g.simRand = rand.New(rand.NewPCG(uint64(opts.Seed), 0x53494d)) // stream = "SIM"
	g.trace = newBattleTrace(opts.Trace)
	if len(opts.Programs) > 0 {
		g.filoEng = newPilotEngine()
		g.factionAIs, err = compilePilots(g.filoEng, opts.Programs)
		if err != nil {
			return BattleResult{}, err
		}
	}
	// The horde is built for its asset cache only — it is never stepped, so a
	// battle has no reinforcements: the fleets that land are the fleets there are.
	g.horde = newHorde(level.Horde{Types: battleKinds, Seed: opts.Seed})

	g.trace.emit(map[string]any{
		"ev": "battle", "n": opts.Battle, "seed": opts.Seed,
		"map": lvl.Title, "w": int(lvl.Size.W), "h": int(lvl.Size.H),
		"factions": g.factions, "ships": ships,
	})

	// Land the fleets through the same vortex machinery the screen uses (which
	// deals the factions round-robin and keeps arrivals off walls and off each
	// other), just without waiting for the show.
	// #nosec G404 -- deterministic battle placement, not a security boundary
	rng := rand.New(rand.NewPCG(uint64(opts.Seed), 0x424154544c45)) // stream = "BATTLE"
	for i := 0; i < ships*g.factions; i++ {
		g.spawnArrival(battleKinds[i%len(battleKinds)], rng)
	}
	for range materialiseTicks + 1 {
		g.stepArrivals()
	}

	last := map[int][2]float64{} // per-ship position at the previous snapshot
	res := BattleResult{Winner: 0, Ticks: maxTicks}
	for tick := 1; tick <= maxTicks; tick++ {
		g.simTick = tick
		g.updateEnemies()
		g.stepEnemyShots()
		if tick%snapEvery == 0 {
			g.traceSnapshot(last)
		}
		if winner, over := g.battleOver(); over {
			res = BattleResult{Winner: winner, Ticks: tick}
			break
		}
	}
	res.Alive = g.aliveByFaction()
	g.trace.emit(map[string]any{
		"ev": "result", "winner": res.Winner, "ticks": res.Ticks, "alive": res.Alive,
	})
	return res, nil
}

// aliveByFaction counts the live hulls per team.
func (g *Game) aliveByFaction() map[int]int {
	alive := map[int]int{}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindEnemy && e.hp > 0 {
			alive[e.faction]++
		}
	}
	return alive
}

// battleOver reports the end of the fight: at most one faction still has
// ships. The winner is that faction, or 0 when nobody survived.
func (g *Game) battleOver() (winner int, over bool) {
	alive := g.aliveByFaction()
	if len(alive) > 1 {
		return 0, false
	}
	for f := range alive {
		return f, true
	}
	return 0, true
}
