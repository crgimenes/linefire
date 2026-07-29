package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// TestDiscoveryDoesNotSeeThroughWalls is the fix for fog clearing across the thin walls of
// Scramble: discovery is line-of-sight only, so a cell on the far side of a wall stays fogged
// even when it is close (the old near-radius reveal punched through walls).
func TestDiscoveryDoesNotSeeThroughWalls(t *testing.T) {
	g := &Game{}
	g.bounds = bounds{minX: 0, minY: 0, maxX: 400, maxY: 200}
	g.segs = []segment{{ax: 200, ay: 0, bx: 200, by: 200}} // a wall at x=200
	g.x, g.y = 150, 100                                    // ship just left of it
	g.disc = newDiscovery(g.bounds)

	g.updateDiscovery()

	seenAt := func(wx, wy float64) bool {
		cx, cy := g.disc.cellAt(wx, wy)
		return g.disc.seen[cy*g.disc.cols+cx]
	}
	if !seenAt(160, 100) {
		t.Fatal("a nearby cell on the ship's side should be discovered")
	}
	if seenAt(250, 100) {
		t.Fatal("a nearby cell across the wall (no line of sight) must stay fogged")
	}
}

func TestRayHitsSegment(t *testing.T) {
	wall := segment{ax: 5, ay: -1, bx: 5, by: 1} // vertical at x=5

	d, ok := rayHitsSegment(0, 0, 1, 0, wall)
	if !ok || math.Abs(d-5) > 1e-9 {
		t.Fatalf("ray +x should hit the wall at d=5, got d=%v ok=%v", d, ok)
	}
	_, ok = rayHitsSegment(0, 0, -1, 0, wall)
	if ok {
		t.Fatal("ray -x should miss a wall on the +x side")
	}
	_, ok = rayHitsSegment(0, 0, 0, 1, wall)
	if ok {
		t.Fatal("ray +y should miss the short vertical wall")
	}
}

func TestVisibilityClippedByWall(t *testing.T) {
	const r = 360.0
	g := &Game{segs: []segment{{ax: 10, ay: -100, bx: 10, by: 100}}} // wall at x=10
	poly := g.visibilityPolygon(0, 0, r)
	if len(poly) < 3 {
		t.Fatalf("expected a polygon, got %d points", len(poly))
	}
	for _, p := range poly {
		if math.Hypot(p.x, p.y) > r+1e-6 {
			t.Fatalf("vertex beyond the radius: %v", p)
		}
		if math.Abs(p.y) < 5 && p.x > 11 {
			t.Fatalf("vision leaked past the wall on the +x side: %v", p)
		}
	}
}

func TestVisionClearsToWalls(t *testing.T) {
	lvl := level.New()
	lvl.Size = asset.Size{W: 1600, H: 1600}
	lvl.Walls[0].Paths = []asset.Path{
		{Commands: []asset.Command{{Op: asset.OpMoveTo, X: 200, Y: 0}, {Op: asset.OpLineTo, X: 200, Y: 1600}}}, // visible
		{Commands: []asset.Command{{Op: asset.OpMoveTo, X: 800, Y: 0}, {Op: asset.OpLineTo, X: 800, Y: 1600}}}, // hidden behind the first
	}
	g := &Game{level: lvl, segs: wallSegments(lvl)}
	g.disc = newDiscovery(mapBounds(lvl, g.segs))

	g.x, g.y = 100, 800 // left of the wall at x=200
	g.updateDiscovery()

	seen := func(wx, wy float64) bool {
		cx, cy := g.disc.cellAt(wx, wy)
		return g.disc.seen[cy*g.disc.cols+cx]
	}
	if !seen(140, 800) {
		t.Fatal("a cell in line of sight should be cleared")
	}
	if !seen(100, 100) { // far but open (no wall in the way) — vision has no distance limit
		t.Fatal("a distant open cell in line of sight should be cleared")
	}
	if seen(300, 800) { // behind the wall: vision is blocked
		t.Fatal("a cell behind the wall must not be cleared (vision is blocked)")
	}
	// Maps only show walls the ship actually saw: the first wall is revealed; the
	// second (hidden behind it) produces no span.
	if len(g.seenSpans(g.segs[0])) == 0 {
		t.Fatal("a visible wall should produce a revealed span")
	}
	if len(g.seenSpans(g.segs[1])) != 0 {
		t.Fatal("a wall hidden behind another must not be revealed on the maps")
	}
}

