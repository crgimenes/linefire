package game

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"sync"
)

// IPC control: anything that can read lines and write lines — a program in
// any language, a person with a terminal, an LLM agent — flies a faction from
// OUTSIDE the process, under the same contract the Filo pilots use. This is
// the project's founding promise ("AI-friendly from birth") crossing the
// process boundary.
//
// THE PROTOCOL — JSON Lines both ways.
//
// Engine -> driver, every ipcSendEvery ticks, one line with the faction's
// live ships and everything their instruments see (the pilot.go contract in
// JSON — contacts included, occlusion included):
//
//	{"tick":420,"field":{"w":2400,"h":1600},"ships":[
//	  {"id":7,"kind":"tank","x":812,"y":400,"vx":0.4,"vy":-1.1,
//	   "heading":-12.5,"hull":9,"hullMax":10,"shield":3,
//	   "speed":1.1,"radar":320,"fireReady":true,
//	   "allies":[{"kind":"enemy","x":700,"y":380,"heading":10,"dist":114}],
//	   "enemies":[...],"loot":[{"kind":"shield","x":900,"y":420,"dist":90}]}]}
//
// Driver -> engine, whenever it wants — the engine NEVER waits for it. Each
// order is STANDING: it holds until replaced or cleared, which is what lets a
// slow driver (an LLM thinking for seconds) fly ships that act every tick:
//
//	{"orders":[{"id":7,"seek":[1200,800],"fire":[900,650]}]}
//	{"orders":[{"id":7,"move":[1,0],"face":90}]}
//	{"orders":[{"id":7,"clear":true}]}
//
// seek/move take the tick's one navigation slot (seek wins when both are in
// one order); face aims the hull; fire is fire-at-will toward a point — the
// ship shoots every time its cooldown allows while the order stands. A ship
// with no standing order patrols like the house brain's idle ships.
//
// A driver that closes its stream or writes a broken line is reported once
// and its faction falls back to the house brain — the same grace the Filo
// pilots get: a dead process makes dumb ships, never dead ones.
const ipcSendEvery = 6 // ticks between state lines: 10 Hz at game speed

// IPCDriver is one faction's external pilot. The caller owns the transport
// (usually a child process's stdio) and hands in its reader and writer; the
// game does no exec of its own.
type IPCDriver struct {
	mu      sync.Mutex
	orders  map[int]*pilotOrders // standing orders per ship id
	dead    bool                 // the stream ended: the faction is on the house brain
	deadWhy string               // what happened, for the one-time report
	shown   bool                 // the death is reported once

	enc     *json.Encoder
	faction int
}

// ipcOrderMsg is one driver line; ipcOrder is one ship's standing order.
type ipcOrderMsg struct {
	Orders []ipcOrder `json:"orders"`
}

type ipcOrder struct {
	ID    int       `json:"id"`
	Seek  []float64 `json:"seek"`
	Move  []float64 `json:"move"`
	Face  *float64  `json:"face"`
	Fire  []float64 `json:"fire"`
	Clear bool      `json:"clear"`
}

