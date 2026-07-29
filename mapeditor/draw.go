package mapeditor

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/editorkit"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/render"
)

// UI colors.
var (
	colorBackground = color.RGBA{0x10, 0x12, 0x18, 0xff}
	colorCanvasBg   = color.RGBA{0x08, 0x0a, 0x10, 0xff}
	colorPanelBg    = color.RGBA{0x16, 0x1a, 0x22, 0xff}
	colorToolbarBg  = color.RGBA{0x1e, 0x22, 0x2c, 0xff}
	colorGridDot    = color.RGBA{0x32, 0x3a, 0x46, 0xff}
	colorAxis       = color.RGBA{0x44, 0x4e, 0x5e, 0xff}

	colorVertex   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colorStart    = color.RGBA{0x80, 0xff, 0xff, 0xff}
	colorEnemy    = color.RGBA{0xff, 0x60, 0x60, 0xff}
	colorPower    = color.RGBA{0x60, 0xff, 0x90, 0xff}
	colorZone     = color.RGBA{0xff, 0xc0, 0x40, 0xff}
	colorPortal   = color.RGBA{0xc0, 0x60, 0xff, 0xff}
	colorEntry    = color.RGBA{0x80, 0xff, 0xc0, 0xff}
	colorControl  = color.RGBA{0xff, 0x80, 0xff, 0xff} // curve control-point handles
	colorDraft    = color.RGBA{0xff, 0xff, 0x80, 0xff}
	colorClose    = color.RGBA{0x60, 0xff, 0x90, 0xff} // wall draft can close into a loop
	colorSelect   = color.RGBA{0x80, 0xff, 0xff, 0xff}
	colorSnap     = color.RGBA{0xff, 0xff, 0x40, 0xff}
	colorJoin     = color.RGBA{0xff, 0xc0, 0x40, 0xff} // armed join end + rubber band
	colorLooseEnd = color.RGBA{0xff, 0x50, 0x50, 0xff} // open-path endpoints: the joinable red squares

	// Negative-space preview: exterior tint and the carved (playable) interior.
	colorPreviewFill = color.RGBA{0x14, 0x38, 0x44, 0xff}
	colorPreviewNav  = color.RGBA{0x06, 0x08, 0x0c, 0xff}
)

// Draw renders the editor for one frame.
func (e *MapEditor) Draw(screen *ebiten.Image) {
	screen.Fill(colorBackground)
	e.drawCanvas(screen)
	e.drawToolbar(screen)
	e.drawPanel(screen)
	if e.colorOpen {
		e.drawColorPanel(screen)
	}
	if e.newAssetOpen {
		e.drawNewAssetDialog(screen)
	}
}

// drawCanvas draws the world: grid, bounds, walls (with glow), spawns, zones,
// the player start and editing overlays.
func (e *MapEditor) drawCanvas(screen *ebiten.Image) {
	r := e.canvasRect()
	view := e.canvasView()
	canvas := screen.SubImage(r).(*ebiten.Image)

	if e.preview {
		// Negative-space look: the exterior tint fills the canvas, then the
		// reachable interior is carved black (even-odd of the closed wall paths).
		canvas.Fill(colorPreviewFill)
		e.carveNavigable(canvas, view)
	} else {
		vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), colorCanvasBg, false)
		if e.level.Editor.GridEnabled && e.level.Editor.GridSize > 0 {
			editorkit.DrawDotGrid(canvas, view, r, e.level.Editor.GridSize, colorGridDot, colorAxis, 0, 0)
		}
		// The map is unbounded; the grid dots and origin axes orient the user
		// instead of a fixed world-bounds box.
	}

	// Reference backdrop sits behind the walls so the author traces over it (never saved).
	e.drawBackdrop(canvas, view)

	// Walls (crisp + glow).
	render.DrawLayers(canvas, e.level.Walls, view, true)
	if e.glowEnabled {
		e.glow.Apply(screen, e.level.Walls, view, r, render.GlowOptions{
			Variant:   render.GlowStable,
			Time:      float64(ebiten.Tick()) / float64(ebiten.TPS()),
			Intensity: 1,
			AntiAlias: true,
		})
	}

	e.drawSpawns(canvas, view)
	e.drawZones(canvas, view)
	e.drawEntries(canvas, view)
	e.drawPlayerStart(canvas, view)
	e.drawWallDraft(canvas, view)
	e.drawOverlayHandles(canvas, view)

	// Loose-end join in progress: ring the armed end and rubber-band to the cursor, so the
	// second click reads as "connect to here" (kutta's affordance).
	if e.hasJoinFrom {
		sx, sy := view.Project(e.joinFrom.x, e.joinFrom.y)
		vector.StrokeCircle(canvas, sx, sy, 9, 2, colorJoin, true)
		cx, cy := view.Project(e.cursor.X, e.cursor.Y)
		vector.StrokeLine(canvas, sx, sy, cx, cy, 1.5, colorJoin, true)
	}
}