func TestDiscoveredAt(t *testing.T) {
	d := newDiscovery(bounds{0, 0, 200, 200})
	if d == nil {
		t.Fatal("expected a discovery grid")
	}

	if d.discoveredAt(50, 50) {
		t.Fatal("nothing has been seen yet")
	}
	d.markSeen(d.cellAt(50, 50))
	if !d.discoveredAt(50, 50) {
		t.Fatal("a marked cell should read discovered")
	}
	if d.discoveredAt(-100, -100) {
		t.Fatal("out-of-bounds is not discovered")
	}
}

func TestVisibilityOpenIsFullRadius(t *testing.T) {
	const r = 360.0
	g := &Game{} // no walls
	poly := g.visibilityPolygon(0, 0, r)
	if len(poly) < visRays {
		t.Fatalf("expected at least %d points with no walls, got %d", visRays, len(poly))
	}
	for _, p := range poly {
		d := math.Hypot(p.x, p.y)
		if math.Abs(d-r) > 1e-6 {
			t.Fatalf("open vision should reach the given radius, got d=%v", d)
		}
	}
}

// TestDiscoveryBoundedToReach: the fog scan now reveals only cells within the on-screen reach, so
// a far cell with a perfectly clear line of sight is NOT discovered until the ship approaches —
// this is what keeps the per-frame cost bounded by screen size, not map size.
func TestDiscoveryBoundedToReach(t *testing.T) {
	lvl := boxLevel(4000, 4000, nil) // big open box: clear LOS everywhere
	segs := wallSegments(lvl)
	b := mapBounds(lvl, segs)
	g := &Game{disc: newDiscovery(b), bounds: b, segs: segs}
	g.x, g.y = (b.minX+b.maxX)/2, (b.minY+b.maxY)/2

	g.updateDiscovery()

	d := g.disc
	seen := func(wx, wy float64) bool {
		cx, cy := int((wx-d.originX)/d.cell), int((wy-d.originY)/d.cell)
		if cx < 0 || cy < 0 || cx >= d.cols || cy >= d.rows {
			return false
		}
		return d.seen[cy*d.cols+cx]
	}
	if !seen(g.x, g.y) {
		t.Fatal("the ship's own cell should be discovered")
	}
	if seen(g.x+g.discoveryReach()*1.5, g.y) {
		t.Fatal("a cell well beyond the on-screen reach (clear LOS) must not be discovered yet")
	}
}

// TestVisibilityOpensThroughDugRock guards the playtest bug: destroying a wall
// did NOT open the fog — the sight rays tested the authored segments, which
// outlive the rock they drew. On a flood map the rays march the region grid, so
// a hole carved through a wall lets a cone of vision through it (and a revisited
// map's restored tunnels clear on the first stamp).
func TestVisibilityOpensThroughDugRock(t *testing.T) {
	// A box arena split by a vertical wall through the middle.
	wall := []asset.Command{
		{Op: asset.OpMoveTo, X: 200, Y: 20},
		{Op: asset.OpLineTo, X: 200, Y: 380},
	}
	lvl := boxLevel(400, 400, wall)
	segs := wallSegments(lvl)
	g := &Game{segs: segs, bounds: mapBounds(lvl, segs)}
	g.flood = buildFloodmap(segs, 100, 200, g.bounds) // seeded on the LEFT side
	if g.flood == nil {
		t.Fatal("expected a floodmap")
	}

	crossed := func(poly []vec2) bool {
		for _, p := range poly {
			if math.Abs(p.y-200) < 30 && p.x > 220 {
				return true
			}
		}
		return false
	}

	if crossed(g.visibilityPolygon(100, 200, 400)) {
		t.Fatal("with the wall intact, vision must not reach the far side")
	}

	// Blast a hole straight through the middle wall at the ship's height.
	g.flood.carve(200, 200, 30)
	if crossed(g.visibilityPolygon(100, 200, 400)) == false {
		t.Fatal("a hole through the wall should open a sight cone to the far side")
	}
}

// TestDigForcesFogRestamp: a dig must reset the fog-stamp skip even for a ship
// holding still — the rock face moved, so the sight lines did.
func TestDigForcesFogRestamp(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	segs := wallSegments(lvl)
	g := &Game{segs: segs, bounds: mapBounds(lvl, segs)}
	g.flood = buildFloodmap(segs, 200, 200, g.bounds)
	g.fogStamped = true // as after a previous stamp with the ship parked

	if !g.flood.carve(20, 200, 20) { // bite the west wall (must actually remove rock)
		t.Fatal("test setup: the bite should remove rock")
	}
	g.refreshFlood()
	if g.fogStamped {
		t.Fatal("a dig should force the next fog stamp")
	}
}
