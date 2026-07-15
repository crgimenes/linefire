package mapeditor

import (
	"fmt"
	"math"

	"linefire/asset"
	"linefire/level"
)

// handleKind classifies a draggable point in the level.
type handleKind int

const (
	hWallVertex handleKind = iota
	hControl               // a curve command's Bézier control point
	hPlayerStart
	hSpawn
	hEntry
	hZone
)

// handle references a single editable point.
type handle struct {
	kind handleKind
	x, y float64

	layer, path, cmd   int // hWallVertex / hControl
	index              int // hSpawn / hEntry
	shapeIdx, pointIdx int // hZone shape/point; for hControl, pointIdx = control index
}

func (h handle) same(o handle) bool {
	if h.kind != o.kind {
		return false
	}
	switch h.kind {
	case hWallVertex:
		return h.layer == o.layer && h.path == o.path && h.cmd == o.cmd
	case hControl:
		return h.layer == o.layer && h.path == o.path && h.cmd == o.cmd && h.pointIdx == o.pointIdx
	case hSpawn, hEntry:
		return h.index == o.index
	case hZone:
		return h.shapeIdx == o.shapeIdx && h.pointIdx == o.pointIdx
	default:
		return true
	}
}

// collectHandles gathers every editable point in the level.
func (e *MapEditor) collectHandles() []handle {
	var hs []handle
	for li := range e.level.Walls {
		for pi := range e.level.Walls[li].Paths {
			for ci, c := range e.level.Walls[li].Paths[pi].Commands {
				if c.Op == asset.OpClose {
					continue
				}
				hs = append(hs, handle{kind: hWallVertex, x: c.X, y: c.Y, layer: li, path: pi, cmd: ci})
				for k, cp := range c.Ctrl {
					hs = append(hs, handle{kind: hControl, x: cp.X, y: cp.Y, layer: li, path: pi, cmd: ci, pointIdx: k})
				}
			}
		}
	}
	hs = append(hs, handle{kind: hPlayerStart, x: e.level.PlayerStart.X, y: e.level.PlayerStart.Y})
	for i, s := range e.level.Spawns {
		hs = append(hs, handle{kind: hSpawn, x: s.X, y: s.Y, index: i})
	}
	for i, en := range e.level.Entries {
		hs = append(hs, handle{kind: hEntry, x: en.X, y: en.Y, index: i})
	}
	for si := range e.level.Zones {
		for pi, p := range e.level.Zones[si].Points {
			hs = append(hs, handle{kind: hZone, x: p.X, y: p.Y, shapeIdx: si, pointIdx: pi})
		}
	}
	return hs
}

// moveHandle writes a new world position into the point a handle refers to.
func (e *MapEditor) moveHandle(h handle, x, y float64) {
	switch h.kind {
	case hWallVertex:
		if h.layer < len(e.level.Walls) {
			layer := &e.level.Walls[h.layer]
			if h.path < len(layer.Paths) && h.cmd < len(layer.Paths[h.path].Commands) {
				layer.Paths[h.path].Commands[h.cmd].X = x
				layer.Paths[h.path].Commands[h.cmd].Y = y
			}
		}
	case hControl:
		if h.layer < len(e.level.Walls) {
			layer := &e.level.Walls[h.layer]
			if h.path < len(layer.Paths) && h.cmd < len(layer.Paths[h.path].Commands) {
				ctrl := layer.Paths[h.path].Commands[h.cmd].Ctrl
				if h.pointIdx < len(ctrl) {
					ctrl[h.pointIdx] = asset.Point{X: x, Y: y}
				}
			}
		}
	case hPlayerStart:
		e.level.PlayerStart.X = x
		e.level.PlayerStart.Y = y
	case hSpawn:
		if h.index < len(e.level.Spawns) {
			e.level.Spawns[h.index].X = x
			e.level.Spawns[h.index].Y = y
		}
	case hEntry:
		if h.index < len(e.level.Entries) {
			e.level.Entries[h.index].X = x
			e.level.Entries[h.index].Y = y
		}
	case hZone:
		if h.shapeIdx < len(e.level.Zones) {
			z := &e.level.Zones[h.shapeIdx]
			if h.pointIdx < len(z.Points) {
				z.Points[h.pointIdx] = asset.Point{X: x, Y: y}
			}
		}
	}
	e.markDirty()
}

// penHandleMin is the smallest drag, in world units, that counts as a curve
// handle rather than a plain click (a corner). Grid snapping usually quantizes a
// real drag well past this; it matters mainly with sub-pixel (Alt) placement.
const penHandleMin = 3.0