// carveNavigable fills the reachable interior (inside the boundary but outside the
// solid blocks — the even-odd region of the closed wall paths) with the dark
// playable color, mirroring the game's negative-space carve so the author can
// preview which space is navigable. Open paths won't enclose anything (the same
// caveat as the game: the negative-space style needs closed shapes).
func (e *MapEditor) carveNavigable(canvas *ebiten.Image, view render.View) {
	var path vector.Path
	for li := range e.level.Walls {
		for _, wp := range e.level.Walls[li].Paths {
			p := wp.Flatten() // curved walls carve as their flattened segments
			for _, c := range p.Commands {
				switch c.Op {
				case asset.OpMoveTo:
					sx, sy := view.Project(c.X, c.Y)
					path.MoveTo(sx, sy)
				case asset.OpLineTo:
					sx, sy := view.Project(c.X, c.Y)
					path.LineTo(sx, sy)
				case asset.OpClose:
					path.Close()
				}
			}
		}
	}
	var cs ebiten.ColorScale
	cs.ScaleWithColor(colorPreviewNav)
	vector.FillPath(canvas, &path, &vector.FillOptions{FillRule: vector.FillRuleEvenOdd}, &vector.DrawPathOptions{
		ColorScale: cs,
		AntiAlias:  true,
	})
}

// drawSpawns draws every spawn as ghost geometry plus a marker, colored by the
// spawn's category.
func (e *MapEditor) drawSpawns(dst *ebiten.Image, view render.View) {
	for _, s := range e.level.Spawns {
		e.drawSpawn(dst, view, s, spawnMarkerColor(s.Kind))
	}
}

// spawnMarkerColor picks a marker color from a spawn category: red for enemies,
// violet for portals, green for everything else (pickups).
func spawnMarkerColor(kind string) color.RGBA {
	switch kind {
	case "enemy":
		return colorEnemy
	case "portal":
		return colorPortal
	default:
		return colorPower
	}
}

func (e *MapEditor) drawSpawn(dst *ebiten.Image, view render.View, s level.Spawn, col color.RGBA) {
	sx, sy := view.Project(s.X, s.Y)
	a := e.assetByRef(s.Asset)
	if a != nil {
		gv := render.View{
			OffsetX: view.OffsetX + (s.X-a.Origin.X)*view.Scale,
			OffsetY: view.OffsetY + (s.Y-a.Origin.Y)*view.Scale,
			Scale:   view.Scale,
		}
		render.DrawLayers(dst, a.Layers, gv, true)
	}
	vector.FillCircle(dst, sx, sy, 3, col, true)
	rad := s.Angle * math.Pi / 180
	vector.StrokeLine(dst, sx, sy, sx+float32(math.Cos(rad))*12, sy+float32(math.Sin(rad))*12, 1, col, true)
}

// drawZones outlines trigger zones.
func (e *MapEditor) drawZones(dst *ebiten.Image, view render.View) {
	for i := range e.level.Zones {
		strokeZone(dst, view, e.level.Zones[i].Kind, e.level.Zones[i].Points, e.level.Zones[i].Radius)
	}
	// In-progress zone preview.
	if e.tool == toolZone && len(e.zoneDraft) > 0 {
		if e.zoneKind == level.ZoneCircle {
			c := e.zoneDraft[0]
			r := math.Hypot(e.cursor.X-c.X, e.cursor.Y-c.Y)
			strokeZone(dst, view, level.ZoneCircle, []asset.Point{c}, r)
		} else {
			strokeZone(dst, view, level.ZoneRect, []asset.Point{e.zoneDraft[0], e.cursor}, 0)
		}
	}
}

