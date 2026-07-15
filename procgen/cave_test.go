package procgen

import (
	"math/rand/v2"
	"testing"

	"linefire/asset"
	"linefire/level"
)

// countGuards counts the enemy-archetype spawns in a level.
func countGuards(lvl *level.Level) int {
	guard := map[string]bool{"enemy": true, "rusher": true, "turret": true, "sniper": true, "tank": true}
	n := 0
	for _, s := range lvl.Spawns {
		if guard[s.Kind] {
			n++
		}
	}
	return n
}

// TestGenEdgeRoom: an edge room arrives on the requested side, has no exit portal (you dig on),
// and is harder the deeper you have dug.
func TestGenEdgeRoom(t *testing.T) {
	mid := float64(caveCols) * caveCell / 2

	left := GenEdgeRoom(3, EdgeLeft, 2, "map0001")
	if len(left.Entries) == 0 || left.Entries[0].Name != "from_edge" {
		t.Fatal("an edge room must have a from_edge entry")
	}
	if left.Entries[0].X >= mid {
		t.Fatalf("EdgeLeft entry should be on the left half, got x=%.0f", left.Entries[0].X)
	}
	if right := GenEdgeRoom(3, EdgeRight, 2, "map0001"); right.Entries[0].X <= mid {
		t.Fatalf("EdgeRight entry should be on the right half, got x=%.0f", right.Entries[0].X)
	}

	// Exactly one portal, and it is the RETURN portal to the authored map (no forward exit).
	portals := 0
	for _, s := range left.Spawns {
		if s.Kind == "portal" {
			portals++
			if s.Target != "map0001" {
				t.Fatalf("the return portal should target the authored map, got %q", s.Target)
			}
		}
	}
	if portals != 1 {
		t.Fatalf("an edge room should have exactly one (return) portal, got %d", portals)
	}

	shallow := countGuards(GenEdgeRoom(7, EdgeTop, 0, "map0001"))
	deep := countGuards(GenEdgeRoom(7, EdgeTop, 5, "map0001"))
	if deep <= shallow {
		t.Fatalf("a deeper edge room should carry more guards: depth0=%d depth5=%d", shallow, deep)
	}
}

// TestCavePipelineConnectedAndPlaced: the carved cave reduces to one connected open region,
// and the entry, exit and every placed spawn land on open cells inside it.
func TestCavePipelineConnectedAndPlaced(t *testing.T) {
	rng := rand.New(rand.NewPCG(splitmix(7), 0x424f4e5553))
	grid := carveCave(rng)
	region := largestOpenRegion(grid)

	inRegion := map[cell]bool{}
	for _, x := range region {
		if grid[x.r][x.c] {
			t.Fatalf("region cell (%d,%d) is rock", x.r, x.c)
		}
		inRegion[x] = true
	}
	// largestOpenRegion must have filled every OTHER open cell with rock: the only open cells
	// left are the region, so it is a single connected component.
	for r := range caveRows {
		for c := range caveCols {
			if !grid[r][c] && !inRegion[cell{r, c}] {
				t.Fatalf("open cell (%d,%d) outside the kept region — cave is not single-component", r, c)
			}
		}
	}

	entry := bottomMost(grid, region)
	exit := exitCell(grid, region, entry)
	if !inRegion[entry] || !inRegion[exit] {
		t.Fatal("entry/exit must be open region cells")
	}
	if entry == exit {
		t.Fatal("exit should be far from entry, not the same cell")
	}
	// The exit must have real clearance around it, not be wedged against rock.
	if caveOpenness(grid, exit) < 5 {
		t.Fatalf("exit cell (%d,%d) has poor clearance: %d/8 open", exit.r, exit.c, caveOpenness(grid, exit))
	}

	spawns := placeContents(rng, region, entry, exit, "map0001:from_bonus")
	for _, s := range spawns {
		c := int(s.X / caveCell)
		r := int(s.Y / caveCell)
		if r < 0 || r >= caveRows || c < 0 || c >= caveCols || grid[r][c] {
			t.Fatalf("spawn %q at cell (%d,%d) is not open", s.Name, r, c)
		}
	}
}

