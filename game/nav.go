package game

import (
	"math"
)

const (
	navCell      = 24.0  // grid cell size in world units
	navClearance = 44.0  // a cell is blocked if a wall passes this close (keeps paths well off walls)
	navWarmBand  = 110.0 // cells within navClearance+this of a wall are "warm" (costlier)
	navHeatMax   = 10.0  // extra A* cost near a wall (0 far away, this much at the edge)
	// navHullClearance is the field-based grid's block threshold: below it NO enemy hull
	// fits (the smallest is the rusher, radius 12). It has to be far under navClearance,
	// or a tunnel the player just dug (clearance ~10-22) would be blocked for every
	// enemy and nothing would follow them in. Tight cells stay passable but very hot, so
	// A* only takes them when the tunnel is the only way.
	navHullClearance = 14.0
	navSearch        = 6 // rings searched to snap a blocked start/goal to a free cell
)

// vec2 is a world-space point.
type vec2 struct {
	x, y float64
}

// navgrid is a uniform occupancy grid over the level used for A* pathfinding:
// cells within navClearance of a wall are blocked so paths keep enemies clear.
type navgrid struct {
	cols, rows       int
	cell             float64
	originX, originY float64 // world coordinate of cell (0,0)
	blocked          []bool
	heat             []float64 // extra A* cost per cell: hotter near walls, so paths run cool

	// A* scratch, reused across searches — they run sequentially in the game
	// loop, and building fresh maps + a heap per search was ~600 KB and 8½k
	// allocations for one long path (44 MiB over a soak). The GENERATION stamp
	// makes old entries invisible without clearing the arrays: a cell's
	// came/gScore is valid only when its stamp equals the current generation.
	gen       uint32
	seenGen   []uint32 // cell -> generation came/gScore are valid for
	closedGen []uint32 // cell -> generation the cell was closed in
	came      []int32
	gScore    []float64
	open      navHeap
	cells     []int32 // reconstruct scratch: goal..start cell walk
}

// buildNavgrid rasterizes the level walls into a blocked/free grid plus a heat
// field (cells near walls cost more), once. The grid is sized to the map bounds
// (with an origin), so it covers the whole map wherever it sits. A* then prefers
// routes that keep clear of walls, rounding tips with comfortable margin.
func buildNavgrid(segs []segment, b bounds) *navgrid {
	w, h := b.w(), b.h()
	if w <= 0 || h <= 0 {
		return &navgrid{cell: navCell}
	}
	cols := int(math.Ceil(w/navCell)) + 1
	rows := int(math.Ceil(h/navCell)) + 1
	n := &navgrid{
		cols: cols, rows: rows, cell: navCell,
		originX: b.minX, originY: b.minY,
		blocked: make([]bool, cols*rows),
		heat:    make([]float64, cols*rows),
	}

	clr2 := navClearance * navClearance
	coolDist := navClearance + navWarmBand
	for cy := range rows {
		for cx := range cols {
			c := n.center(cx, cy)
			minD2 := math.Inf(1)
			for _, s := range segs {
				d2 := distPointSegmentSq(c.x, c.y, s.ax, s.ay, s.bx, s.by)
				if d2 < minD2 {
					minD2 = d2
				}
			}
			idx := cy*cols + cx
			if minD2 <= clr2 {
				n.blocked[idx] = true
				continue
			}
			d := math.Sqrt(minD2)
			if d < coolDist {
				n.heat[idx] = navHeatMax * (coolDist - d) / navWarmBand
			}
		}
	}
	return n
}

// buildNavgridFromFlood derives the A* grid from the region field instead of the wall
// segments, so a tunnel the player digs is immediately pathable (SPIKE: destructible
// rock). The clearance field already answers "how far is the rock from here", which is
// exactly what blocked/heat need — crg's point that one field feeds everything.
// It spans the WHOLE field (level bounds plus the gridPad of exterior rock), because
// a tunnel dug out past the level boundary must still have A* cells — otherwise
// enemies simply cannot follow the player into it.
func buildNavgridFromFlood(f *floodmap) *navgrid {
	if f == nil || f.cols == 0 || f.rows == 0 {
		return &navgrid{cell: navCell}
	}
	w := float64(f.cols) * f.cell
	h := float64(f.rows) * f.cell
	cols := int(math.Ceil(w/navCell)) + 1
	rows := int(math.Ceil(h/navCell)) + 1
	n := &navgrid{
		cols: cols, rows: rows, cell: navCell,
		originX: f.originX, originY: f.originY,
		blocked: make([]bool, cols*rows),
		heat:    make([]float64, cols*rows),
	}
	for cy := range rows {
		for cx := range cols {
			n.refreshCell(f, cx, cy)
		}
	}
	return n
}

