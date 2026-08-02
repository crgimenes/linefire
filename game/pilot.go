package game

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/filo"
)

// A pilot is a Filo program driving a skirmish ship: THE way skirmish is
// played. The player writes an AI, one program per faction, and the engine
// instantiates it per ship — every hull of the faction runs its own copy with
// its own memory (the Core Wars model). Ships without a program fly the house
// brain (updateEnemies), which is also every program's safety net.
//
// THE CONTRACT — one call per ship per tick.
//
// Instruments (globals, overwritten every tick; everything the program defines
// with (def ...) persists to the next tick — that is the ship's memory):
//
//	self-x self-y        position, world units
//	self-vx self-vy      velocity, world units per tick
//	self-heading         degrees (0 = +x, grows clockwise on screen)
//	self-hull            hits left before destruction
//	self-kind            ship type: "enemy" "rusher" "sniper" "tank" ...
//	self-faction         team number (1..8)
//	self-speed           this hull's movement speed, world units per tick
//	self-radar           sensor radius, world units
//	fire-ready           #t when the gun is off cooldown
//	field-w field-h      arena size, world units
//	tick                 the engine's frame counter
//	first-tick           #t only on this ship's first run: the whole program
//	                     re-runs every tick, so unconditional (def n 0) would
//	                     reset n each time — guard memory initialisation with
//	                     (if first-tick (def n 0))
//	self-shield          salvaged shield: it absorbs damage before the hull
//	self-hull-max        what this hull was built with, so a program can tell
//	                     a scratch from a wreck and decide to go for a repair
//	allies enemies       lists of VISIBLE contacts — inside self-radar AND in
//	                     line of sight (rock hides what is behind it) — sorted
//	                     nearest first. Each contact is a list of five values:
//	                     (kind x y heading dist).
//	loot                 pickups the ship can see, same rules, nearest first:
//	                     (kind x y dist), where kind is "heal", "shield",
//	                     "fire", "rate", "damage" or a weapon key. Salvaging
//	                     one is a matter of flying over it (see salvage.go),
//	                     so a program that wants it just seeks it — and so
//	                     does the enemy program.
//
// Orders (builtins; the last call of each wins; physics stays the engine's —
// a program steers a ship, it does not teleport one):
//
//	(move dx dy)   desired direction this tick; the engine normalizes it,
//	               applies THIS hull's speed, separation from neighbours and
//	               wall sliding. Returns #t.
//	(seek x y)     navigate TOWARD a world point, riding the engine's
//	               pathfinding around rock — the same A* the house brain uses:
//	               the program says where, the engine knows how. Where (move)
//	               presses blindly into a wall, (seek) goes around it. One
//	               navigation order per tick — (seek) or (move), the last call
//	               wins. Returns #t.
//	(face deg)     aim the hull; the turn rate is the engine's. Without it the
//	               hull faces its movement. Returns #t.
//	(fire x y)     shoot at a world point. Lands only if fire-ready (the
//	               cooldown is the engine's); rock eats blind shots. Returns
//	               #t when the shot was actually fired.
//
// Math builtins (Go does the heavy lifting; Filo stays small): (dist x1 y1 x2
// y2), (bearing x1 y1 x2 y2) -> degrees from point 1 to point 2, (norm-angle
// deg) -> (-180,180], (sin deg) (cos deg) (atan2 y x) -> degrees, (sqrt v)
// (abs v) (mod a b) (floor v) (min a b) (max a b) (clamp v lo hi).
//
// Budget: pilotStepLimit evaluation steps and pilotTimeout per tick. A program
// that errors or blows its budget is reported once per faction on stderr and
// its ship falls back to the house brain FOR THAT TICK — the show goes on, and
// the next tick tries the program again.
// pilotStepLimit is the real per-tick budget: evaluation steps, deterministic
// and fair. pilotTimeout is only the backstop for a blocked run — wall-clock
// deadlines trip spuriously under a saturated CPU (the headless runner
// hammering all cores), so it is deliberately loose; the step limit is what
// actually cuts a runaway program.
const (
	pilotStepLimit = 4000
	pilotTimeout   = 20 * time.Millisecond
)

// factionAI is one faction's compiled program, shared by all its ships.
type factionAI struct {
	faction  int
	prog     *filo.Program
	errShown bool // the first script error is reported once, not 60x/second
}