// ipcShipState and friends are the engine->driver line.
type ipcContact struct {
	Kind    string  `json:"kind"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Heading float64 `json:"heading"`
	Dist    float64 `json:"dist"`
}

type ipcShipState struct {
	ID        int          `json:"id"`
	Kind      string       `json:"kind"`
	X         float64      `json:"x"`
	Y         float64      `json:"y"`
	VX        float64      `json:"vx"`
	VY        float64      `json:"vy"`
	Heading   float64      `json:"heading"`
	Hull      int          `json:"hull"`
	HullMax   int          `json:"hullMax"`
	Shield    int          `json:"shield"`
	Weapon    string       `json:"weapon"` // "" = the hull's own bolt; see shiparms.go
	Speed     float64      `json:"speed"`
	Radar     float64      `json:"radar"`
	FireReady bool         `json:"fireReady"`
	Allies    []ipcContact `json:"allies"`
	Enemies   []ipcContact `json:"enemies"`
	Loot      []ipcContact `json:"loot"` // pickups in sight; Heading is meaningless for these
}

type ipcStateMsg struct {
	Tick  int `json:"tick"`
	Field struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	} `json:"field"`
	Ships []ipcShipState `json:"ships"`
}

// NewIPCDriver wires a driver to its transport and starts reading its orders.
// The reader goroutine lives until r closes; the writer never blocks the game
// beyond the OS pipe buffer (a driver that stops reading eventually stalls
// its own state feed, not the battle — see send).
func NewIPCDriver(r io.Reader, w io.Writer) *IPCDriver {
	d := &IPCDriver{
		orders: map[int]*pilotOrders{},
		enc:    json.NewEncoder(w),
	}
	go d.readOrders(r)
	return d
}

// readOrders consumes the driver's lines until the stream ends; each order
// replaces the ship's standing one whole.
func (d *IPCDriver) readOrders(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg ipcOrderMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			d.fail("sent a broken line: " + err.Error())
			return
		}
		d.mu.Lock()
		for _, o := range msg.Orders {
			if o.Clear {
				delete(d.orders, o.ID)
				continue
			}
			d.orders[o.ID] = ipcToShipOrders(o)
		}
		d.mu.Unlock()
	}
	d.fail("closed its stream")
}

// ipcToShipOrders translates one wire order into the shared order form.
func ipcToShipOrders(o ipcOrder) *pilotOrders {
	so := &pilotOrders{}
	if len(o.Move) == 2 {
		so.nav, so.navX, so.navY = "move", o.Move[0], o.Move[1]
	}
	if len(o.Seek) == 2 {
		so.nav, so.navX, so.navY = "seek", o.Seek[0], o.Seek[1]
	}
	if o.Face != nil {
		so.hasFace, so.faceDeg = true, *o.Face
	}
	if len(o.Fire) == 2 {
		so.hasFire, so.fireX, so.fireY = true, o.Fire[0], o.Fire[1]
	}
	return so
}

// fail marks the driver dead; the report happens on the game loop (runIPCShip)
// so it lands once and next to the fallback it announces.
func (d *IPCDriver) fail(why string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.dead {
		d.dead = true
		d.deadWhy = why
	}
}

// standing returns the ship's current order, or nil.
func (d *IPCDriver) standing(id int) *pilotOrders {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dead {
		return nil
	}
	return d.orders[id]
}

// runIPCShip applies the ship's standing order, reporting whether the driver
// is flying it. No order yet = not flying: the house brain's idle patrol keeps
// the ship alive-looking until the driver speaks.
func (g *Game) runIPCShip(d *IPCDriver, e *entity) bool {
	d.mu.Lock()
	dead, why, shown := d.dead, d.deadWhy, d.shown
	if dead {
		d.shown = true
	}
	d.mu.Unlock()
	if dead {
		if !shown {
			log.Printf("faction %d driver %s (its ships fall back to the house brain)", d.faction, why)
		}
		return false
	}
	o := d.standing(e.id)
	if o == nil {
		return false // nothing ordered yet: the house brain fills the silence
	}
	orders := *o // the standing order is shared state; fire mutates cooldowns via e, not via the order
	g.applyShipOrders(e, &orders)
	return true
}

// stepIPC feeds every driver its faction's state at ipcSendEvery cadence.
func (g *Game) stepIPC() {
	if len(g.ipcDrivers) == 0 || g.currentTick()%ipcSendEvery != 0 {
		return
	}
	for faction, d := range g.ipcDrivers {
		if d == nil {
			continue
		}
		d.mu.Lock()
		dead := d.dead
		d.mu.Unlock()
		if dead {
			continue
		}
		// ships is ALWAYS an array, never null — a faction between fleets
		// (all dead, reinforcements still in their vortices) must not make
		// every client defensive about its own contract.
		msg := ipcStateMsg{Tick: g.currentTick(), Ships: []ipcShipState{}}
		msg.Field.W, msg.Field.H = g.bounds.w(), g.bounds.h()
		for i := range g.entities {
			e := &g.entities[i]
			if e.kind != kindEnemy || e.hp <= 0 || e.faction != faction {
				continue
			}
			msg.Ships = append(msg.Ships, g.ipcShipState(e))
		}
		if err := d.enc.Encode(msg); err != nil {
			d.fail("stopped reading: " + err.Error())
		}
	}
}

// ipcShipState is one ship's instruments in wire form.
func (g *Game) ipcShipState(e *entity) ipcShipState {
	allies, enemies := g.sensorContacts(e)
	return ipcShipState{
		ID: e.id, Kind: e.kindName(),
		X: e.x, Y: e.y, VX: e.vx, VY: e.vy,
		Heading: e.angle, Hull: e.hp, HullMax: e.hullMax(), Shield: e.shield, Weapon: e.weaponKey,
		Speed: e.moveSpeed(), Radar: e.detectRange(),
		FireReady: e.fireCD <= 0,
		Allies:    ipcContacts(allies),
		Enemies:   ipcContacts(enemies),
		Loot:      ipcContacts(g.lootContacts(e)),
	}
}

// ipcContacts encodes contacts in wire form: the same five fields, the same
// order as the Filo lists — one contract, two encodings.
func ipcContacts(cs []contact) []ipcContact {
	out := make([]ipcContact, 0, len(cs))
	for _, c := range cs {
		out = append(out, ipcContact{Kind: c.kind, X: c.x, Y: c.y, Heading: c.heading, Dist: c.dist})
	}
	return out
}