// refreshCell recomputes one cell's blocked/heat from the region field. A cell is
// blocked only where no hull fits; everything tighter than the old comfort margin is
// merely HOT, so A* prefers open space but will squeeze down a tunnel when it must.
func (n *navgrid) refreshCell(f *floodmap, cx, cy int) {
	if cx < 0 || cy < 0 || cx >= n.cols || cy >= n.rows {
		return
	}
	idx := cy*n.cols + cx
	c := n.center(cx, cy)
	d := f.clearanceAt(c.x, c.y)
	if d <= navHullClearance {
		n.blocked[idx] = true
		n.heat[idx] = 0
		return
	}
	n.blocked[idx] = false
	coolDist := navClearance + navWarmBand
	h := 0.0
	if d < coolDist {
		h = navHeatMax * (coolDist - d) / navWarmBand
	}
	n.heat[idx] = math.Min(h, navHeatMax)
}

// navRefreshAround re-derives the A* cells a dig could have freed, so enemies follow
// the player down a fresh tunnel. Cheap: a handful of cells.
func (g *Game) navRefreshAround(x, y, r float64) {
	n, f := g.nav, g.flood
	if n == nil || f == nil || n.cols == 0 {
		return
	}
	reach := r + navClearance + navCell
	minCX := int(math.Floor((x - reach - n.originX) / n.cell))
	maxCX := int(math.Ceil((x + reach - n.originX) / n.cell))
	minCY := int(math.Floor((y - reach - n.originY) / n.cell))
	maxCY := int(math.Ceil((y + reach - n.originY) / n.cell))
	for cy := max(minCY, 0); cy <= min(maxCY, n.rows-1); cy++ {
		for cx := max(minCX, 0); cx <= min(maxCX, n.cols-1); cx++ {
			n.refreshCell(f, cx, cy)
		}
	}
}

// heatAt is the cell's heat, or 0 for grids built without a heat field (tests).
func (n *navgrid) heatAt(idx int) float64 {
	if n.heat == nil {
		return 0
	}
	return n.heat[idx]
}

func (n *navgrid) inBounds(cx, cy int) bool {
	return cx >= 0 && cy >= 0 && cx < n.cols && cy < n.rows
}

func (n *navgrid) isBlocked(cx, cy int) bool {
	return !n.inBounds(cx, cy) || n.blocked[cy*n.cols+cx]
}

func (n *navgrid) cellOf(x, y float64) (int, int) {
	return int((x - n.originX) / n.cell), int((y - n.originY) / n.cell)
}

func (n *navgrid) center(cx, cy int) vec2 {
	return vec2{n.originX + (float64(cx)+0.5)*n.cell, n.originY + (float64(cy)+0.5)*n.cell}
}