// shipPilot is one ship's running instance: the shared program plus this
// hull's own memory (its globals between ticks).
type shipPilot struct {
	ai  *factionAI
	mem map[string]filo.Value
}

// pilotOrders is what one tick of a program asked for; the builtins write it
// through the context, so the shared engine needs no per-ship state. move and
// seek share the one navigation slot: the last call wins.
type pilotOrders struct {
	navX, navY   float64
	nav          string // "", "move" or "seek"
	faceDeg      float64
	hasFace      bool
	fireX, fireY float64
	hasFire      bool
	e            *entity // the ship being flown (fire-ready lives on its cooldown)
}

type pilotCtxKey struct{}

// newPilotEngine builds the Filo engine with the skirmish builtins registered.
// One engine serves every faction: programs differ, the vocabulary does not.
func newPilotEngine() *filo.Engine {
	eng := filo.NewEngine()

	order := func(name string, minArgs int, apply func(o *pilotOrders, args []filo.Value) (filo.Value, error)) {
		eng.MustRegisterBuiltin(name, func(ctx context.Context, args []filo.Value) (filo.Value, error) {
			if len(args) < minArgs {
				return filo.Value{}, fmt.Errorf("%s wants %d arguments, got %d", name, minArgs, len(args))
			}
			o, ok := ctx.Value(pilotCtxKey{}).(*pilotOrders)
			if !ok {
				return filo.Value{}, fmt.Errorf("%s: no ship on this run", name)
			}
			return apply(o, args)
		})
	}

	order("move", 2, func(o *pilotOrders, args []filo.Value) (filo.Value, error) {
		dx, err := args[0].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		dy, err := args[1].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		o.navX, o.navY, o.nav = dx, dy, "move"
		return filo.VBool(true), nil
	})
	order("seek", 2, func(o *pilotOrders, args []filo.Value) (filo.Value, error) {
		x, err := args[0].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		y, err := args[1].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		o.navX, o.navY, o.nav = x, y, "seek"
		return filo.VBool(true), nil
	})
	order("face", 1, func(o *pilotOrders, args []filo.Value) (filo.Value, error) {
		deg, err := args[0].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		o.faceDeg, o.hasFace = deg, true
		return filo.VBool(true), nil
	})
	order("fire", 2, func(o *pilotOrders, args []filo.Value) (filo.Value, error) {
		x, err := args[0].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		y, err := args[1].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		o.fireX, o.fireY, o.hasFire = x, y, true
		return filo.VBool(o.e.fireCD <= 0), nil
	})

	num := func(name string, nargs int, f func(a []float64) float64) {
		eng.MustRegisterBuiltin(name, func(_ context.Context, args []filo.Value) (filo.Value, error) {
			if len(args) != nargs {
				return filo.Value{}, fmt.Errorf("%s wants %d arguments, got %d", name, nargs, len(args))
			}
			vals := make([]float64, nargs)
			for i := range args {
				v, err := args[i].AsNumber()
				if err != nil {
					return filo.Value{}, fmt.Errorf("%s: %w", name, err)
				}
				vals[i] = v
			}
			return filo.VNum(f(vals)), nil
		})
	}

	num("dist", 4, func(a []float64) float64 { return math.Hypot(a[2]-a[0], a[3]-a[1]) })
	num("bearing", 4, func(a []float64) float64 { return math.Atan2(a[3]-a[1], a[2]-a[0]) * 180 / math.Pi })
	num("norm-angle", 1, func(a []float64) float64 { return normDeg(a[0]) })
	num("sin", 1, func(a []float64) float64 { return math.Sin(a[0] * math.Pi / 180) })
	num("cos", 1, func(a []float64) float64 { return math.Cos(a[0] * math.Pi / 180) })
	num("atan2", 2, func(a []float64) float64 { return math.Atan2(a[0], a[1]) * 180 / math.Pi })
	num("sqrt", 1, func(a []float64) float64 { return math.Sqrt(a[0]) })
	num("abs", 1, func(a []float64) float64 { return math.Abs(a[0]) })
	num("mod", 2, func(a []float64) float64 { return math.Mod(a[0], a[1]) })
	num("floor", 1, func(a []float64) float64 { return math.Floor(a[0]) })
	num("min", 2, func(a []float64) float64 { return math.Min(a[0], a[1]) })
	num("max", 2, func(a []float64) float64 { return math.Max(a[0], a[1]) })
	num("clamp", 3, func(a []float64) float64 { return math.Min(math.Max(a[0], a[1]), a[2]) })

	return eng
}

