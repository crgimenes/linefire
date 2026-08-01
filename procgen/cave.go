package procgen

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// Bonus cave rooms: a finite, self-contained procedural stage a player reaches by an OPTIONAL
// portal in an authored level. It is carved as a network of open CHAMBERS linked by winding
// CORRIDORS (plus a few loop edges, so there is more than one route), lightly smoothed for an
// organic edge, then reduced to its single connected region and seeded with loot, a few guards
// and an exit portal home. Pure cellular automata was tried first but collapsed into one big
// open blob (~90% open, no paths); chambers+corridors gives real routes and fewer dead pockets.
// The reward for the detour is random loot; the risk is the guards.

const (
	caveCols = 44   // cave grid width in cells
	caveRows = 44   // cave grid height in cells
	caveCell = 48.0 // world units per cave cell

	caveChambers   = 8    // open chambers carved before they are linked
	caveChRadMin   = 2    // chamber radius range, in cells
	caveChRadMax   = 5    //
	caveCorridorW  = 1    // corridor brush radius (cells): a disk of r1 leaves a ~3-cell passage
	caveWobble     = 0.15 // per-step chance a corridor drifts sideways instead of toward its target
	caveLoopFactor = 0.4  // extra corridor edges beyond the spanning tree, as a fraction of chambers

	caveSmooth  = 1    // organic-rounding automata passes (kept to 1 so width-1 corridors survive)
	caveMinFrac = 0.15 // sanity floor on the open fraction; below it, re-seed
	caveTries   = 24   // seeds to try before falling back to an open room

	caveSimplifyEps = 34.0 // Douglas-Peucker tolerance (world units): big enough to straighten the grid
	//                        staircase into diagonals, so walls read ANGULAR (like the hand-made maps),
	//                        not rounded — just de-serrilhated.
)

// lootDrop is a bonus reward: the pickup asset to place and the effect kind it declares.
type lootDrop struct{ ref, kind string }

// caveLoot is the bonus reward pool — weighted toward strong pickups, since the room is an
// opt-in risk. A weapon or a combat mod is the payoff for braving the guards.
var caveLoot = []lootDrop{
	{"firepower", "fire"}, {"ratepower", "rate"}, {"damagepower", "damage"},
	{"seekpower", "seek"}, {"shield", "shield"}, {"bubblepower", "bubble"},
	{"reflectpower", "reflect"}, {"allypower", "ally"}, {"dronepower", "drone"},
	{"wpn_missile", "weapon-missile"}, {"wpn_laser", "weapon-laser"}, {"wpn_mine", "weapon-mine"},
	{"powerup", "heal"},
}

// caveGuards are the archetypes that guard the loot (lighter than a boss fight).
var caveGuards = []string{"enemy", "rusher", "turret", "sniper"}

// GenCaveRoom builds a bonus cave stage for the seed, with an exit portal targeting
// exitTarget ("map:label"). It always returns a runnable level: on the rare fully-blocked
// seed it falls back to an open room, so callers never get an empty or broken bonus.
func GenCaveRoom(seed int64, exitTarget string) *level.Level {
	// #nosec G404 G115 -- deterministic procedural generation (not crypto); the seed's bits are hashed on purpose
	rng := rand.New(rand.NewPCG(splitmix(uint64(seed)), 0x424f4e5553)) // stream = "BONUS"

	grid, region := carveValidCave(rng, caveRows, caveCols)
	entry := bottomMost(grid, region)
	exit := exitCell(grid, region, entry)

	lvl := level.New()
	lvl.Name = fmt.Sprintf("bonus %d", seed)
	lvl.Title = "Bonus Vault"
	lvl.Tags = []string{"stage", "bonus"}
	lvl.Size = asset.Size{W: caveCols * caveCell, H: caveRows * caveCell}
	lvl.PlayerStart = level.Start{X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Stroke: "#c8a0ff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8,
		Paths: caveWalls(grid),
	}}
	lvl.Entries = []level.Entry{{Name: "from_bonus", X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}}
	lvl.Spawns = placeContents(rng, region, entry, exit, exitTarget)
	return lvl
}