func strokeZone(dst *ebiten.Image, view render.View, kind string, pts []asset.Point, radius float64) {
	switch kind {
	case level.ZoneCircle:
		if len(pts) >= 1 {
			cx, cy := view.Project(pts[0].X, pts[0].Y)
			vector.StrokeCircle(dst, cx, cy, float32(radius*view.Scale), 1, colorZone, true)
		}
	case level.ZoneRect:
		if len(pts) >= 2 {
			x0, y0 := view.Project(pts[0].X, pts[0].Y)
			x1, y1 := view.Project(pts[1].X, pts[1].Y)
			vector.StrokeRect(dst, min(x0, x1), min(y0, y1), abs32(x1-x0), abs32(y1-y0), 1, colorZone, true)
		}
	}
}

// drawPlayerStart marks where the ship begins, with a facing tick.
func (e *MapEditor) drawPlayerStart(dst *ebiten.Image, view render.View) {
	sx, sy := view.Project(e.level.PlayerStart.X, e.level.PlayerStart.Y)
	vector.StrokeCircle(dst, sx, sy, 6, 1.5, colorStart, true)
	vector.StrokeLine(dst, sx-9, sy, sx+9, sy, 1, colorStart, true)
	vector.StrokeLine(dst, sx, sy-9, sx, sy+9, 1, colorStart, true)
	rad := e.level.PlayerStart.Angle * math.Pi / 180
	vector.StrokeLine(dst, sx, sy, sx+float32(math.Cos(rad))*16, sy+float32(math.Sin(rad))*16, 1.5, colorStart, true)
}

// drawEntries marks each named arrival point with a diamond, a facing tick and
// its name, so portal destinations are visible and easy to wire up.
func (e *MapEditor) drawEntries(dst *ebiten.Image, view render.View) {
	for _, en := range e.level.Entries {
		sx, sy := view.Project(en.X, en.Y)
		vector.StrokeLine(dst, sx, sy-7, sx+7, sy, 1.5, colorEntry, true)
		vector.StrokeLine(dst, sx+7, sy, sx, sy+7, 1.5, colorEntry, true)
		vector.StrokeLine(dst, sx, sy+7, sx-7, sy, 1.5, colorEntry, true)
		vector.StrokeLine(dst, sx-7, sy, sx, sy-7, 1.5, colorEntry, true)
		rad := en.Angle * math.Pi / 180
		vector.StrokeLine(dst, sx, sy, sx+float32(math.Cos(rad))*14, sy+float32(math.Sin(rad))*14, 1.5, colorEntry, true)
		ebitenutil.DebugPrintAt(dst, en.Name, int(sx)+9, int(sy)-7)
	}
}