// compilePilots compiles one source per faction (1-based: sources[0] drives
// faction 1). An empty source leaves that faction on the house brain.
func compilePilots(eng *filo.Engine, sources []string) (map[int]*factionAI, error) {
	ais := map[int]*factionAI{}
	for i, src := range sources {
		if src == "" {
			continue
		}
		prog, err := eng.Compile(src)
		if err != nil {
			return nil, fmt.Errorf("faction %d program: %w", i+1, err)
		}
		ais[i+1] = &factionAI{faction: i + 1, prog: prog}
	}
	return ais, nil
}

// pilotFor hands a landing ship its faction's program, or nil for the house
// brain. A new ship is a new mind: memory starts empty.
func (g *Game) pilotFor(faction int) *shipPilot {
	ai := g.factionAIs[faction]
	if ai == nil {
		return nil
	}
	return &shipPilot{ai: ai}
}

// runPilot executes one tick of the ship's program and applies its orders.
// It reports false — fall back to the house brain — when the program errors,
// so a broken script degrades to a dumb ship instead of a dead one.
func (g *Game) runPilot(e *entity) bool {
	p := e.pilot
	orders := &pilotOrders{e: e}
	ctx := context.WithValue(context.Background(), pilotCtxKey{}, orders)

	mem := p.mem
	first := mem == nil
	if first {
		mem = map[string]filo.Value{}
	}
	g.fillInstruments(e, mem)
	mem["first-tick"] = filo.VBool(first)

	_, newMem, err := p.ai.prog.Execute(ctx, mem, filo.EvalConfig{
		StepLimit: pilotStepLimit,
		Timeout:   pilotTimeout,
	})
	if err != nil {
		if !p.ai.errShown {
			p.ai.errShown = true
			log.Printf("faction %d program: %v (its ships fall back to the house brain on error)", p.ai.faction, err)
		}
		return false
	}
	p.mem = newMem
	g.applyShipOrders(e, orders)
	return true
}

// applyShipOrders is the one gate between "what a driver asked for" and "what
// the ship does" — shared by the Filo pilots and the IPC drivers, so both fly
// under exactly the same physics: the hull's own speed, separation, wall
// sliding, the engine's turn rate and fire cooldown.
func (g *Game) applyShipOrders(e *entity, orders *pilotOrders) {
	if orders.nav != "" && !e.stationary {
		bx, by := e.x, e.y
		if orders.nav == "seek" {
			// The program says where, the engine knows how: the same A*
			// pursuit the house brain uses, stuck-recovery included, so a
			// seeking ship goes AROUND rock instead of pressing into it.
			g.pursuePath(e, orders.navX, orders.navY)
		} else {
			g.applyEnemyMove(e, orders.navX, orders.navY, e.moveSpeed())
		}
		moved := math.Hypot(e.x-bx, e.y-by)
		if !orders.hasFace && moved > 0 {
			e.angle = turnToward(e.angle, math.Atan2(e.y-by, e.x-bx)*180/math.Pi, enemyTurnRate)
		}
		if orders.nav == "seek" {
			if moved < stuckEps {
				e.stuck++
				if e.stuck >= stuckLimit {
					g.unstick(e, orders.navX, orders.navY)
					e.stuck = 0
				}
			} else {
				e.stuck = 0
			}
		}
	}
	if orders.hasFace {
		e.angle = turnToward(e.angle, orders.faceDeg, enemyTurnRate)
	}
	if orders.hasFire && e.fireCD <= 0 {
		e.fireCD = e.fireInterval()
		g.enemyFire(e, orders.fireX, orders.fireY)
	}
}

// fillInstruments writes this tick's readings into the ship's globals. The
// program's own defs in the same map are left alone: that is its memory.
func (g *Game) fillInstruments(e *entity, mem map[string]filo.Value) {
	mem["self-x"] = filo.VNum(e.x)
	mem["self-y"] = filo.VNum(e.y)
	mem["self-vx"] = filo.VNum(e.vx)
	mem["self-vy"] = filo.VNum(e.vy)
	mem["self-heading"] = filo.VNum(e.angle)
	mem["self-hull"] = filo.VNum(float64(e.hp))
	mem["self-kind"] = filo.VString(e.kindName())
	mem["self-faction"] = filo.VNum(float64(e.faction))
	mem["self-speed"] = filo.VNum(e.moveSpeed())
	mem["self-radar"] = filo.VNum(e.detectRange())
	mem["self-shield"] = filo.VNum(float64(e.shield))
	mem["self-hull-max"] = filo.VNum(float64(e.hullMax()))
	mem["fire-ready"] = filo.VBool(e.fireCD <= 0)
	mem["field-w"] = filo.VNum(g.bounds.w())
	mem["field-h"] = filo.VNum(g.bounds.h())
	mem["tick"] = filo.VNum(float64(g.currentTick()))

	allies, enemies := g.sensorContacts(e)
	mem["allies"] = filo.VList(contactsToFilo(allies))
	mem["enemies"] = filo.VList(contactsToFilo(enemies))
	mem["loot"] = filo.VList(lootToFilo(g.lootContacts(e)))
}