// TestGenCaveRoomDeterministic: the same seed always builds the same bonus room.
func TestGenCaveRoomDeterministic(t *testing.T) {
	a := GenCaveRoom(42, "map0001:from_bonus")
	b := GenCaveRoom(42, "map0001:from_bonus")
	if len(a.Walls[0].Paths) != len(b.Walls[0].Paths) || len(a.Spawns) != len(b.Spawns) {
		t.Fatalf("same seed should match: walls %d/%d spawns %d/%d",
			len(a.Walls[0].Paths), len(b.Walls[0].Paths), len(a.Spawns), len(b.Spawns))
	}
	if a.PlayerStart != b.PlayerStart {
		t.Fatal("same seed should place the player identically")
	}
	for i := range a.Spawns {
		if a.Spawns[i] != b.Spawns[i] {
			t.Fatalf("spawn %d differs between identical seeds", i)
		}
	}
}

// TestCaveWallsAreClosedAndCarveOpenSpace guards the bug where the whole cave rendered in the
// wall colour: the negative-space carve and the fog mask fill the wall paths with the even-odd
// rule, so the walls MUST be closed loops. Every path has to open with a MoveTo and end with a
// Close, and an even-odd ray test must place the player start (open space) INSIDE the figure and
// a border rock cell OUTSIDE it.
func TestCaveWallsAreClosedAndCarveOpenSpace(t *testing.T) {
	lvl := GenCaveRoom(11, "map0001:from_bonus")
	paths := lvl.Walls[0].Paths
	if len(paths) == 0 {
		t.Fatal("cave has no walls")
	}
	for i, p := range paths {
		if len(p.Commands) < 4 {
			t.Fatalf("path %d has too few commands to be a closed loop: %d", i, len(p.Commands))
		}
		if p.Commands[0].Op != asset.OpMoveTo {
			t.Fatalf("path %d must start with MoveTo", i)
		}
		if p.Commands[len(p.Commands)-1].Op != asset.OpClose {
			t.Fatalf("path %d must end with Close — an open polyline carves nothing", i)
		}
	}
	if evenOddCrossings(paths, lvl.PlayerStart.X, lvl.PlayerStart.Y)%2 == 0 {
		t.Fatal("player start (open space) should be INSIDE the carved figure: odd crossings")
	}
	// A top-border cell is always rock; its centre must fall OUTSIDE the figure: even crossings.
	if evenOddCrossings(paths, cellCenterX(caveCols/2), cellCenterY(0))%2 != 0 {
		t.Fatal("a border rock cell should be OUTSIDE the figure: even crossings")
	}
}

// evenOddCrossings counts how many wall segments a +x ray from (px,py) crosses — the even-odd
// fill rule the renderer carves the navigable region with.
func evenOddCrossings(paths []asset.Path, px, py float64) int {
	n := 0
	cast := func(ax, ay, bx, by float64) {
		if (ay > py) == (by > py) {
			return
		}
		if ax+(py-ay)/(by-ay)*(bx-ax) > px {
			n++
		}
	}
	for _, p := range paths {
		var first, prev asset.Point
		for _, c := range p.Commands {
			switch c.Op {
			case asset.OpMoveTo:
				first, prev = asset.Point{X: c.X, Y: c.Y}, asset.Point{X: c.X, Y: c.Y}
			case asset.OpLineTo:
				cast(prev.X, prev.Y, c.X, c.Y)
				prev = asset.Point{X: c.X, Y: c.Y}
			case asset.OpClose:
				cast(prev.X, prev.Y, first.X, first.Y)
			}
		}
	}
	return n
}

// TestGenCaveRoomHasExitAndLoot: a bonus room ships walls, an exit portal to the given target,
// several rewards and a few guards.
func TestGenCaveRoomHasExitAndLoot(t *testing.T) {
	lvl := GenCaveRoom(7, "map0002:from_bonus")
	if len(lvl.Walls[0].Paths) == 0 {
		t.Fatal("a cave should have walls")
	}
	guardKinds := map[string]bool{}
	for _, k := range caveGuards {
		guardKinds[k] = true
	}
	loot, guards, portals := 0, 0, 0
	for _, s := range lvl.Spawns {
		switch {
		case s.Kind == "portal":
			portals++
			if s.Target != "map0002:from_bonus" {
				t.Fatalf("exit portal target = %q, want the return", s.Target)
			}
		case guardKinds[s.Kind]:
			guards++
		default:
			loot++
		}
	}
	if portals != 1 {
		t.Fatalf("want exactly 1 exit portal, got %d", portals)
	}
	if loot < 5 {
		t.Fatalf("want >=5 rewards, got %d", loot)
	}
	if guards < 3 {
		t.Fatalf("want >=3 guards, got %d", guards)
	}
}