// drawWallDraft renders the wall being drawn (curves flattened) plus the pen
// handle preview, the rubber-band segment and the close indicator.
func (e *MapEditor) drawWallDraft(dst *ebiten.Image, view render.View) {
	if !e.drafting || len(e.draft) == 0 {
		return
	}
	// Committed part: flatten so curve segments preview as smooth polylines.
	flat := asset.Path{Commands: e.draft}.Flatten().Commands
	var px, py float32
	started := false
	for _, c := range flat {
		if c.Op == asset.OpClose {
			continue
		}
		sx, sy := view.Project(c.X, c.Y)
		if started {
			vector.StrokeLine(dst, px, py, sx, sy, 1.5, colorDraft, true)
		}
		px, py, started = sx, sy, true
	}
	// Vertex markers and control-point hints at the real anchors.
	for _, c := range e.draft {
		if c.Op == asset.OpClose {
			continue
		}
		sx, sy := view.Project(c.X, c.Y)
		vector.FillRect(dst, sx-2, sy-2, 4, 4, colorDraft, false)
		for _, cp := range c.Ctrl {
			cx, cy := view.Project(cp.X, cp.Y)
			vector.StrokeLine(dst, sx, sy, cx, cy, 1, colorClose, true)
			vector.StrokeCircle(dst, cx, cy, 3, 1, colorClose, true)
		}
	}
	// Pen drag preview: the handle being pulled (both sides, since it is mirrored).
	if e.penActive && e.mouseInCanvas {
		ax, ay := view.Project(e.penAnchor.X, e.penAnchor.Y)
		hx, hy := view.Project(e.cursor.X, e.cursor.Y)
		mx, my := view.Project(2*e.penAnchor.X-e.cursor.X, 2*e.penAnchor.Y-e.cursor.Y)
		vector.StrokeLine(dst, ax, ay, hx, hy, 1, colorClose, true)
		vector.StrokeLine(dst, ax, ay, mx, my, 1, colorClose, true)
		vector.FillRect(dst, ax-2, ay-2, 4, 4, colorDraft, false)
	}
	// Rubber-band from the last vertex to the cursor (when not dragging a handle).
	closing := e.atDraftStart()
	if e.mouseInCanvas && !e.penActive {
		last := e.draft[len(e.draft)-1]
		x0, y0 := view.Project(last.X, last.Y)
		x1, y1 := view.Project(e.cursor.X, e.cursor.Y)
		band := colorDraft
		if closing {
			band = colorClose
		}
		vector.StrokeLine(dst, x0, y0, x1, y1, 1.5, band, true)
	}
	if closing {
		sx, sy := view.Project(e.draft[0].X, e.draft[0].Y)
		vector.StrokeCircle(dst, sx, sy, 7, 2, colorClose, true)
	}
}

// drawOverlayHandles draws wall vertex markers, zone point markers, selection
// and snap highlights, plus the cursor readout.
func (e *MapEditor) drawOverlayHandles(dst *ebiten.Image, view render.View) {
	for li := range e.level.Walls {
		for _, p := range e.level.Walls[li].Paths {
			var prevX, prevY float32
			havePrev := false
			for _, c := range p.Commands {
				if c.Op == asset.OpClose {
					continue
				}
				sx, sy := view.Project(c.X, c.Y)
				vector.FillRect(dst, sx-2, sy-2, 4, 4, colorVertex, false)
				// Curve control handles: line to the anchor they pull (control 0 to
				// the previous vertex, control 1 to this one) + a grab dot.
				for k, cp := range c.Ctrl {
					cx, cy := view.Project(cp.X, cp.Y)
					ax, ay := sx, sy
					if k == 0 && havePrev {
						ax, ay = prevX, prevY
					}
					vector.StrokeLine(dst, ax, ay, cx, cy, 1, colorControl, true)
					vector.StrokeCircle(dst, cx, cy, 3, 1, colorControl, true)
				}
				prevX, prevY, havePrev = sx, sy, true
			}
		}
	}
	// Loose ends of OPEN walls draw as bigger red squares: the joinable points (click two to
	// weld). Always visible, so an accidentally open wall is impossible to miss.
	for li := range e.level.Walls {
		for pi := range e.level.Walls[li].Paths {
			p := &e.level.Walls[li].Paths[pi]
			if p.Closed() || len(p.Commands) < 2 {
				continue
			}
			last := lastDrawableIdx(p.Commands)
			for _, ci := range []int{0, last} {
				sx, sy := view.Project(p.Commands[ci].X, p.Commands[ci].Y)
				vector.FillRect(dst, sx-3, sy-3, 6, 6, colorLooseEnd, false)
			}
		}
	}

	for si := range e.level.Zones {
		for _, p := range e.level.Zones[si].Points {
			sx, sy := view.Project(p.X, p.Y)
			vector.FillRect(dst, sx-2, sy-2, 4, 4, colorZone, false)
		}
	}

	if e.hasActive {
		sx, sy := view.Project(e.active.x, e.active.y)
		vector.StrokeCircle(dst, sx, sy, 7, 1, colorSelect, true)
	}
	if e.snapActive {
		sx, sy := view.Project(e.snapPoint.X, e.snapPoint.Y)
		vector.StrokeCircle(dst, sx, sy, 7, 1.5, colorSnap, true)
	}
	if e.mouseInCanvas {
		sx, sy := view.Project(e.cursor.X, e.cursor.Y)
		ebitenutil.DebugPrintAt(dst, fmt.Sprintf("%g, %g", e.cursor.X, e.cursor.Y), int(sx)+8, int(sy)-18)
	}
}