// lootToFilo encodes visible pickups as four-slot lists: a canister has no
// heading to report, so it carries one field fewer than a ship.
func lootToFilo(cs []contact) []filo.Value {
	out := make([]filo.Value, 0, len(cs))
	for _, c := range cs {
		out = append(out, filo.VList([]filo.Value{
			filo.VString(c.kind),
			filo.VNum(c.x),
			filo.VNum(c.y),
			filo.VNum(c.dist),
		}))
	}
	return out
}

// contactsToFilo encodes contacts as the contract's five-slot lists.
func contactsToFilo(cs []contact) []filo.Value {
	out := make([]filo.Value, 0, len(cs))
	for _, c := range cs {
		out = append(out, filo.VList([]filo.Value{
			filo.VString(c.kind),
			filo.VNum(c.x),
			filo.VNum(c.y),
			filo.VNum(c.heading),
			filo.VNum(c.dist),
		}))
	}
	return out
}

// currentTick is the simulation clock: the headless runner's own counter when
// one is running (ebiten's never advances without a window), the display's
// otherwise.
func (g *Game) currentTick() int {
	if g.simTick > 0 {
		return g.simTick
	}
	return int(ebiten.Tick())
}

// kindName is the ship's archetype name, empty-safe for asset-less markers.
func (e *entity) kindName() string {
	if e.a == nil {
		return ""
	}
	return e.a.Kind
}

// contact is one sighted ship, in the neutral form both encodings share: the
// Filo lists and the IPC wire carry these same five fields in this order.
type contact struct {
	kind    string
	x, y    float64
	heading float64
	dist    float64
}

// sensorContacts is what this ship can SEE: every live ship inside its radar
// with a clear line of sight — rock hides what is behind it, which is the
// whole point of fighting in a maze. Sorted nearest first, so (head enemies)
// is always the closest threat.
func (g *Game) sensorContacts(e *entity) (allies, enemies []contact) {
	type sighted struct {
		c    contact
		ally bool
	}
	var seen []sighted
	radar := e.detectRange()
	for i := range g.entities {
		o := &g.entities[i]
		if o == e || o.kind != kindEnemy || o.hp <= 0 {
			continue
		}
		d := math.Hypot(o.x-e.x, o.y-e.y)
		if d > radar || !g.lineOfSight(e.x, e.y, o.x, o.y) {
			continue
		}
		seen = append(seen, sighted{
			c:    contact{kind: o.kindName(), x: o.x, y: o.y, heading: o.angle, dist: d},
			ally: o.faction == e.faction,
		})
	}
	sort.Slice(seen, func(i, j int) bool { return seen[i].c.dist < seen[j].c.dist })
	for _, s := range seen {
		if s.ally {
			allies = append(allies, s.c)
		} else {
			enemies = append(enemies, s.c)
		}
	}
	return allies, enemies
}

// lootContacts is the pickups this ship can see, under the same rules its
// sensors use for ships: inside the radar and not behind rock, nearest first.
// Loot a program cannot see is loot it cannot race anyone for.
func (g *Game) lootContacts(e *entity) []contact {
	var seen []contact
	radar := e.detectRange()
	for i := range g.entities {
		o := &g.entities[i]
		if o.kind != kindPowerUp && o.kind != kindWeapon {
			continue
		}
		d := math.Hypot(o.x-e.x, o.y-e.y)
		if d > radar || !g.lineOfSight(e.x, e.y, o.x, o.y) {
			continue
		}
		seen = append(seen, contact{kind: o.power, x: o.x, y: o.y, dist: d})
	}
	sort.Slice(seen, func(i, j int) bool { return seen[i].dist < seen[j].dist })
	return seen
}