// nearestFree returns the closest non-blocked cell to (cx,cy), so a start or goal
// sitting inside the clearance band still yields a usable path.
func (n *navgrid) nearestFree(cx, cy int) (int, int, bool) {
	if !n.isBlocked(cx, cy) {
		return cx, cy, true
	}
	for r := 1; r <= navSearch; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx > -r && dx < r && dy > -r && dy < r {
					continue // only the ring at distance r
				}
				if !n.isBlocked(cx+dx, cy+dy) {
					return cx + dx, cy + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

var navDirs = []struct {
	dx, dy int
	cost   float64
}{
	{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1},
	{1, 1, math.Sqrt2}, {1, -1, math.Sqrt2}, {-1, 1, math.Sqrt2}, {-1, -1, math.Sqrt2},
}

// findPath returns world waypoints from (sx,sy) toward (tx,ty), routing around
// walls, or nil if no path exists. The start cell is omitted; the last waypoint
// is the goal cell center.
func (n *navgrid) findPath(sx, sy, tx, ty float64) []vec2 {
	if n.cols == 0 {
		return nil
	}
	scx, scy := n.cellOf(sx, sy)
	tcx, tcy := n.cellOf(tx, ty)
	scx, scy, ok1 := n.nearestFree(scx, scy)
	tcx, tcy, ok2 := n.nearestFree(tcx, tcy)
	if !ok1 || !ok2 {
		return nil
	}
	start := scy*n.cols + scx
	goal := tcy*n.cols + tcx
	if start == goal {
		return []vec2{n.center(tcx, tcy)}
	}

	if len(n.seenGen) != n.cols*n.rows {
		size := n.cols * n.rows
		n.seenGen = make([]uint32, size)
		n.closedGen = make([]uint32, size)
		n.came = make([]int32, size)
		n.gScore = make([]float64, size)
	}
	n.gen++
	if n.gen == 0 { // a wrapped generation would alias ancient stamps: reset them
		clear(n.seenGen)
		clear(n.closedGen)
		n.gen = 1
	}
	gen := n.gen

	n.seenGen[start] = gen
	n.came[start] = -1
	n.gScore[start] = 0
	open := &n.open
	*open = append((*open)[:0], navNode{idx: start, f: n.heur(scx, scy, tcx, tcy)})

	for len(*open) > 0 {
		cur := open.pop()
		if n.closedGen[cur.idx] == gen {
			continue
		}
		if cur.idx == goal {
			return n.reconstruct(goal)
		}
		n.closedGen[cur.idx] = gen

		ccx, ccy := cur.idx%n.cols, cur.idx/n.cols
		for _, d := range navDirs {
			ncx, ncy := ccx+d.dx, ccy+d.dy
			if n.isBlocked(ncx, ncy) {
				continue
			}
			if d.dx != 0 && d.dy != 0 && (n.isBlocked(ccx+d.dx, ccy) || n.isBlocked(ccx, ccy+d.dy)) {
				continue // do not cut diagonally through a blocked corner
			}
			ni := ncy*n.cols + ncx
			if n.closedGen[ni] == gen {
				continue
			}
			tentative := n.gScore[cur.idx] + d.cost*(1+n.heatAt(ni))
			if n.seenGen[ni] == gen && tentative >= n.gScore[ni] {
				continue
			}
			n.seenGen[ni] = gen
			n.came[ni] = int32(cur.idx) // #nosec G115 -- cell count fits an int32 by construction
			n.gScore[ni] = tentative
			open.push(navNode{idx: ni, f: tentative + n.heur(ncx, ncy, tcx, tcy)})
		}
	}
	return nil
}

func (n *navgrid) heur(cx, cy, tx, ty int) float64 {
	return math.Hypot(float64(cx-tx), float64(cy-ty))
}

func (n *navgrid) reconstruct(goal int) []vec2 {
	cells := n.cells[:0]
	for cur := int32(goal); cur >= 0; cur = n.came[cur] { // #nosec G115 -- cell count fits an int32 by construction
		cells = append(cells, cur)
	}
	n.cells = cells
	// cells is goal..start; emit start+1..goal as waypoints (the enemy is already
	// at the start cell). The returned path is the search's ONE allocation: each
	// entity keeps its result, so it cannot share scratch.
	out := make([]vec2, 0, len(cells)-1)
	for i := len(cells) - 2; i >= 0; i-- {
		out = append(out, n.center(int(cells[i])%n.cols, int(cells[i])/n.cols))
	}
	return out
}

// navNode is an A* open-set entry; navHeap orders them by f-score.
type navNode struct {
	idx int
	f   float64
}

// navHeap is a TYPED min-heap on f-score. container/heap boxes every node into
// an `any`, which put one small heap allocation on every push — thousands per
// long search; the typed sift functions keep the whole open set allocation-free.
type navHeap []navNode

// push adds v and sifts it up.
func (h *navHeap) push(v navNode) {
	*h = append(*h, v)
	s := *h
	i := len(s) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if s[parent].f <= s[i].f {
			break
		}
		s[parent], s[i] = s[i], s[parent]
		i = parent
	}
}

// pop removes and returns the smallest-f node.
func (h *navHeap) pop() navNode {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	*h = s
	i := 0
	for {
		l, r := 2*i+1, 2*i+2
		small := i
		if l < len(s) && s[l].f < s[small].f {
			small = l
		}
		if r < len(s) && s[r].f < s[small].f {
			small = r
		}
		if small == i {
			break
		}
		s[i], s[small] = s[small], s[i]
		i = small
	}
	return top
}
