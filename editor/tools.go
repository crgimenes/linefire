package editor

import (
	"fmt"
	"math"
	"strings"

	"linefire/asset"
)

// handleKind classifies a draggable/snappable point in the document.
type handleKind int

const (
	handleVertex  handleKind = iota
	handleControl            // a curve command's Bézier control point
	handleOrigin
	handleHardpoint
	handleCollision
)

// handle references a single editable point. The identity fields depend on the
// kind: vertices use layer/path/cmd, hardpoints use index, collision points use
// shapeIdx/pointIdx, and the origin needs no extra identity.
type handle struct {
	kind handleKind
	x, y float64

	layer, path, cmd   int // handleVertex
	index              int // handleHardpoint
	shapeIdx, pointIdx int // handleCollision
}

// priority orders handles for snapping: lower wins. Matches the documented
// priority vertices > origin > hardpoints > collision.
func (h handle) priority() int {
	switch h.kind {
	case handleVertex:
		return 0
	case handleOrigin:
		return 1
	case handleHardpoint:
		return 2
	case handleControl:
		return 4 // lowest: control points must not steal snaps from real points
	default:
		return 3
	}
}

// sameTarget reports whether two handles refer to the same editable point,
// used to avoid snapping a dragged handle onto itself.
func (h handle) sameTarget(o handle) bool {
	if h.kind != o.kind {
		return false
	}
	switch h.kind {
	case handleVertex:
		return h.layer == o.layer && h.path == o.path && h.cmd == o.cmd
	case handleControl:
		return h.layer == o.layer && h.path == o.path && h.cmd == o.cmd && h.pointIdx == o.pointIdx
	case handleHardpoint:
		return h.index == o.index
	case handleCollision:
		return h.shapeIdx == o.shapeIdx && h.pointIdx == o.pointIdx
	default:
		return true
	}
}

// collectHandles gathers every editable point currently in the document.
func (e *Editor) collectHandles() []handle {
	var hs []handle
	for li := range e.asset.Layers {
		layer := &e.asset.Layers[li]
		if layer.Hidden {
			continue
		}
		for pi := range layer.Paths {
			for ci, c := range layer.Paths[pi].Commands {
				if c.Op == asset.OpClose {
					continue
				}
				hs = append(hs, handle{
					kind: handleVertex, x: c.X, y: c.Y,
					layer: li, path: pi, cmd: ci,
				})
				for k, cp := range c.Ctrl {
					hs = append(hs, handle{
						kind: handleControl, x: cp.X, y: cp.Y,
						layer: li, path: pi, cmd: ci, pointIdx: k,
					})
				}
			}
		}
	}
	hs = append(hs, handle{kind: handleOrigin, x: e.asset.Origin.X, y: e.asset.Origin.Y})
	for i, hp := range e.asset.Hardpoints {
		hs = append(hs, handle{kind: handleHardpoint, x: hp.X, y: hp.Y, index: i})
	}
	for si := range e.asset.Collisions {
		for pi, p := range e.asset.Collisions[si].Points {
			hs = append(hs, handle{kind: handleCollision, x: p.X, y: p.Y, shapeIdx: si, pointIdx: pi})
		}
	}
	return hs
}

// moveHandle writes a new asset-space position into the point a handle refers
// to. It silently ignores stale references (e.g. after a deletion).
func (e *Editor) moveHandle(h handle, x, y float64) {
	switch h.kind {
	case handleVertex:
		if h.layer < len(e.asset.Layers) {
			layer := &e.asset.Layers[h.layer]
			if h.path < len(layer.Paths) && h.cmd < len(layer.Paths[h.path].Commands) {
				layer.Paths[h.path].Commands[h.cmd].X = x
				layer.Paths[h.path].Commands[h.cmd].Y = y
			}
		}
	case handleControl:
		if h.layer < len(e.asset.Layers) {
			layer := &e.asset.Layers[h.layer]
			if h.path < len(layer.Paths) && h.cmd < len(layer.Paths[h.path].Commands) {
				ctrl := layer.Paths[h.path].Commands[h.cmd].Ctrl
				if h.pointIdx < len(ctrl) {
					ctrl[h.pointIdx] = asset.Point{X: x, Y: y}
				}
			}
		}
	case handleOrigin:
		e.asset.Origin = asset.Point{X: x, Y: y}
	case handleHardpoint:
		if h.index < len(e.asset.Hardpoints) {
			e.asset.Hardpoints[h.index].X = x
			e.asset.Hardpoints[h.index].Y = y
		}
	case handleCollision:
		if h.shapeIdx < len(e.asset.Collisions) {
			s := &e.asset.Collisions[h.shapeIdx]
			if h.pointIdx < len(s.Points) {
				s.Points[h.pointIdx] = asset.Point{X: x, Y: y}
			}
		}
	}
	e.markDirty()
}