// Edge sides: which side of a generated screen the player arrives on (and, for the caller, which
// map edge the player breached — the two are opposite).
const (
	EdgeLeft = iota
	EdgeRight
	EdgeTop
	EdgeBottom
)

// carveValidCave carves a rows x cols cave and reduces it to one connected
// region, retrying a few seeds until the open area is big enough and falling
// back to an open room if none qualifies. Every helper below reads the grid's
// own dimensions, so only the carvers need the numbers.
func carveValidCave(rng *rand.Rand, rows, cols int) ([][]bool, []cell) {
	var grid [][]bool
	var region []cell
	for range caveTries {
		grid = carveCave(rng, rows, cols)
		region = largestOpenRegion(grid)
		if float64(len(region)) >= caveMinFrac*float64((rows-2)*(cols-2)) {
			break
		}
		region = nil
	}
	if region == nil {
		grid, region = openFallback(rows, cols)
	}
	return grid, region
}

// edgeGuardsTough and edgeGuardsLight are the archetype pools for an edge room: the deeper the
// player has dug, the more the tough pool is drawn from.
var (
	edgeGuardsTough = []string{"tank", "sniper", "turret"}
	edgeGuardsLight = []string{"enemy", "rusher", "turret", "sniper"}
)

const (
	edgeBaseGuards     = 6    // guards at difficulty 0
	edgeGuardsPerDepth = 3    // extra guards per dig-depth
	edgeGuardsMax      = 22   // hard cap so a deep room stays runnable
	edgeToughBase      = 0.20 // chance a guard is from the tough pool at difficulty 0
	edgeToughPerDepth  = 0.12 // and how fast that climbs with depth
	edgeToughMax       = 0.80
)

// GenEdgeRoom builds a procedural screen the player reaches by DIGGING past a map's edge: a cave
// entered from entrySide (so the tunnel reads as continuing), with NO exit portal — you leave it
// by digging on. It is deliberately harder than a normal screen, and harsher the deeper you have
// dug (difficulty): more guards, drawn increasingly from tanks/snipers/turrets.
func GenEdgeRoom(seed int64, entrySide, difficulty int, returnTarget string) *level.Level {
	// #nosec G404 G115 -- deterministic procedural generation (not crypto); the seed's bits are hashed on purpose
	rng := rand.New(rand.NewPCG(splitmix(uint64(seed)), 0x45444745)) // stream = "EDGE"

	grid, region := carveValidCave(rng, caveRows, caveCols)
	entry := entryCellForSide(grid, region, entrySide)

	lvl := level.New()
	lvl.Name = fmt.Sprintf("edge %d", seed)
	lvl.Title = "The Rift"
	lvl.Tags = []string{"stage", "edge", "procedural"}
	lvl.Size = asset.Size{W: caveCols * caveCell, H: caveRows * caveCell}
	lvl.PlayerStart = level.Start{X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: entryAngleForSide(entrySide)}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Stroke: "#ff7060", StrokeWidth: 2, Fill: "transparent", Glow: 0.8, // red rock: danger
		Paths: caveWalls(grid),
	}}
	lvl.Entries = []level.Entry{{Name: "from_edge", X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: entryAngleForSide(entrySide)}}
	lvl.Spawns = placeEdgeContents(rng, region, entry, difficulty, returnTarget)
	return lvl
}

// entryCellForSide picks the region cell nearest the chosen side that also has clearance, so the
// ship arrives in open space on the side its tunnel came from.
func entryCellForSide(grid [][]bool, region []cell, side int) cell {
	best := region[0]
	bestScore := -math.MaxFloat64
	for _, x := range region {
		var toward float64
		switch side {
		case EdgeLeft:
			toward = -float64(x.c)
		case EdgeRight:
			toward = float64(x.c)
		case EdgeTop:
			toward = -float64(x.r)
		default: // EdgeBottom
			toward = float64(x.r)
		}
		score := toward + float64(caveOpenness(grid, x))
		if score > bestScore {
			bestScore, best = score, x
		}
	}
	return best
}