// drawToolbar draws the tool bar with the active tool highlighted.
func (e *MapEditor) drawToolbar(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, toolbarHeight, colorToolbarBg, false)
	e.bar.Render(screen)
}

// drawPanel draws the right-hand properties, asset list and selection panel.
func (e *MapEditor) drawPanel(screen *ebiten.Image) {
	px := screenWidth - panelWidth
	vector.FillRect(screen, float32(px), toolbarHeight, panelWidth, screenHeight-toolbarHeight, colorPanelBg, false)
	ebitenutil.DebugPrintAt(screen, e.propertiesText(), px+16, toolbarHeight+10)
	e.assetList.Render(screen)
	e.drawAssetThumbs(screen)
	if e.spawnPanelActive() {
		e.spawnPanel.Render(screen)
	}
	// No shortcut legend: it overflowed the panel and was unreadable. The full
	// reference lives in the README; the toolbar buttons carry the essentials.

	// The open filter dropdown floats ABOVE everything in the panel: an opaque backdrop hides the
	// asset rows underneath, then the category list renders on top.
	if e.filterOpen {
		r := e.filterOverlayRect()
		vector.FillRect(screen, float32(r.x0-4), float32(r.y0-4), float32(r.x1-r.x0+8), float32(r.y1-r.y0+8), colorPanelBg, false)
		e.filterList.Render(screen)
	}
}

// propertiesText builds the multi-line panel summary.
func (e *MapEditor) propertiesText() string {
	palette := "none"
	entry, ok := e.currentPaletteEntry()
	if ok {
		palette = fmt.Sprintf("%s (%d/%d)", entry.name, e.paletteIdx+1, len(e.palette))
	}
	grid := fmt.Sprintf("%s (size %g)", onOff(e.level.Editor.GridEnabled), e.level.Editor.GridSize)
	if e.level.Editor.SnapToGrid {
		grid += ", snap"
	}
	saved := "saved"
	if e.dirty {
		saved = "UNSAVED *"
	}
	wallPaths, openPaths := 0, 0
	for i := range e.level.Walls {
		for _, p := range e.level.Walls[i].Paths {
			wallPaths++
			if !p.Closed() {
				openPaths++
			}
		}
	}
	enemies, pickups := 0, 0
	for _, s := range e.level.Spawns {
		if s.Kind == "enemy" {
			enemies++
		} else {
			pickups++
		}
	}
	walls := fmt.Sprintf("%d path(s)", wallPaths)
	if openPaths > 0 {
		// Open walls leak in the negative-space flood fill — flag them.
		walls += fmt.Sprintf("  !%d OPEN", openPaths)
	}
	zoneKind := ""
	if e.tool == toolZone {
		zoneKind = " [+" + e.zoneKind + "]"
	}
	return fmt.Sprintf(
		"name:    %s\nsize:    %.0fx%.0f\ntool:    %s%s\nsel:     %s\nzoom:    %.2fx\n\nstart:   %.0f,%.0f @%.0f\nwalls:   %s\nspawns:  %d (%de %dp)\nzones:   %d\n\nplace:   %s\ngrid:    %s\nglow:    %s\n\nfile:    %s (%s)\nstatus:  %s",
		e.level.Name,
		e.level.Size.W, e.level.Size.H,
		toolName(e.tool), zoneKind,
		e.selectionLabel(),
		e.cam.View.Scale,
		e.level.PlayerStart.X, e.level.PlayerStart.Y, e.level.PlayerStart.Angle,
		walls,
		len(e.level.Spawns), enemies, pickups,
		len(e.level.Zones),
		palette,
		grid,
		onOff(e.glowEnabled),
		e.savePathLabel(), saved,
		e.status,
	)
}

// abs32 returns the absolute value of a float32.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
