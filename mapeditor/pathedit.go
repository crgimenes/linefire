package mapeditor

import "github.com/crgimenes/linefire/asset"

// Path editing (kutta-style): the Delete tool removes a node under the cursor, and the Join tool
// connects two OPEN endpoints with a new line — splicing two paths into one, or closing a path onto
// itself. Together with the pen no longer welding/auto-closing (see resolveCursor), this lets the
// author cut a wall apart and stitch the pieces back together freely.

// deleteAtCursor removes whatever node sits under the cursor (a wall vertex, spawn, entry or zone
// point). Backs the Delete tool — no need to select first.
func (e *MapEditor) deleteAtCursor(mx, my float64) {
	h, ok := e.handleAt(mx, my)
	if !ok {
		e.status = "delete: nothing under the cursor"
		return
	}
	e.active, e.hasActive = h, true
	e.deleteSelected() // routes wall-vertex / spawn / entry / zone; clears the selection + dirties
}

// dragThresholdPx separates a CLICK from a DRAG on a loose end (kutta's discrimination): within
// this travel a press-release joins; past it the endpoint is being moved.
const dragThresholdPx = 4.0

// edgeHitPx is how close (screen px) a Shift+click must land to a wall edge to insert a vertex.
const edgeHitPx = 6.0

// clickLooseEnd is a clean click on a red loose end (implicit join, no tool): the first click arms
// the join, the second welds the two ends — same path closes, different paths splice into one.
func (e *MapEditor) clickLooseEnd(h handle) {
	if !e.hasJoinFrom {
		e.joinFrom, e.hasJoinFrom = h, true
		e.status = "join: click the other red end (Esc cancels)"
		return
	}
	if e.joinFrom.same(h) {
		e.status = "join: pick a DIFFERENT red end"
		return
	}
	e.doJoin(e.joinFrom, h)
	e.hasJoinFrom = false
}

// insertEdgeVertexAt inserts a vertex on the wall edge nearest the screen point (within edgeHitPx)
// and returns its handle, ok=false when no straight edge is close enough. Straight (line) edges
// only, including a closed path's implicit closing edge; splitting a Bézier is out of scope — the
// walls are angular. The new vertex lands on the grid, ready to be dragged.
func (e *MapEditor) insertEdgeVertexAt(mx, my float64) (handle, bool) {
	view := e.canvasView()
	best := edgeHitPx * edgeHitPx
	found := false
	var bl, bp, bi int // layer, path, insert position (command index to insert AT)
	var bx, by float64
	try := func(li, pi, insertAt int, x0, y0, x1, y1 float64) {
		sx0, sy0 := view.Project(x0, y0)
		sx1, sy1 := view.Project(x1, y1)
		t, d := pointSegParam(mx, my, float64(sx0), float64(sy0), float64(sx1), float64(sy1))
		if d >= best || t <= 0 || t >= 1 {
			return // beyond tolerance, or on a vertex (grab/join owns those)
		}
		best, found = d, true
		bl, bp, bi = li, pi, insertAt
		bx = e.snapScalar(x0 + (x1-x0)*t)
		by = e.snapScalar(y0 + (y1-y0)*t)
	}
	for li := range e.level.Walls {
		for pi := range e.level.Walls[li].Paths {
			cmds := e.level.Walls[li].Paths[pi].Commands
			for ci := 1; ci < len(cmds); ci++ {
				if cmds[ci].Op == asset.OpLineTo {
					try(li, pi, ci, cmds[ci-1].X, cmds[ci-1].Y, cmds[ci].X, cmds[ci].Y)
				}
				if cmds[ci].Op == asset.OpClose && ci > 0 {
					// The closing edge runs from the last drawable point back to the start.
					try(li, pi, ci, cmds[ci-1].X, cmds[ci-1].Y, cmds[0].X, cmds[0].Y)
				}
			}
		}
	}
	if !found {
		return handle{}, false
	}
	cmds := e.level.Walls[bl].Paths[bp].Commands
	nc := make([]asset.Command, 0, len(cmds)+1)
	nc = append(nc, cmds[:bi]...)
	nc = append(nc, asset.Command{Op: asset.OpLineTo, X: bx, Y: by})
	nc = append(nc, cmds[bi:]...)
	e.level.Walls[bl].Paths[bp].Commands = nc
	e.markDirty()
	e.status = "vertex added on edge"
	return handle{kind: hWallVertex, x: bx, y: by, layer: bl, path: bp, cmd: bi}, true
}