// GenRiftRoom builds an ENDLESS-mode ("The Rift") screen: like an edge room — a harder cave whose
// guard set scales with depth — but a ONE-WAY trap. It has NO return portal; the only way onward is
// a forward portal to the next Rift, and it opens only when the room is cleared (a DoPortal
// resolution on OnCleared). This is what the player falls into by taking the boss's portal: no exit
// but death. forwardTarget is the reserved runtime name that regenerates the next Rift ("@rift").
func GenRiftRoom(seed int64, difficulty int, forwardTarget string) *level.Level {
	// #nosec G404 G115 -- deterministic procedural generation (not crypto); the seed's bits are hashed on purpose
	rng := rand.New(rand.NewPCG(splitmix(uint64(seed)), 0x52494654)) // stream = "RIFT"

	grid, region := carveValidCave(rng, caveRows, caveCols)
	entry := bottomMost(grid, region)
	exit := exitCell(grid, region, entry)

	lvl := level.New()
	lvl.Name = fmt.Sprintf("rift %d", seed)
	lvl.Title = "The Rift"
	lvl.Tags = []string{"stage", "rift", "procedural"}
	lvl.Size = asset.Size{W: caveCols * caveCell, H: caveRows * caveCell}
	lvl.PlayerStart = level.Start{X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}
	lvl.Walls = []asset.Layer{{
		Name: "walls", Stroke: "#ff7060", StrokeWidth: 2, Fill: "transparent", Glow: 0.8, // red rock: danger
		Paths: caveWalls(grid),
	}}
	lvl.Entries = []level.Entry{{Name: "from_rift", X: cellCenterX(entry.c), Y: cellCenterY(entry.r), Angle: -90}}
	lvl.Spawns = placeEdgeContents(rng, region, entry, difficulty, "") // ramped guards, NO return portal ("" target)
	// The way onward opens only on clear: a forward portal to the next Rift, at the far exit cell.
	lvl.Resolutions = []level.Resolution{{
		On: level.OnCleared, Do: level.DoPortal, Target: forwardTarget,
		X: cellCenterX(exit.c), Y: cellCenterY(exit.r),
	}}
	return lvl
}

// entryAngleForSide faces the ship INTO the room from the side it arrives on (0 = right, 90 = down,
// 180 = left, -90 = up).
func entryAngleForSide(side int) float64 {
	switch side {
	case EdgeLeft:
		return 0
	case EdgeRight:
		return 180
	case EdgeTop:
		return 90
	default: // EdgeBottom
		return -90
	}
}

// placeEdgeContents scatters a harder guard set (scaling with difficulty) plus a little loot as a
// reward for going deep, and ONE randomly-placed return portal back to the authored map (so the
// player can always get out, not just dig deeper). No forward exit portal — the way on is to dig.
func placeEdgeContents(rng *rand.Rand, region []cell, entry cell, difficulty int, returnTarget string) []level.Spawn {
	taken := map[cell]bool{entry: true}
	free := func() (cell, bool) {
		for range 40 {
			x := region[rng.IntN(len(region))]
			if taken[x] || near(x, entry, 3) {
				continue
			}
			taken[x] = true
			return x, true
		}
		return cell{}, false
	}

	var spawns []level.Spawn
	nGuard := min(edgeBaseGuards+difficulty*edgeGuardsPerDepth, edgeGuardsMax)
	for i := range nGuard {
		x, ok := free()
		if !ok {
			break
		}
		k := edgeGuardFor(rng, difficulty)
		spawns = append(spawns, level.Spawn{
			Name: fmt.Sprintf("edge_guard_%d", i), Asset: k, Kind: k,
			X: cellCenterX(x.c), Y: cellCenterY(x.r), Angle: -90,
		})
	}
	nLoot := 2 + rng.IntN(3) // 2..4 rewards
	for i := range nLoot {
		x, ok := free()
		if !ok {
			break
		}
		d := caveLoot[rng.IntN(len(caveLoot))]
		spawns = append(spawns, level.Spawn{
			Name: fmt.Sprintf("edge_loot_%d", i), Asset: d.ref, Kind: d.kind,
			X: cellCenterX(x.c), Y: cellCenterY(x.r), Angle: -90,
		})
	}
	// A return portal to the authored map, dropped at a random open cell away from the entry —
	// always a way back, but you have to find (and reach) it.
	if returnTarget != "" {
		if x, ok := free(); ok {
			spawns = append(spawns, level.Spawn{
				Name: "portal_return", Asset: "portal", Kind: "portal", Target: returnTarget,
				X: cellCenterX(x.c), Y: cellCenterY(x.r), Angle: 0,
			})
		}
	}
	return spawns
}