// commitDraft appends the in-progress path to the current layer if it has at
// least one segment, then clears the draft.
func (e *Editor) commitDraft() {
	if len(e.draft) >= 2 {
		layer := e.currentLayer()
		cmds := make([]asset.Command, len(e.draft))
		copy(cmds, e.draft)
		layer.Paths = append(layer.Paths, asset.Path{Commands: cmds})
		e.markDirty()
		e.status = fmt.Sprintf("path added (%d points)", len(cmds))
	} else {
		e.status = "path discarded (needs 2+ points)"
	}
	e.draft = nil
	e.drafting = false
	e.penActive = false
	e.draftLastHandle = asset.Point{}
}

// cancelDraft drops the in-progress path without committing it.
func (e *Editor) cancelDraft() {
	e.draft = nil
	e.drafting = false
	e.penActive = false
	e.draftLastHandle = asset.Point{}
	e.status = "draft cancelled"
}

// penHandleMin is the smallest drag, in asset units, that counts as a curve
// handle rather than a plain click (a corner). Grid snapping usually quantizes a
// real drag well past this; it matters mainly with sub-pixel (Alt) placement.
const penHandleMin = 3.0

// atDraftStart reports whether the cursor sits on the draft's start point and the
// draft has enough points to close into a shape.
func (e *Editor) atDraftStart() bool {
	return e.drafting && len(e.draft) >= 3 && e.cursor.X == e.draft[0].X && e.cursor.Y == e.draft[0].Y
}

// beginLineNode starts placing a vertex at the press point; the drag until release
// becomes its curve handle (see finishLineNode).
func (e *Editor) beginLineNode(p asset.Point) {
	if !e.drafting {
		e.drafting = true
		e.draftLastHandle = asset.Point{}
		e.status = "line: drag = curve, click = corner; click start = close"
	}
	e.penActive = true
	e.penAnchor = p
}

// finishLineNode commits the vertex on release: a near-zero handle makes a corner
// (line), otherwise a smooth cubic using the previous vertex's out-handle and this
// vertex's mirrored in-handle.
func (e *Editor) finishLineNode(end asset.Point) {
	e.penActive = false
	handle := asset.Point{X: end.X - e.penAnchor.X, Y: end.Y - e.penAnchor.Y}
	if math.Hypot(handle.X, handle.Y) < penHandleMin {
		handle = asset.Point{}
	}

	if len(e.draft) == 0 {
		e.draft = []asset.Command{{Op: asset.OpMoveTo, X: e.penAnchor.X, Y: e.penAnchor.Y}}
		e.draftLastHandle = handle
		return
	}

	prev := e.draft[len(e.draft)-1]
	straight := e.draftLastHandle == (asset.Point{}) && handle == (asset.Point{})
	if straight {
		e.draft = append(e.draft, asset.Command{Op: asset.OpLineTo, X: e.penAnchor.X, Y: e.penAnchor.Y})
	} else {
		c1 := asset.Point{X: prev.X + e.draftLastHandle.X, Y: prev.Y + e.draftLastHandle.Y}
		c2 := asset.Point{X: e.penAnchor.X - handle.X, Y: e.penAnchor.Y - handle.Y}
		e.draft = append(e.draft, asset.Command{Op: asset.OpCubicTo, X: e.penAnchor.X, Y: e.penAnchor.Y, Ctrl: []asset.Point{c1, c2}})
	}
	e.draftLastHandle = handle
	e.markDirty()
}

// closeLine closes the draft into a shape and commits it.
func (e *Editor) closeLine() {
	e.draft = append(e.draft, asset.Command{Op: asset.OpClose})
	e.commitDraft()
}

// addHardpoint inserts a new hardpoint of the given kind at the cursor.
func (e *Editor) addHardpoint(kind string, x, y float64) {
	angle := -90.0 // weapons point up by default
	if kind == asset.KindThruster {
		angle = 90.0 // thrusters point down (exhaust toward the rear)
	}
	n := 1
	for _, hp := range e.asset.Hardpoints {
		if hp.Kind == kind {
			n++
		}
	}
	name := fmt.Sprintf("%s_%d", kind, n)
	e.asset.Hardpoints = append(e.asset.Hardpoints, asset.Hardpoint{
		Name: name, Kind: kind, X: x, Y: y, Angle: angle,
	})
	e.markDirty()
	e.status = "added " + name
}