// atDraftStart reports whether the cursor sits on the draft's start point and the
// draft has enough points to close into a shape.
func (e *MapEditor) atDraftStart() bool {
	return e.drafting && len(e.draft) >= 3 && e.cursor.X == e.draft[0].X && e.cursor.Y == e.draft[0].Y
}

// beginWallNode starts placing a vertex at the press point; the drag until release
// becomes its curve handle (see finishWallNode).
func (e *MapEditor) beginWallNode(p asset.Point) {
	if !e.drafting {
		e.drafting = true
		e.draftLastHandle = asset.Point{}
		e.status = "wall: drag = curve, click = corner; click start = close"
	}
	e.penActive = true
	e.penAnchor = p
}

// finishWallNode commits the vertex on release: a near-zero handle makes a corner
// (line), otherwise a smooth cubic using the previous vertex's out-handle and this
// vertex's mirrored in-handle.
func (e *MapEditor) finishWallNode(end asset.Point) {
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

// closeWall closes the draft into a filled shape and commits it.
func (e *MapEditor) closeWall() {
	e.draft = append(e.draft, asset.Command{Op: asset.OpClose})
	e.commitWall()
}

// commitWall appends the wall draft to the current wall layer.
func (e *MapEditor) commitWall() {
	if len(e.draft) >= 2 {
		cmds := make([]asset.Command, len(e.draft))
		copy(cmds, e.draft)
		layer := e.currentWallLayer()
		layer.Paths = append(layer.Paths, asset.Path{Commands: cmds})
		e.markDirty()
		e.status = fmt.Sprintf("wall added (%d points)", len(cmds))
	} else {
		e.status = "wall discarded (needs 2+ points)"
	}
	e.draft = nil
	e.drafting = false
	e.penActive = false
	e.draftLastHandle = asset.Point{}
}

// placeSpawn adds a spawn at the cursor from the selected palette entry. The kind
// is intrinsic to the entry — an asset declares what it is, a map places a portal
// to itself — so what you place is always what it says it is, no role to choose.
func (e *MapEditor) placeSpawn(x, y float64) {
	entry, ok := e.currentPaletteEntry()
	if !ok {
		e.status = "palette empty (run in a dir with asset/map JSONs)"
		return
	}
	if entry.isMap {
		e.placePortal(entry, x, y)
		return
	}
	if entry.isExit {
		e.placeEntry(x, y)
		return
	}
	kind := assetKind(entry)
	s := level.Spawn{
		Name:  fmt.Sprintf("%s_%d", kind, len(e.level.Spawns)+1),
		Asset: entry.ref,
		Kind:  kind,
		X:     x,
		Y:     y,
		Angle: -90,
	}
	e.level.Spawns = append(e.level.Spawns, s)
	e.selectSpawn(len(e.level.Spawns) - 1)
	e.markDirty()
	e.status = "placed " + s.Name + " (" + entry.name + ")"
}

// selectSpawn makes the spawn at idx the active selection, so its properties show
// in the panel right after placing — no need to switch to the Select tool.
func (e *MapEditor) selectSpawn(idx int) {
	if idx < 0 || idx >= len(e.level.Spawns) {
		return
	}
	s := e.level.Spawns[idx]
	e.active = handle{kind: hSpawn, x: s.X, y: s.Y, index: idx}
	e.hasActive = true
}

// selectEntry makes the entry at idx the active selection (see selectSpawn).
func (e *MapEditor) selectEntry(idx int) {
	if idx < 0 || idx >= len(e.level.Entries) {
		return
	}
	en := e.level.Entries[idx]
	e.active = handle{kind: hEntry, x: en.X, y: en.Y, index: idx}
	e.hasActive = true
}

// placePortal drops a portal spawn that travels to the selected map. Edit the
// target in the panel to add a ":label" entry (or point it at the current map for
// an in-map teleport).
func (e *MapEditor) placePortal(entry paletteEntry, x, y float64) {
	s := level.Spawn{
		Name:   fmt.Sprintf("portal_%d", len(e.level.Spawns)+1),
		Asset:  entry.ref, // the portal visual
		Kind:   "portal",
		Target: entry.name, // the destination map stem
		X:      x,
		Y:      y,
		Angle:  -90,
	}
	e.level.Spawns = append(e.level.Spawns, s)
	e.selectSpawn(len(e.level.Spawns) - 1)
	e.markDirty()
	e.status = "placed portal -> " + entry.name
}

// placeEntry adds a named arrival point at the cursor. Its name is what portals
// in other maps reference; edit it in the panel after placing.
func (e *MapEditor) placeEntry(x, y float64) {
	en := level.Entry{
		Name:  fmt.Sprintf("entry_%d", len(e.level.Entries)+1),
		X:     x,
		Y:     y,
		Angle: -90,
	}
	e.level.Entries = append(e.level.Entries, en)
	e.selectEntry(len(e.level.Entries) - 1)
	e.markDirty()
	e.status = "placed " + en.Name
}

// assetKind returns the intrinsic category of a palette entry: "map" for a map,
// "exit" for a portal-exit point, the asset's declared kind otherwise, defaulting
// to "enemy" when unset.
func assetKind(entry paletteEntry) string {
	switch {
	case entry.isMap:
		return "map"
	case entry.isExit:
		return "exit"
	case entry.asset != nil && entry.asset.Kind != "":
		return entry.asset.Kind
	default:
		return "enemy"
	}
}

// placeZonePoint adds a click to the in-progress zone and finalizes it.
func (e *MapEditor) placeZonePoint(x, y float64) {
	switch e.zoneKind {
	case level.ZoneCircle:
		if len(e.zoneDraft) == 0 {
			e.zoneDraft = []asset.Point{{X: x, Y: y}}
			e.status = "zone circle: click radius"
			return
		}
		c := e.zoneDraft[0]
		r := math.Round(math.Hypot(x-c.X, y-c.Y))
		e.addZone(level.Zone{Name: e.zoneName(), Kind: level.ZoneCircle, Points: []asset.Point{c}, Radius: r})
	default: // rect
		if len(e.zoneDraft) == 0 {
			e.zoneDraft = []asset.Point{{X: x, Y: y}}
			e.status = "zone rect: click opposite corner"
			return
		}
		e.addZone(level.Zone{Name: e.zoneName(), Kind: level.ZoneRect, Points: []asset.Point{e.zoneDraft[0], {X: x, Y: y}}})
	}
}

func (e *MapEditor) zoneName() string {
	return fmt.Sprintf("zone_%d", len(e.level.Zones)+1)
}

func (e *MapEditor) addZone(z level.Zone) {
	e.level.Zones = append(e.level.Zones, z)
	e.zoneDraft = nil
	e.markDirty()
	e.status = "added " + z.Name
}

// cycleZoneKind toggles the zone shape being drawn.
func (e *MapEditor) cycleZoneKind() {
	if e.zoneKind == level.ZoneRect {
		e.zoneKind = level.ZoneCircle
	} else {
		e.zoneKind = level.ZoneRect
	}
	e.zoneDraft = nil
	e.status = "zone shape: " + e.zoneKind
}

// deleteSelectedOrHint deletes the selected handle, or sets a status hint when
// nothing is selected. It backs the toolbar Del button (the Delete/Backspace key
// goes through removeLastOrSelected so it can also trim a wall draft).
func (e *MapEditor) deleteSelectedOrHint() {
	if !e.hasActive {
		e.status = "nothing selected to delete"
		return
	}
	e.deleteSelected()
}

// deleteSelected removes whatever handle is selected.
func (e *MapEditor) deleteSelected() {
	if !e.hasActive {
		return
	}
	switch e.active.kind {
	case hWallVertex:
		e.deleteWallVertex(e.active)
	case hSpawn:
		if e.active.index < len(e.level.Spawns) {
			e.level.Spawns = append(e.level.Spawns[:e.active.index], e.level.Spawns[e.active.index+1:]...)
			e.clearSelection("spawn removed")
		}
	case hEntry:
		if e.active.index < len(e.level.Entries) {
			e.level.Entries = append(e.level.Entries[:e.active.index], e.level.Entries[e.active.index+1:]...)
			e.clearSelection("entry removed")
		}
	case hZone:
		if e.active.shapeIdx < len(e.level.Zones) {
			e.level.Zones = append(e.level.Zones[:e.active.shapeIdx], e.level.Zones[e.active.shapeIdx+1:]...)
			e.clearSelection("zone removed")
		}
	case hPlayerStart:
		e.status = "player start cannot be removed"
	}
}

func (e *MapEditor) clearSelection(msg string) {
	e.hasActive = false
	e.markDirty()
	e.status = msg
}

// deleteWallVertex removes a wall vertex by OPENING the path at that point rather than healing the
// gap: deleting a node leaves a real opening — a closed loop reopens where the vertex was, an open
// path splits into two — so you can cut a shape apart and stitch it back with the Join tool instead
// of the editor silently reconnecting the ends. Pieces that fall below two points are dropped.
func (e *MapEditor) deleteWallVertex(h handle) {
	if h.layer >= len(e.level.Walls) {
		return
	}
	layer := &e.level.Walls[h.layer]
	if h.path >= len(layer.Paths) {
		return
	}
	pts, closed := drawableCommands(layer.Paths[h.path].Commands)
	if h.cmd >= len(pts) {
		return
	}
	pieces := cutAtVertex(pts, h.cmd, closed)

	rebuilt := make([]asset.Path, 0, len(layer.Paths)+len(pieces))
	rebuilt = append(rebuilt, layer.Paths[:h.path]...)
	for _, pc := range pieces {
		rebuilt = append(rebuilt, asset.Path{Commands: pc})
	}
	rebuilt = append(rebuilt, layer.Paths[h.path+1:]...)
	layer.Paths = rebuilt
	e.clearSelection("vertex removed (gap left open)")
}

// rotateSelected turns a selected spawn or the player start by delta degrees.
func (e *MapEditor) rotateSelected(delta float64) {
	switch e.active.kind {
	case hPlayerStart:
		e.level.PlayerStart.Angle = normalizeAngle(e.level.PlayerStart.Angle + delta)
		e.markDirty()
	case hSpawn:
		if e.active.index < len(e.level.Spawns) {
			e.level.Spawns[e.active.index].Angle = normalizeAngle(e.level.Spawns[e.active.index].Angle + delta)
			e.markDirty()
		}
	case hEntry:
		if e.active.index < len(e.level.Entries) {
			e.level.Entries[e.active.index].Angle = normalizeAngle(e.level.Entries[e.active.index].Angle + delta)
			e.markDirty()
		}
	}
}

func normalizeAngle(a float64) float64 {
	for a > 180 {
		a -= 360
	}
	for a <= -180 {
		a += 360
	}
	return a
}

// snapAllToGrid rounds every level coordinate to the active grid.
func (e *MapEditor) snapAllToGrid() {
	for li := range e.level.Walls {
		for pi := range e.level.Walls[li].Paths {
			cmds := e.level.Walls[li].Paths[pi].Commands
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
	e.level.PlayerStart.X = e.snapScalar(e.level.PlayerStart.X)
	e.level.PlayerStart.Y = e.snapScalar(e.level.PlayerStart.Y)
	for i := range e.level.Spawns {
		e.level.Spawns[i].X = e.snapScalar(e.level.Spawns[i].X)
		e.level.Spawns[i].Y = e.snapScalar(e.level.Spawns[i].Y)
	}
	for i := range e.level.Entries {
		e.level.Entries[i].X = e.snapScalar(e.level.Entries[i].X)
		e.level.Entries[i].Y = e.snapScalar(e.level.Entries[i].Y)
	}
	for si := range e.level.Zones {
		z := &e.level.Zones[si]
		for pi := range z.Points {
			z.Points[pi].X = e.snapScalar(z.Points[pi].X)
			z.Points[pi].Y = e.snapScalar(z.Points[pi].Y)
		}
		z.Radius = e.snapScalar(z.Radius)
	}
	e.markDirty()
	e.status = "snapped all to grid"
}

// selectionLabel describes the selected handle for the panel.
func (e *MapEditor) selectionLabel() string {
	if !e.hasActive {
		return "none"
	}
	switch e.active.kind {
	case hWallVertex:
		return fmt.Sprintf("wall L%d/P%d/C%d", e.active.layer, e.active.path, e.active.cmd)
	case hControl:
		return fmt.Sprintf("curve control #%d", e.active.pointIdx)
	case hPlayerStart:
		return "player start"
	case hSpawn:
		if e.active.index < len(e.level.Spawns) {
			s := e.level.Spawns[e.active.index]
			kind := s.Kind
			if kind == "" {
				kind = "spawn"
			}
			return fmt.Sprintf("%s #%d", kind, e.active.index)
		}
		return "spawn"
	case hEntry:
		if e.active.index < len(e.level.Entries) {
			return fmt.Sprintf("entry %q", e.level.Entries[e.active.index].Name)
		}
		return "entry"
	case hZone:
		if e.active.shapeIdx < len(e.level.Zones) {
			return fmt.Sprintf("zone %s #%d", e.level.Zones[e.active.shapeIdx].Kind, e.active.shapeIdx)
		}
		return "zone"
	}
	return "none"
}