// edgeGuardFor draws a guard archetype, biased toward the tough pool the deeper the player has dug.
func edgeGuardFor(rng *rand.Rand, difficulty int) string {
	tough := math.Min(edgeToughBase+edgeToughPerDepth*float64(difficulty), edgeToughMax)
	if rng.Float64() < tough {
		return edgeGuardsTough[rng.IntN(len(edgeGuardsTough))]
	}
	return edgeGuardsLight[rng.IntN(len(edgeGuardsLight))]
}

// cell is a grid coordinate.
type cell struct{ r, c int }

func cellCenterX(c int) float64 { return float64(c)*caveCell + caveCell/2 }
func cellCenterY(r int) float64 { return float64(r)*caveCell + caveCell/2 }

// carveCave builds the cave: start solid, carve a handful of chambers, link them with winding
// corridors (a spanning tree plus a few loop edges for alternate routes), then smooth once for
// an organic edge. The border stays rock so the room is sealed.
func carveCave(rng *rand.Rand, rows, cols int) [][]bool {
	grid := make([][]bool, rows)
	for r := range grid {
		grid[r] = make([]bool, cols)
		for c := range grid[r] {
			grid[r][c] = true // start solid; the chambers and corridors carve the open space
		}
	}
	centers := make([]cell, caveChambers)
	for i := range centers {
		centers[i] = cell{1 + rng.IntN(rows-2), 1 + rng.IntN(cols-2)}
		carveDisk(grid, centers[i], caveChRadMin+rng.IntN(caveChRadMax-caveChRadMin+1))
	}
	linkChambers(grid, centers, rng)
	for range caveSmooth {
		grid = smoothCave(grid)
	}
	return grid
}

// caveInBounds reports whether (r,c) is an interior cell (never the sealed border).
func caveInBounds(grid [][]bool, r, c int) bool {
	return r > 0 && c > 0 && r < len(grid)-1 && c < len(grid[0])-1
}

// carveDisk opens every interior cell within rad of ctr (a filled disk).
func carveDisk(grid [][]bool, ctr cell, rad int) {
	for r := ctr.r - rad; r <= ctr.r+rad; r++ {
		for c := ctr.c - rad; c <= ctr.c+rad; c++ {
			if caveInBounds(grid, r, c) && (r-ctr.r)*(r-ctr.r)+(c-ctr.c)*(c-ctr.c) <= rad*rad {
				grid[r][c] = false
			}
		}
	}
}

// linkChambers connects every chamber into one network: a nearest-neighbour spanning tree
// (Prim) guarantees reachability, then a few extra edges add loops so the cave has more than
// one path through it.
func linkChambers(grid [][]bool, centers []cell, rng *rand.Rand) {
	if len(centers) < 2 {
		return
	}
	used := make([]bool, len(centers))
	used[0] = true
	for range len(centers) - 1 {
		bi, bj, bd := -1, -1, math.MaxFloat64
		for i := range centers {
			if !used[i] {
				continue
			}
			for j := range centers {
				if used[j] {
					continue
				}
				if d := chamberDist(centers[i], centers[j]); d < bd {
					bd, bi, bj = d, i, j
				}
			}
		}
		carveCorridor(grid, centers[bi], centers[bj], rng)
		used[bj] = true
	}
	for range int(caveLoopFactor * float64(len(centers))) {
		i, j := rng.IntN(len(centers)), rng.IntN(len(centers))
		if i != j {
			carveCorridor(grid, centers[i], centers[j], rng)
		}
	}
}

func chamberDist(a, b cell) float64 {
	return math.Hypot(float64(a.r-b.r), float64(a.c-b.c))
}