// cycleCollKind advances the collision shape kind being authored and clears any
// in-progress draft.
func (e *Editor) cycleCollKind() {
	switch e.collKind {
	case asset.CollisionCircle:
		e.collKind = asset.CollisionRect
	case asset.CollisionRect:
		e.collKind = asset.CollisionTriangle
	default:
		e.collKind = asset.CollisionCircle
	}
	e.collDraft = nil
	e.status = "collision shape: " + e.collKind
}

// placeCollisionPoint adds a click to the in-progress collision shape and
// finalizes it once enough points are gathered.
func (e *Editor) placeCollisionPoint(x, y float64) {
	switch e.collKind {
	case asset.CollisionCircle:
		if len(e.collDraft) == 0 {
			e.collDraft = []asset.Point{{X: x, Y: y}}
			e.status = "circle: click radius"
			return
		}
		c := e.collDraft[0]
		r := math.Round(math.Hypot(x-c.X, y-c.Y))
		e.addCollisionShape(asset.CollisionShape{Kind: asset.CollisionCircle, Points: []asset.Point{c}, Radius: r})
	case asset.CollisionRect:
		if len(e.collDraft) == 0 {
			e.collDraft = []asset.Point{{X: x, Y: y}}
			e.status = "rect: click opposite corner"
			return
		}
		e.addCollisionShape(asset.CollisionShape{Kind: asset.CollisionRect, Points: []asset.Point{e.collDraft[0], {X: x, Y: y}}})
	case asset.CollisionTriangle:
		e.collDraft = append(e.collDraft, asset.Point{X: x, Y: y})
		if len(e.collDraft) < 3 {
			e.status = fmt.Sprintf("triangle: %d/3 points", len(e.collDraft))
			return
		}
		pts := make([]asset.Point, 3)
		copy(pts, e.collDraft)
		e.addCollisionShape(asset.CollisionShape{Kind: asset.CollisionTriangle, Points: pts})
	}
}

// addCollisionShape appends a finished collision shape and clears the draft.
func (e *Editor) addCollisionShape(s asset.CollisionShape) {
	e.asset.Collisions = append(e.asset.Collisions, s)
	e.collDraft = nil
	e.markDirty()
	e.status = "added collision " + s.Kind
}

// deleteVertex removes the vertex a handle points at; if its path becomes empty
// it is removed too.
func (e *Editor) deleteVertex(h handle) {
	if h.kind != handleVertex || h.layer >= len(e.asset.Layers) {
		return
	}
	layer := &e.asset.Layers[h.layer]
	if h.path >= len(layer.Paths) {
		return
	}
	cmds := layer.Paths[h.path].Commands
	if h.cmd >= len(cmds) {
		return
	}
	cmds = append(cmds[:h.cmd], cmds[h.cmd+1:]...)
	if hasDrawablePoint(cmds) {
		// Ensure the path still begins with a move command.
		if len(cmds) > 0 && cmds[0].Op == asset.OpLineTo {
			cmds[0].Op = asset.OpMoveTo
		}
		layer.Paths[h.path].Commands = cmds
	} else {
		layer.Paths = append(layer.Paths[:h.path], layer.Paths[h.path+1:]...)
	}
	e.hasActive = false
	e.markDirty()
	e.status = "vertex removed"
}

// deleteSelected removes whatever handle is currently selected: a vertex, a
// hardpoint or the collision shape. The origin cannot be removed.
func (e *Editor) deleteSelected() {
	if !e.hasActive {
		return
	}
	switch e.active.kind {
	case handleVertex:
		e.deleteVertex(e.active)
	case handleHardpoint:
		if e.active.index < len(e.asset.Hardpoints) {
			e.asset.Hardpoints = append(e.asset.Hardpoints[:e.active.index], e.asset.Hardpoints[e.active.index+1:]...)
			e.hasActive = false
			e.markDirty()
			e.status = "hardpoint removed"
		}
	case handleCollision:
		if e.active.shapeIdx < len(e.asset.Collisions) {
			e.asset.Collisions = append(e.asset.Collisions[:e.active.shapeIdx], e.asset.Collisions[e.active.shapeIdx+1:]...)
			e.hasActive = false
			e.markDirty()
			e.status = "collision removed"
		}
	case handleOrigin:
		e.status = "origin cannot be removed"
	}
}

