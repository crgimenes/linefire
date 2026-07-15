package mapeditor

import (
	"image"
	"math"

	"linefire/asset"
	"linefire/render"
)

// canvasRect is the screen region used for the world canvas.
func (e *MapEditor) canvasRect() image.Rectangle {
	return image.Rect(0, toolbarHeight, screenWidth-panelWidth, screenHeight)
}

// canvasView is the current world->screen transform.
func (e *MapEditor) canvasView() render.View {
	return e.cam.View
}

// resetView frames the level's nominal world box.
func (e *MapEditor) resetView() {
	e.cam.FitBox(e.canvasRect(), 0, 0, e.level.Size.W, e.level.Size.H)
	e.status = "view reset"
}

// frameContent frames the drawn walls; falls back to the world box.
func (e *MapEditor) frameContent() {
	minX, minY, maxX, maxY, ok := e.contentBounds()
	if !ok {
		e.resetView()
		return
	}
	e.cam.FitBox(e.canvasRect(), minX, minY, maxX, maxY)
	e.status = "framed content"
}

// contentBounds returns the bounding box of all wall vertices.
func (e *MapEditor) contentBounds() (minX, minY, maxX, maxY float64, ok bool) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for li := range e.level.Walls {
		for _, p := range e.level.Walls[li].Paths {
			for _, c := range p.Commands {
				if c.Op == asset.OpClose {
					continue
				}
				ok = true
				minX = math.Min(minX, c.X)
				minY = math.Min(minY, c.Y)
				maxX = math.Max(maxX, c.X)
				maxY = math.Max(maxY, c.Y)
			}
		}
	}
	return minX, minY, maxX, maxY, ok
}

// snapScalar rounds a value to the active editing grid (grid_size when grid
// snapping is on, otherwise the integer pixel grid).
func (e *MapEditor) snapScalar(v float64) float64 {
	g := e.level.Editor.GridSize
	if e.level.Editor.SnapToGrid && g > 1 {
		return math.Round(v/g) * g
	}
	return math.Round(v)
}

// resolveCursor converts the mouse to a world coordinate: Alt = free sub-pixel,
// else snap to a nearby handle, else quantize to the grid. exclude prevents a
// dragged handle snapping onto itself.
func (e *MapEditor) resolveCursor(mouseX, mouseY float64, exclude *handle) (x, y float64) {
	view := e.canvasView()
	rawX, rawY := view.Unproject(mouseX, mouseY)

	e.snapActive = false
	if e.snapDisabled {
		return rawX, rawY
	}

	if e.level.Editor.SnapEnabled {
		tol := e.level.Editor.SnapRadiusPx
		bestDist := tol * tol
		found := false
		var bx, by float64
		consider := func(hx, hy float64) {
			sx, sy := view.Project(hx, hy)
			dx, dy := float64(sx)-mouseX, float64(sy)-mouseY
			d := dx*dx + dy*dy
			if d <= bestDist {
				bx, by, bestDist, found = hx, hy, d, true
			}
		}
		// The wall pen NEVER snaps onto existing vertices or its own start point — it only quantizes
		// to the grid below — so you can cut a segment and splice it elsewhere without the editor
		// welding the ends together or auto-closing the loop. Closing/joining is deliberate: draw
		// the last vertex onto the start (the grid lands you there), or use the Join tool. Every
		// other tool still snaps to handles (to grab/align).
		if e.tool != toolWall {
			for _, h := range e.collectHandles() {
				if exclude != nil && h.same(*exclude) {
					continue
				}
				consider(h.x, h.y)
			}
		}
		if found {
			e.snapActive = true
			e.snapPoint = asset.Point{X: bx, Y: by}
			return bx, by
		}
	}

	return e.snapScalar(rawX), e.snapScalar(rawY)
}

// handleAt returns the nearest handle within the snap radius of a screen point.
func (e *MapEditor) handleAt(mouseX, mouseY float64) (handle, bool) {
	view := e.canvasView()
	tol := e.level.Editor.SnapRadiusPx
	bestDist := tol * tol
	found := false
	var best handle
	for _, h := range e.collectHandles() {
		sx, sy := view.Project(h.x, h.y)
		dx := float64(sx) - mouseX
		dy := float64(sy) - mouseY
		dist := dx*dx + dy*dy
		if dist <= bestDist {
			best = h
			bestDist = dist
			found = true
		}
	}
	return best, found
}