// carveCorridor tunnels from a to b, stepping mostly toward the target but drifting sideways
// now and then so the passage bends instead of running ruler-straight. The brush disk gives it
// width; the endpoint disk guarantees it meets b's chamber.
func carveCorridor(grid [][]bool, a, b cell, rng *rand.Rand) {
	r, c := a.r, a.c
	for guard := 0; (r != b.r || c != b.c) && guard < 4*(len(grid)+len(grid[0])); guard++ {
		carveDisk(grid, cell{r, c}, caveCorridorW)
		if rng.Float64() < caveWobble {
			if rng.IntN(2) == 0 && caveInBounds(grid, r+1, c) {
				r++
			} else if caveInBounds(grid, r-1, c) {
				r--
			}
			continue
		}
		if r != b.r && (c == b.c || rng.IntN(2) == 0) {
			r += stepToward(r, b.r)
		} else {
			c += stepToward(c, b.c)
		}
	}
	carveDisk(grid, b, caveCorridorW)
}

// stepToward returns the unit step that moves from toward to.
func stepToward(from, to int) int {
	switch {
	case from < to:
		return 1
	case from > to:
		return -1
	default:
		return 0
	}
}

// caveOpenness counts open cells in the 3x3 around x (0..8): a rough clearance measure, so
// spawns and portals can prefer the middle of a chamber over a cranny jammed against rock.
func caveOpenness(grid [][]bool, x cell) int {
	n := 0
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr == 0 && dc == 0 {
				continue
			}
			if !caveRock(grid, x.r+dr, x.c+dc) {
				n++
			}
		}
	}
	return n
}

// smoothCave applies one automata pass: a cell becomes rock when at least five of its eight
// neighbours are rock (out-of-grid counts as rock), which grows blobs into smooth caverns.
func smoothCave(grid [][]bool) [][]bool {
	next := make([][]bool, len(grid))
	for r := range next {
		next[r] = make([]bool, len(grid[0]))
		for c := range next[r] {
			if r == 0 || c == 0 || r == len(grid)-1 || c == len(grid[0])-1 {
				next[r][c] = true
				continue
			}
			next[r][c] = rockNeighbors(grid, r, c) >= 5
		}
	}
	return next
}

// rockNeighbors counts rock in the 8-neighbourhood, treating off-grid as rock.
func rockNeighbors(grid [][]bool, r, c int) int {
	n := 0
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr == 0 && dc == 0 {
				continue
			}
			nr, nc := r+dr, c+dc
			if nr < 0 || nc < 0 || nr >= len(grid) || nc >= len(grid[0]) || grid[nr][nc] {
				n++
			}
		}
	}
	return n
}

// largestOpenRegion 4-connected-flood-fills the open cells, keeps only the biggest component,
// and fills every other open cell with rock — leaving one connected cave. Returns its cells.
func largestOpenRegion(grid [][]bool) []cell {
	seen := make([][]bool, len(grid))
	for r := range seen {
		seen[r] = make([]bool, len(grid[0]))
	}
	var best []cell
	for r := range len(grid) {
		for c := range len(grid[0]) {
			if grid[r][c] || seen[r][c] {
				continue
			}
			comp := floodOpen(grid, seen, r, c)
			if len(comp) > len(best) {
				best = comp
			}
		}
	}
	// Fill every open cell not in the winning region back to rock.
	inBest := make(map[cell]bool, len(best))
	for _, x := range best {
		inBest[x] = true
	}
	for r := range len(grid) {
		for c := range len(grid[0]) {
			if !grid[r][c] && !inBest[cell{r, c}] {
				grid[r][c] = true
			}
		}
	}
	return best
}

// floodOpen returns the 4-connected open component reachable from (r,c), marking seen.
func floodOpen(grid, seen [][]bool, r, c int) []cell {
	stack := []cell{{r, c}}
	seen[r][c] = true
	var comp []cell
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		comp = append(comp, x)
		for _, n := range neighbors4(x) {
			if n.r < 0 || n.c < 0 || n.r >= len(grid) || n.c >= len(grid[0]) {
				continue
			}
			if grid[n.r][n.c] || seen[n.r][n.c] {
				continue
			}
			seen[n.r][n.c] = true
			stack = append(stack, n)
		}
	}
	return comp
}