// deleteAtCursor removes whatever editable point lies under the screen position,
// used by the delete tool. Vertices, hardpoints and collision shapes can go; the
// origin cannot.
func (e *Editor) deleteAtCursor(mx, my float64) {
	h, ok := e.handleAt(mx, my)
	if !ok {
		e.status = "nothing to delete here"
		return
	}
	e.active = h
	e.hasActive = true
	e.deleteSelected()
}

// rotateHardpoint adjusts the angle of the selected hardpoint by delta degrees,
// normalized to (-180, 180].
func (e *Editor) rotateHardpoint(delta float64) {
	if e.active.kind != handleHardpoint || e.active.index >= len(e.asset.Hardpoints) {
		return
	}
	a := normalizeAngle(e.asset.Hardpoints[e.active.index].Angle + delta)
	e.asset.Hardpoints[e.active.index].Angle = a
	e.markDirty()
	e.status = fmt.Sprintf("angle %g", a)
}

// adjustStrokeWidth changes the current layer's stroke width, clamped at zero.
func (e *Editor) adjustStrokeWidth(delta float64) {
	layer := e.currentLayer()
	w := layer.StrokeWidth + delta
	if w < 0 {
		w = 0
	}
	layer.StrokeWidth = w
	e.markDirty()
	e.status = fmt.Sprintf("stroke width %g", w)
}

// snapScalar rounds a single value to the active editing grid (grid_size when
// grid snapping is on, otherwise the integer pixel grid).
func (e *Editor) snapScalar(v float64) float64 {
	g := e.asset.Editor.GridSize
	if e.asset.Editor.SnapToGrid && g > 1 {
		return math.Round(v/g) * g
	}
	return math.Round(v)
}

// snapAllToGrid rounds every coordinate in the asset to the active grid, useful
// for cleaning up legacy assets with fractional coordinates.
func (e *Editor) snapAllToGrid() {
	for li := range e.asset.Layers {
		paths := e.asset.Layers[li].Paths
		for pi := range paths {
			cmds := paths[pi].Commands
			for ci := range cmds {
				if cmds[ci].Op == asset.OpClose {
					continue
				}
				cmds[ci].X = e.snapScalar(cmds[ci].X)
				cmds[ci].Y = e.snapScalar(cmds[ci].Y)
				for k := range cmds[ci].Ctrl {
					cmds[ci].Ctrl[k].X = e.snapScalar(cmds[ci].Ctrl[k].X)
					cmds[ci].Ctrl[k].Y = e.snapScalar(cmds[ci].Ctrl[k].Y)
				}
			}
		}
	}
	e.asset.Origin.X = e.snapScalar(e.asset.Origin.X)
	e.asset.Origin.Y = e.snapScalar(e.asset.Origin.Y)
	for i := range e.asset.Hardpoints {
		e.asset.Hardpoints[i].X = e.snapScalar(e.asset.Hardpoints[i].X)
		e.asset.Hardpoints[i].Y = e.snapScalar(e.asset.Hardpoints[i].Y)
	}
	for si := range e.asset.Collisions {
		s := &e.asset.Collisions[si]
		for pi := range s.Points {
			s.Points[pi].X = e.snapScalar(s.Points[pi].X)
			s.Points[pi].Y = e.snapScalar(s.Points[pi].Y)
		}
		s.Radius = e.snapScalar(s.Radius)
	}
	e.markDirty()
	e.status = "snapped all to grid"
}

// selectionLabel describes the currently selected handle for the properties
// panel.
func (e *Editor) selectionLabel() string {
	if !e.hasActive {
		return "none"
	}
	switch e.active.kind {
	case handleVertex:
		return fmt.Sprintf("vertex L%d/P%d/C%d", e.active.layer, e.active.path, e.active.cmd)
	case handleControl:
		return fmt.Sprintf("control L%d/P%d/C%d#%d", e.active.layer, e.active.path, e.active.cmd, e.active.pointIdx)
	case handleOrigin:
		return "origin"
	case handleHardpoint:
		if e.active.index < len(e.asset.Hardpoints) {
			return "hardpoint " + e.asset.Hardpoints[e.active.index].Name
		}
		return "hardpoint"
	case handleCollision:
		if e.active.shapeIdx < len(e.asset.Collisions) {
			return fmt.Sprintf("collision %s #%d", e.asset.Collisions[e.active.shapeIdx].Kind, e.active.shapeIdx)
		}
		return "collision"
	}
	return "none"
}