// pointSegParam returns the projection parameter t of point (px,py) onto segment (x0,y0)-(x1,y1),
// clamped to [0,1], and the squared distance to that projection.
func pointSegParam(px, py, x0, y0, x1, y1 float64) (t, distSq float64) {
	dx, dy := x1-x0, y1-y0
	den := dx*dx + dy*dy
	if den == 0 {
		return 0, (px-x0)*(px-x0) + (py-y0)*(py-y0)
	}
	t = ((px-x0)*dx + (py-y0)*dy) / den
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	qx, qy := x0+dx*t, y0+dy*t
	return t, (px-qx)*(px-qx) + (py-qy)*(py-qy)
}

// drawableCommands splits a path into its drawable commands (move + line/quad/cubic, in order) and
// whether it carried a close.
func drawableCommands(cmds []asset.Command) (pts []asset.Command, closed bool) {
	for _, c := range cmds {
		if c.Op == asset.OpClose {
			closed = true
			continue
		}
		pts = append(pts, c)
	}
	return pts, closed
}

// cutAtVertex removes the vertex at di and OPENS the path there instead of bridging its neighbours.
// A closed loop reopens into one path, rotated so the gap sits where the vertex was; an open path
// splits into the two pieces on either side. Pieces below two points are dropped.
func cutAtVertex(pts []asset.Command, di int, closed bool) [][]asset.Command {
	n := len(pts)
	if closed {
		if n < 3 {
			return nil // a 2-point "loop" has no shape left once a vertex is cut
		}
		order := make([]int, 0, n-1)
		for k := 1; k < n; k++ {
			order = append(order, (di+k)%n) // di+1 … di-1, wrapping, skipping di
		}
		open := buildOpenFrom(pts, order)
		if len(open) < 2 {
			return nil
		}
		return [][]asset.Command{open}
	}
	var out [][]asset.Command
	if before := pts[:di]; len(before) >= 2 {
		out = append(out, clonePathCommands(before))
	}
	if after := pts[di+1:]; len(after) >= 2 {
		b := clonePathCommands(after)
		b[0].Op = asset.OpMoveTo // the piece past the cut starts fresh
		b[0].Ctrl = nil
		out = append(out, b)
	}
	return out
}

// buildOpenFrom builds an open path that visits pts in the given index order, carrying each
// segment's original op/handles. The points came from a CLOSED loop, so the edge into point 0 (the
// old closing edge) is a straight line.
func buildOpenFrom(pts []asset.Command, order []int) []asset.Command {
	out := make([]asset.Command, 0, len(order))
	for k, j := range order {
		c := pts[j]
		if k == 0 {
			out = append(out, asset.Command{Op: asset.OpMoveTo, X: c.X, Y: c.Y})
			continue
		}
		op, ctrl := c.Op, c.Ctrl
		if j == 0 {
			op, ctrl = asset.OpLineTo, nil // the old closing edge was a line back to the start
		}
		out = append(out, asset.Command{Op: op, X: c.X, Y: c.Y, Ctrl: append([]asset.Point(nil), ctrl...)})
	}
	return out
}

// lastDrawableIdx returns the index of a path's final non-close command (its end vertex), or -1.
func lastDrawableIdx(cmds []asset.Command) int {
	for i := len(cmds) - 1; i >= 0; i-- {
		if cmds[i].Op != asset.OpClose {
			return i
		}
	}
	return -1
}

// wallEndpoint reports whether a handle is the START (atStart) or END of an OPEN wall path — the
// only nodes the Join tool accepts. ok is false for a mid-vertex, a control point, a closed path,
// or a non-wall handle.
func (e *MapEditor) wallEndpoint(h handle) (atStart bool, ok bool) {
	if h.kind != hWallVertex || h.layer >= len(e.level.Walls) {
		return false, false
	}
	layer := e.level.Walls[h.layer]
	if h.path >= len(layer.Paths) {
		return false, false
	}
	p := layer.Paths[h.path]
	if p.Closed() {
		return false, false
	}
	switch h.cmd {
	case 0:
		return true, true
	case lastDrawableIdx(p.Commands):
		return false, true
	}
	return false, false
}