func neighbors4(x cell) [4]cell {
	return [4]cell{{x.r - 1, x.c}, {x.r + 1, x.c}, {x.r, x.c - 1}, {x.r, x.c + 1}}
}

// openFallback is the safety net for a degenerate seed: a plain open room (rock border only).
func openFallback(rows, cols int) ([][]bool, []cell) {
	grid := make([][]bool, rows)
	var region []cell
	for r := range grid {
		grid[r] = make([]bool, cols)
		for c := range grid[r] {
			border := r == 0 || c == 0 || r == rows-1 || c == cols-1
			grid[r][c] = border
			if !border {
				region = append(region, cell{r, c})
			}
		}
	}
	return grid, region
}

// bottomMost returns the region cell nearest the bottom-center that also has clearance — the
// player's entry, so the ship spawns in open space, not wedged in a corridor against rock.
func bottomMost(grid [][]bool, region []cell) cell {
	best := region[0]
	bestScore := -math.MaxFloat64
	mid := float64(len(grid[0])) / 2
	for _, x := range region {
		score := float64(x.r) - math.Abs(float64(x.c)-mid) + float64(caveOpenness(grid, x)) // low, central, open
		if score > bestScore {
			bestScore, best = score, x
		}
	}
	return best
}

// bfsDist returns the 4-connected step distance from start to every reachable open cell.
func bfsDist(grid [][]bool, start cell) map[cell]int {
	dist := map[cell]int{start: 0}
	queue := []cell{start}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		for _, n := range neighbors4(x) {
			if n.r < 0 || n.c < 0 || n.r >= len(grid) || n.c >= len(grid[0]) || grid[n.r][n.c] {
				continue
			}
			if _, ok := dist[n]; ok {
				continue
			}
			dist[n] = dist[x] + 1
			queue = append(queue, n)
		}
	}
	return dist
}

// exitCell picks where the exit portal goes: far from the entry (so the player crosses the
// cave) but with clearance around it, so the portal sits in the middle of a chamber rather than
// jammed against a wall in the deepest cranny. Among the cells in the far quartile, the one with
// the most open neighbours wins; distance breaks ties.
func exitCell(grid [][]bool, region []cell, entry cell) cell {
	dist := bfsDist(grid, entry)
	maxD := 0
	for _, x := range region {
		if d, ok := dist[x]; ok && d > maxD {
			maxD = d
		}
	}
	threshold := 3 * maxD / 4
	best, bestScore := entry, -1
	for _, x := range region {
		d, ok := dist[x]
		if !ok || d < threshold {
			continue
		}
		score := caveOpenness(grid, x)*maxD + d // clearance dominates, distance breaks ties
		if score > bestScore {
			bestScore, best = score, x
		}
	}
	return best
}

// Boundary-edge directions around a grid vertex (dr,dc in cell units).
const (
	dirRight = iota // +c
	dirDown         // +r
	dirLeft         // -c
	dirUp           // -r
)