// addLayer inserts a new empty layer on top and selects it.
func (e *Editor) addLayer() {
	e.asset.Layers = append(e.asset.Layers, asset.Layer{
		Name:        fmt.Sprintf("layer_%d", len(e.asset.Layers)+1),
		Stroke:      strokePalette[0],
		StrokeWidth: 2,
		Fill:        "transparent",
		Glow:        0.8,
		Paths:       []asset.Path{},
	})
	e.layerIdx = len(e.asset.Layers) - 1
	e.hasActive = false
	e.syncPaletteIndices()
	e.markDirty()
	e.status = "layer added: " + e.currentLayer().Name
}

// deleteLayer removes the current layer, keeping at least one layer.
func (e *Editor) deleteLayer() {
	if len(e.asset.Layers) <= 1 {
		e.status = "cannot remove the last layer"
		return
	}
	name := e.asset.Layers[e.layerIdx].Name
	e.asset.Layers = append(e.asset.Layers[:e.layerIdx], e.asset.Layers[e.layerIdx+1:]...)
	if e.layerIdx >= len(e.asset.Layers) {
		e.layerIdx = len(e.asset.Layers) - 1
	}
	e.hasActive = false
	e.syncPaletteIndices()
	e.markDirty()
	e.status = "layer removed: " + name
}

// selectLayer moves the current layer index by delta, wrapping around.
func (e *Editor) selectLayer(delta int) {
	n := len(e.asset.Layers)
	if n == 0 {
		return
	}
	e.layerIdx = ((e.layerIdx+delta)%n + n) % n
	e.hasActive = false
	e.syncPaletteIndices()
	e.status = fmt.Sprintf("layer %d/%d: %s", e.layerIdx+1, n, e.currentLayer().Name)
}

// moveLayer reorders the current layer in draw order: +1 moves it up (in front),
// -1 moves it down (behind).
func (e *Editor) moveLayer(delta int) {
	j := e.layerIdx + delta
	if j < 0 || j >= len(e.asset.Layers) {
		return
	}
	e.asset.Layers[e.layerIdx], e.asset.Layers[j] = e.asset.Layers[j], e.asset.Layers[e.layerIdx]
	e.layerIdx = j
	e.markDirty()
	e.status = fmt.Sprintf("layer moved to %d/%d", e.layerIdx+1, len(e.asset.Layers))
}

// toggleLayerVisibility shows or hides the current layer.
func (e *Editor) toggleLayerVisibility() {
	layer := e.currentLayer()
	layer.Hidden = !layer.Hidden
	e.markDirty()
	if layer.Hidden {
		e.status = "layer hidden"
	} else {
		e.status = "layer shown"
	}
}

// adjustGlow changes the current layer's glow, clamped to [0, 2].
func (e *Editor) adjustGlow(delta float64) {
	layer := e.currentLayer()
	g := layer.Glow + delta
	if g < 0 {
		g = 0
	}
	if g > 2 {
		g = 2
	}
	layer.Glow = g
	e.markDirty()
	e.status = fmt.Sprintf("glow %.1f", g)
}

// layersText renders the layer list for the properties panel, marking the
// current layer and any hidden layers.
func (e *Editor) layersText() string {
	var b strings.Builder
	b.WriteString("layers:")
	for i := range e.asset.Layers {
		l := &e.asset.Layers[i]
		mark := "  "
		if i == e.layerIdx {
			mark = "> "
		}
		hidden := ""
		if l.Hidden {
			hidden = " (hidden)"
		}
		fmt.Fprintf(&b, "\n%s%d %s%s", mark, i, l.Name, hidden)
	}
	return b.String()
}

// hasDrawablePoint reports whether the command list still has a move or line.
func hasDrawablePoint(cmds []asset.Command) bool {
	for _, c := range cmds {
		if c.Op == asset.OpMoveTo || c.Op == asset.OpLineTo {
			return true
		}
	}
	return false
}

// cycleStroke advances the stroke color of the current layer.
func (e *Editor) cycleStroke() {
	e.strokeIdx = (e.strokeIdx + 1) % len(strokePalette)
	e.currentLayer().Stroke = strokePalette[e.strokeIdx]
	e.markDirty()
	e.status = "stroke " + strokePalette[e.strokeIdx]
}

// cycleFill advances the fill color of the current layer.
func (e *Editor) cycleFill() {
	e.fillIdx = (e.fillIdx + 1) % len(fillPalette)
	e.currentLayer().Fill = fillPalette[e.fillIdx]
	e.markDirty()
	e.status = "fill " + fillPalette[e.fillIdx]
}
