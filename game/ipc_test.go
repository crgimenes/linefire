package game

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// ipcGame is one ship (id 1, faction 1) driven over in-memory pipes, plus the
// test's ends of both streams.
func ipcGame(t *testing.T) (g *Game, orders io.WriteCloser, state *bufio.Reader) {
	t.Helper()
	orderR, orderW := io.Pipe()
	stateR, stateW := io.Pipe()
	d := NewIPCDriver(orderR, stateW)
	d.faction = 1

	g = &Game{}
	g.skirmishMode = true
	g.bounds = bounds{minX: 0, minY: 0, maxX: 1000, maxY: 1000}
	g.ipcDrivers = map[int]*IPCDriver{1: d}
	g.entities = []entity{{kind: kindEnemy, id: 1, x: 500, y: 500, radius: 8, hp: 3, faction: 1, radar: 400}}
	return g, orderW, bufio.NewReader(stateR)
}

// waitStanding polls until the reader goroutine has digested the order.
func waitStanding(t *testing.T, d *IPCDriver, id int) {
	t.Helper()
	for range 200 {
		if d.standing(id) != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the order never arrived")
}

// A standing order flies the ship tick after tick — the driver wrote ONCE and
// the ship keeps moving and firing at the engine's own cadence.
func TestIPCStandingOrderFliesTheShip(t *testing.T) {
	g, orders, _ := ipcGame(t)
	d := g.ipcDrivers[1]

	if _, err := orders.Write([]byte(`{"orders":[{"id":1,"move":[1,0],"fire":[900,500]}]}` + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitStanding(t, d, 1)

	for range 10 {
		g.updateEnemies()
	}
	e := &g.entities[0]
	if e.x <= 500 {
		t.Fatalf("ship ordered east stayed at x=%v", e.x)
	}
	if len(g.enemyShots) != 1 {
		t.Fatalf("fire-at-will made %d shots in 10 ticks; the cooldown allows exactly one", len(g.enemyShots))
	}
	if g.enemyShots[0].faction != 1 || g.enemyShots[0].vx <= 0 {
		t.Fatal("the bolt should be faction 1's, flying east")
	}

	// clear drops the standing order: the ship goes back to the house brain.
	if _, err := orders.Write([]byte(`{"orders":[{"id":1,"clear":true}]}` + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	for range 200 {
		if d.standing(1) == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("clear never landed")
}

// The state feed carries the whole contract in JSON: the ship, its
// instruments, and only what its sensors can SEE.
func TestIPCStateFeedSpeaksTheContract(t *testing.T) {
	g, orders, state := ipcGame(t)
	defer func() { _ = orders.Close() }()
	g.entities = append(g.entities,
		entity{kind: kindEnemy, id: 2, x: 700, y: 500, radius: 8, hp: 3, faction: 2}, // visible foe
		entity{kind: kindEnemy, id: 3, x: 500, y: 950, radius: 8, hp: 3, faction: 2}, // beyond the radar
	)

	g.simTick = ipcSendEvery // on cadence
	go g.stepIPC()
	line, err := state.ReadString('\n')
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var msg ipcStateMsg
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("bad state line: %v", err)
	}
	if msg.Field.W != 1000 || msg.Field.H != 1000 {
		t.Fatalf("field = %+v", msg.Field)
	}
	if len(msg.Ships) != 1 || msg.Ships[0].ID != 1 {
		t.Fatalf("the feed should carry faction 1's one ship, got %+v", msg.Ships)
	}
	s := msg.Ships[0]
	if len(s.Enemies) != 1 || s.Enemies[0].Dist != 200 {
		t.Fatalf("sensors should see exactly the in-range foe at dist 200, got %+v", s.Enemies)
	}
	if !s.FireReady || s.Hull != 3 {
		t.Fatalf("instruments wrong: %+v", s)
	}
}

// ships is ALWAYS an array, never null: a faction between fleets must not
// crash every client that trusts its own contract (the first live bot died
// exactly this way).
func TestIPCStateShipsIsNeverNull(t *testing.T) {
	g, orders, state := ipcGame(t)
	defer func() { _ = orders.Close() }()
	g.entities = nil // the fleet is between spawns
	g.simTick = ipcSendEvery
	go g.stepIPC()
	line, err := state.ReadString('\n')
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(line, `"ships":[]`) {
		t.Fatalf("a fleetless faction must still get an array: %s", line)
	}
}

// A driver that closes its stream is reported and its ships fall back to the
// house brain — a dead process makes dumb ships, never dead ones.
func TestIPCDeadDriverFallsBack(t *testing.T) {
	g, orders, _ := ipcGame(t)
	d := g.ipcDrivers[1]

	_ = orders.Close()
	for range 200 {
		d.mu.Lock()
		dead := d.dead
		d.mu.Unlock()
		if dead {
			break
		}
		time.Sleep(time.Millisecond)
	}

	bx := g.entities[0].x
	for range 30 {
		g.updateEnemies() // must not panic, must patrol
	}
	if g.entities[0].x == bx && g.entities[0].y == 500 {
		t.Fatal("the abandoned ship should at least patrol")
	}
}