// doJoin connects two open endpoints. Same path -> close it; different paths -> splice into one.
func (e *MapEditor) doJoin(from, to handle) {
	fromStart, ok1 := e.wallEndpoint(from)
	toStart, ok2 := e.wallEndpoint(to)
	if !ok1 || !ok2 {
		e.status = "join: an endpoint is no longer valid"
		return
	}
	if from.layer == to.layer && from.path == to.path {
		if from.cmd == to.cmd {
			e.status = "join: pick two DIFFERENT endpoints"
			return
		}
		e.level.Walls[from.layer].Paths[from.path].Commands =
			append(e.level.Walls[from.layer].Paths[from.path].Commands, asset.Command{Op: asset.OpClose})
		e.markDirty()
		e.status = "path closed"
		return
	}
	e.mergePaths(from, fromStart, to, toStart)
}

// mergePaths splices path B onto path A with a connecting line: A is oriented so its clicked
// endpoint is the END, B so its clicked endpoint is the START, then B's leading move becomes a line
// (the new connecting segment) and the two command lists concatenate. B's now-empty path is dropped.
func (e *MapEditor) mergePaths(from handle, fromStart bool, to handle, toStart bool) {
	a := stripClose(clonePathCommands(e.level.Walls[from.layer].Paths[from.path].Commands))
	b := stripClose(clonePathCommands(e.level.Walls[to.layer].Paths[to.path].Commands))
	if fromStart {
		a = reversePath(a) // want A to END at the clicked point
	}
	if !toStart {
		b = reversePath(b) // want B to START at the clicked point
	}
	if len(b) > 0 {
		b[0].Op = asset.OpLineTo // B's start move becomes the line joining A's end to B's start
		b[0].Ctrl = nil
	}
	e.level.Walls[from.layer].Paths[from.path].Commands = append(a, b...)
	e.removePath(to.layer, to.path) // A was written by its original index above, so this is safe
	e.markDirty()
	e.status = "paths joined"
}

// removePath drops one wall path from its layer.
func (e *MapEditor) removePath(li, pi int) {
	layer := &e.level.Walls[li]
	layer.Paths = append(layer.Paths[:pi], layer.Paths[pi+1:]...)
}

// stripClose drops any trailing close command, leaving a pure polyline/curve list.
func stripClose(cmds []asset.Command) []asset.Command {
	for len(cmds) > 0 && cmds[len(cmds)-1].Op == asset.OpClose {
		cmds = cmds[:len(cmds)-1]
	}
	return cmds
}

// clonePathCommands deep-copies a command list (including the Ctrl handle slices), so surgery on a
// merged copy never mutates the source path in place.
func clonePathCommands(cmds []asset.Command) []asset.Command {
	out := make([]asset.Command, len(cmds))
	for i, c := range cmds {
		out[i] = c
		if c.Ctrl != nil {
			out[i].Ctrl = append([]asset.Point(nil), c.Ctrl...)
		}
	}
	return out
}

// reversePath reverses a drawable command list (a leading move + line/quad/cubic segments, no
// close), flipping each segment's direction. A cubic's two handles swap (the handle near the old
// end is now near the new start); a quad's single handle and a line are unchanged.
func reversePath(cmds []asset.Command) []asset.Command {
	n := len(cmds)
	if n < 2 {
		return cmds
	}
	out := make([]asset.Command, n)
	out[0] = asset.Command{Op: asset.OpMoveTo, X: cmds[n-1].X, Y: cmds[n-1].Y}
	for k := 1; k < n; k++ {
		seg := cmds[n-k]  // the segment ending at old point n-k, now walked in reverse
		pt := cmds[n-1-k] // its far end becomes the new endpoint
		out[k] = asset.Command{Op: seg.Op, X: pt.X, Y: pt.Y, Ctrl: reverseCtrl(seg.Ctrl)}
	}
	return out
}

// reverseCtrl reverses a segment's control points (swaps a cubic's two handles; leaves a quad's
// single handle or a line's nil as is).
func reverseCtrl(ctrl []asset.Point) []asset.Point {
	if len(ctrl) == 0 {
		return nil
	}
	out := make([]asset.Point, len(ctrl))
	for i := range ctrl {
		out[i] = ctrl[len(ctrl)-1-i]
	}
	return out
}
