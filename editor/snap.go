package editor

import "math"

// resolveCursor converts the raw mouse position to an asset-space coordinate.
// It updates the per-frame snap state and returns the coordinate the active
// tool should use:
//
//   - Alt/Option held: the raw, sub-pixel coordinate with no snapping.
//   - Otherwise, a nearby existing point (vertex/origin/hardpoint/collision)
//     wins so segments can share exact endpoints.
//   - Failing that, the coordinate is quantized to the integer pixel grid to
//     keep saved files clean and free of floating-point rounding noise.
//
// exclude, when set, is a handle that must not be considered a snap target
// (used so a dragged handle does not snap onto itself).
func (e *Editor) resolveCursor(mouseX, mouseY float64, exclude *handle) (x, y float64) {
	view := e.canvasView()
	rawX, rawY := view.Unproject(mouseX, mouseY)

	e.snapActive = false
	if e.snapDisabled {
		return rawX, rawY
	}

	if e.asset.Editor.SnapEnabled {
		tol := e.asset.Editor.SnapRadiusPx
		bestPriority := math.MaxInt
		bestDist := tol * tol
		found := false
		var best handle

		for _, h := range e.collectHandles() {
			if exclude != nil && h.sameTarget(*exclude) {
				continue
			}
			sx, sy := view.Project(h.x, h.y)
			dx := float64(sx) - mouseX
			dy := float64(sy) - mouseY
			dist := dx*dx + dy*dy
			if dist > tol*tol {
				continue
			}
			// Prefer higher-priority handles; break ties by screen distance.
			if !found || h.priority() < bestPriority ||
				(h.priority() == bestPriority && dist < bestDist) {
				best = h
				bestPriority = h.priority()
				bestDist = dist
				found = true
			}
		}

		if found {
			e.snapActive = true
			e.snapPoint.X = best.x
			e.snapPoint.Y = best.y
			return best.x, best.y
		}
	}

	return e.quantizeFree(rawX, rawY)
}

// quantizeFree snaps a free coordinate to the editing grid. When grid snapping
// is on (and the grid is coarser than a pixel) it rounds to the nearest
// multiple of grid_size; otherwise it rounds to the integer pixel grid so saved
// files stay clean.
func (e *Editor) quantizeFree(x, y float64) (float64, float64) {
	return e.snapScalar(x), e.snapScalar(y)
}

// handleAt returns the topmost handle within the snap radius of the screen
// point, used by the select tool to pick a handle to drag.
func (e *Editor) handleAt(mouseX, mouseY float64) (handle, bool) {
	view := e.canvasView()
	tol := e.asset.Editor.SnapRadiusPx
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