var (
	caveDirStep = [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	caveDirOpp  = [4]int{dirLeft, dirUp, dirRight, dirDown}
)

// caveRock reports whether cell (r,c) is rock, treating everything off the grid as rock.
func caveRock(grid [][]bool, r, c int) bool {
	if r < 0 || c < 0 || r >= len(grid) || c >= len(grid[0]) {
		return true
	}
	return grid[r][c]
}

// caveWalls turns the open/rock boundary into CLOSED wall loops. Every unit edge where open
// meets rock is stitched head-to-tail into closed polygons, with collinear runs merged. The
// loops MUST be closed: the even-odd fill that carves the navigable region to black (and the
// fog mask) reads the wall paths as closed shapes — an open polyline carves nothing and would
// leave the whole cave the wall colour. Deterministic: vertices and directions are scanned in
// a fixed order, so the same grid always yields the same loops.
func caveWalls(grid [][]bool) []asset.Path {
	edge := boundaryEdges(grid)
	var paths []asset.Path
	for sr := 0; sr <= len(grid); sr++ {
		for sc := 0; sc <= len(grid[0]); sc++ {
			for hasBoundaryEdge(edge[sr][sc]) {
				loop := traceCaveLoop(edge, sr, sc)
				if p, ok := closedCavePath(loop); ok {
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

// vtx is a grid vertex: r,c in cell units (world = c*caveCell, r*caveCell).
type vtx struct{ r, c int }

// boundaryEdges marks, per grid vertex, which of the four incident unit edges lie on an
// open/rock boundary. Each edge is recorded on both of its endpoints so the tracer can walk
// it from either side.
func boundaryEdges(grid [][]bool) [][][4]bool {
	rows, cols := len(grid), len(grid[0])
	edge := make([][][4]bool, rows+1)
	for i := range edge {
		edge[i] = make([][4]bool, cols+1)
	}
	// Horizontal edges (grid line r, spanning c..c+1) split the cells above and below.
	for r := 0; r <= rows; r++ {
		for c := range cols {
			if caveRock(grid, r-1, c) != caveRock(grid, r, c) {
				edge[r][c][dirRight] = true
				edge[r][c+1][dirLeft] = true
			}
		}
	}
	// Vertical edges (grid line c, spanning r..r+1) split the cells left and right.
	for c := 0; c <= cols; c++ {
		for r := range rows {
			if caveRock(grid, r, c-1) != caveRock(grid, r, c) {
				edge[r][c][dirDown] = true
				edge[r+1][c][dirUp] = true
			}
		}
	}
	return edge
}

func hasBoundaryEdge(v [4]bool) bool {
	return v[0] || v[1] || v[2] || v[3]
}

// traceCaveLoop walks boundary edges from (sr,sc), consuming each edge as it goes, until it
// returns to the start — one closed loop. At each vertex it takes the first unused edge in a
// fixed direction order, which is enough: every boundary vertex has even degree, so the walk
// always closes.
func traceCaveLoop(edge [][][4]bool, sr, sc int) []vtx {
	var loop []vtx
	r, c := sr, sc
	for {
		loop = append(loop, vtx{r, c})
		d := -1
		for k := range 4 {
			if edge[r][c][k] {
				d = k
				break
			}
		}
		if d < 0 {
			break // no exit (only the degenerate first step of an emptied vertex)
		}
		edge[r][c][d] = false
		nr, nc := r+caveDirStep[d][0], c+caveDirStep[d][1]
		edge[nr][nc][caveDirOpp[d]] = false
		r, c = nr, nc
		if r == sr && c == sc {
			break
		}
	}
	return loop
}

// closedCavePath turns a boundary loop into an ANGULAR de-serrilhated closed path: reduce its
// collinear steps to corners, then Douglas-Peucker with a tolerance near the cell size, which
// collapses each grid staircase into a single straight diagonal while keeping real corners
// sharp. No curves — the walls stay angular like the hand-made maps, just without the per-cell
// jaggies. Reports false for a degenerate loop.
func closedCavePath(loop []vtx) (asset.Path, bool) {
	corners := simplifyCaveLoop(loop)
	if len(corners) < 3 {
		return asset.Path{}, false
	}
	poly := make([]ptf, len(corners))
	for i, v := range corners {
		poly[i] = ptf{float64(v.c) * caveCell, float64(v.r) * caveCell}
	}
	poly = simplifyPoly(poly, caveSimplifyEps)
	if len(poly) < 3 {
		return asset.Path{}, false
	}
	cmds := make([]asset.Command, 0, len(poly)+1)
	for i, p := range poly {
		op := asset.OpLineTo
		if i == 0 {
			op = asset.OpMoveTo
		}
		cmds = append(cmds, asset.Command{Op: op, X: p.x, Y: p.y})
	}
	cmds = append(cmds, asset.Command{Op: asset.OpClose})
	return asset.Path{Commands: cmds}, true
}

// ptf is a world-space point.
type ptf struct{ x, y float64 }

// simplifyPoly thins a closed polygon with Douglas-Peucker: it keeps the points that carry the
// shape (bends beyond eps) and drops the rest, straightening the grid staircase into a handful
// of segments — which both de-serrilhates the walls and keeps the flood/nav build cheap.
func simplifyPoly(pts []ptf, eps float64) []ptf {
	n := len(pts)
	if n < 4 {
		return pts
	}
	// Open the loop at point 0 (duplicated at the end) so the anchored recursion keeps it closed.
	chain := douglasPeucker(append(pts, pts[0]), eps)
	if len(chain) > 1 {
		chain = chain[:len(chain)-1] // drop the duplicate seam point
	}
	if len(chain) < 3 {
		return pts
	}
	return chain
}

// douglasPeucker returns the subset of pts within eps of the original polyline (endpoints kept).
func douglasPeucker(pts []ptf, eps float64) []ptf {
	if len(pts) < 3 {
		return pts
	}
	last := len(pts) - 1
	maxD, idx := 0.0, 0
	for i := 1; i < last; i++ {
		if d := perpDist(pts[i], pts[0], pts[last]); d > maxD {
			maxD, idx = d, i
		}
	}
	if maxD <= eps {
		return []ptf{pts[0], pts[last]}
	}
	left := douglasPeucker(pts[:idx+1], eps)
	right := douglasPeucker(pts[idx:], eps)
	return append(left[:len(left)-1], right...)
}

// perpDist is the distance from p to the segment a-b (or to a if a==b).
func perpDist(p, a, b ptf) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.x-a.x, p.y-a.y)
	}
	t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p.x-(a.x+t*dx), p.y-(a.y+t*dy))
}

// simplifyCaveLoop keeps only the corners of an axis-aligned loop: a vertex whose predecessor
// and successor share its row or its column lies mid-run and is dropped. Cyclic, so the seam
// between the last and first vertex is simplified too.
func simplifyCaveLoop(loop []vtx) []vtx {
	n := len(loop)
	if n < 3 {
		return loop
	}
	var out []vtx
	for i := range loop {
		prev := loop[(i-1+n)%n]
		next := loop[(i+1)%n]
		straight := (prev.r == loop[i].r && loop[i].r == next.r) || (prev.c == loop[i].c && loop[i].c == next.c)
		if straight {
			continue
		}
		out = append(out, loop[i])
	}
	return out
}

// placeContents scatters loot and guards through the region and drops the exit portal at exit.
// Loot and guards keep clear of the entry and of each other so nothing spawns on the ship or
// stacks up.
func placeContents(rng *rand.Rand, region []cell, entry, exit cell, exitTarget string) []level.Spawn {
	taken := map[cell]bool{entry: true, exit: true}
	// keep spawns a few cells apart and off the entry
	free := func() (cell, bool) {
		for range 40 {
			x := region[rng.IntN(len(region))]
			if taken[x] || near(x, entry, 3) {
				continue
			}
			taken[x] = true
			return x, true
		}
		return cell{}, false
	}

	var spawns []level.Spawn
	nLoot := 5 + rng.IntN(4) // 5..8 rewards
	for i := range nLoot {
		x, ok := free()
		if !ok {
			break
		}
		d := caveLoot[rng.IntN(len(caveLoot))]
		spawns = append(spawns, level.Spawn{
			Name: fmt.Sprintf("loot_%d", i), Asset: d.ref, Kind: d.kind,
			X: cellCenterX(x.c), Y: cellCenterY(x.r), Angle: -90,
		})
	}
	nGuard := 3 + rng.IntN(4) // 3..6 guards
	for i := range nGuard {
		x, ok := free()
		if !ok {
			break
		}
		k := caveGuards[rng.IntN(len(caveGuards))]
		spawns = append(spawns, level.Spawn{
			Name: fmt.Sprintf("guard_%d", i), Asset: k, Kind: k,
			X: cellCenterX(x.c), Y: cellCenterY(x.r), Angle: -90,
		})
	}
	spawns = append(spawns, level.Spawn{
		Name: "portal_exit", Asset: "portal", Kind: "portal", Target: exitTarget,
		X: cellCenterX(exit.c), Y: cellCenterY(exit.r), Angle: -90,
	})
	return spawns
}

// near reports whether two cells are within d steps on each axis.
func near(a, b cell, d int) bool {
	dr, dc := a.r-b.r, a.c-b.c
	if dr < 0 {
		dr = -dr
	}
	if dc < 0 {
		dc = -dc
	}
	return dr <= d && dc <= d
}
