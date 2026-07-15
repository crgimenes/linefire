package editor

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"linefire/asset"
	"linefire/editorkit"
	"linefire/render"
)

// UI colors.
var (
	colorBackground = color.RGBA{0x10, 0x12, 0x18, 0xff}
	colorCanvasBg   = color.RGBA{0x08, 0x0a, 0x10, 0xff}
	colorPanelBg    = color.RGBA{0x16, 0x1a, 0x22, 0xff}
	colorToolbarBg  = color.RGBA{0x1e, 0x22, 0x2c, 0xff}
	colorBox        = color.RGBA{0x33, 0x3a, 0x48, 0xff}
	colorGridDot    = color.RGBA{0x32, 0x3a, 0x46, 0xff}
	colorAxis       = color.RGBA{0x44, 0x4e, 0x5e, 0xff}

	colorVertex    = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colorOrigin    = color.RGBA{0xff, 0xa0, 0x40, 0xff}
	colorWeapon    = color.RGBA{0xff, 0x50, 0x50, 0xff}
	colorThruster  = color.RGBA{0x50, 0xa0, 0xff, 0xff}
	colorCollision = color.RGBA{0x40, 0xff, 0x80, 0xff}
	colorSnap      = color.RGBA{0xff, 0xff, 0x40, 0xff}
	colorSelect    = color.RGBA{0x80, 0xff, 0xff, 0xff}
	colorDraft     = color.RGBA{0xff, 0xff, 0x80, 0xff}
	colorControl   = color.RGBA{0xff, 0x80, 0xff, 0xff} // curve control-point handles
	colorClose     = color.RGBA{0x60, 0xff, 0x90, 0xff} // draft can close into a loop
)

const (
	previewW = panelWidth - 32
	previewH = 150
)

// Draw renders the whole editor for one frame. The CRT post-process is applied
// only to the small preview (which represents the in-game look), not the whole
// editing UI.
func (e *Editor) Draw(screen *ebiten.Image) {
	screen.Fill(colorBackground)
	e.drawCanvas(screen)
	e.drawToolbar(screen)
	e.drawPanel(screen)
	if e.hpPanel {
		e.drawHardpointPanel(screen)
	}
	if e.colorPanel {
		e.drawColorPanel(screen)
	}
	if e.sndPanel {
		e.drawSoundPanel(screen)
	}
}

// drawCanvas draws the zoomed editing area: the asset, the size box and all
// editing overlays, clipped to the canvas region.
func (e *Editor) drawCanvas(screen *ebiten.Image) {
	r := e.canvasRect()
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), colorCanvasBg, false)

	view := e.canvasView()
	canvas := screen.SubImage(r).(*ebiten.Image)

	if e.asset.Editor.GridEnabled && e.asset.Editor.GridSize > 0 {
		editorkit.DrawDotGrid(canvas, view, r, e.asset.Editor.GridSize, colorGridDot, colorAxis, e.asset.Origin.X, e.asset.Origin.Y)
	}

	// Size box outline.
	bx, by := view.Project(0, 0)
	vector.StrokeRect(canvas, bx, by, float32(e.asset.Size.W*view.Scale), float32(e.asset.Size.H*view.Scale), 1, colorBox, false)

	render.DrawAsset(canvas, e.asset, view, true)

	if e.glowEnabled {
		e.glowMain.Apply(screen, e.asset.Layers, view, r, e.glowOptions())
	}

	e.drawOverlays(canvas, view)
}

// glowOptions builds the current glow parameters from the editor state.
func (e *Editor) glowOptions() render.GlowOptions {
	return render.GlowOptions{
		Variant:   e.glowVariant,
		Time:      float64(ebiten.Tick()) / float64(ebiten.TPS()),
		Intensity: 1,
		AntiAlias: true,
	}
}

// drawOverlays draws vertices, origin, hardpoints, collision, the in-progress
// path and the snap/selection highlights.
func (e *Editor) drawOverlays(dst *ebiten.Image, view render.View) {
	// Path vertices (skip hidden layers).
	for li := range e.asset.Layers {
		if e.asset.Layers[li].Hidden {
			continue
		}
		for _, p := range e.asset.Layers[li].Paths {
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

	// Origin crosshair.
	ox, oy := view.Project(e.asset.Origin.X, e.asset.Origin.Y)
	vector.StrokeLine(dst, ox-8, oy, ox+8, oy, 1, colorOrigin, true)
	vector.StrokeLine(dst, ox, oy-8, ox, oy+8, 1, colorOrigin, true)
	vector.StrokeCircle(dst, ox, oy, 4, 1, colorOrigin, true)

	// Hardpoints.
	for _, hp := range e.asset.Hardpoints {
		hx, hy := view.Project(hp.X, hp.Y)
		col := colorWeapon
		if hp.Kind == asset.KindThruster {
			col = colorThruster
		}
		vector.FillCircle(dst, hx, hy, 3, col, true)
		rad := hp.Angle * math.Pi / 180
		ex := hx + float32(math.Cos(rad))*10
		ey := hy + float32(math.Sin(rad))*10
		vector.StrokeLine(dst, hx, hy, ex, ey, 1, col, true)
	}

	// Collision shapes and their point markers.
	for si := range e.asset.Collisions {
		e.drawCollisionShape(dst, view, &e.asset.Collisions[si])
	}
	e.drawCollisionDraft(dst, view)

	e.drawDraft(dst, view)

	// Selection highlight.
	if e.hasActive {
		sx, sy := view.Project(e.active.x, e.active.y)
		vector.StrokeCircle(dst, sx, sy, 7, 1, colorSelect, true)
	}

	// Snap highlight.
	if e.snapActive {
		sx, sy := view.Project(e.snapPoint.X, e.snapPoint.Y)
		vector.StrokeCircle(dst, sx, sy, 7, 1.5, colorSnap, true)
	}

	// Cursor coordinate readout, in asset space, next to the pointer.
	if e.mouseInCanvas {
		sx, sy := view.Project(e.cursor.X, e.cursor.Y)
		ebitenutil.DebugPrintAt(dst, fmt.Sprintf("%g, %g", e.cursor.X, e.cursor.Y), int(sx)+8, int(sy)-18)
	}
}

// drawDraft renders the path being drawn (curves flattened) plus the pen handle
// preview, the rubber-band segment to the cursor and the close indicator.
func (e *Editor) drawDraft(dst *ebiten.Image, view render.View) {
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

// drawCollisionShape strokes a collision shape and marks its points.
func (e *Editor) drawCollisionShape(dst *ebiten.Image, view render.View, s *asset.CollisionShape) {
	strokeCollision(dst, view, s.Kind, s.Points, s.Radius)
	for _, p := range s.Points {
		sx, sy := view.Project(p.X, p.Y)
		vector.FillRect(dst, sx-2, sy-2, 4, 4, colorCollision, false)
	}
}

// drawCollisionDraft previews the collision shape currently being placed.
func (e *Editor) drawCollisionDraft(dst *ebiten.Image, view render.View) {
	if e.tool != toolCollision || len(e.collDraft) == 0 {
		return
	}
	switch e.collKind {
	case asset.CollisionCircle:
		c := e.collDraft[0]
		r := math.Hypot(e.cursor.X-c.X, e.cursor.Y-c.Y)
		strokeCollision(dst, view, asset.CollisionCircle, []asset.Point{c}, r)
	case asset.CollisionRect:
		strokeCollision(dst, view, asset.CollisionRect, []asset.Point{e.collDraft[0], e.cursor}, 0)
	case asset.CollisionTriangle:
		pts := append(append([]asset.Point{}, e.collDraft...), e.cursor)
		strokePolyline(dst, view, pts, false)
	}
	for _, p := range e.collDraft {
		sx, sy := view.Project(p.X, p.Y)
		vector.FillRect(dst, sx-2, sy-2, 4, 4, colorCollision, false)
	}
}

// strokeCollision strokes a shape of the given kind without point markers.
func strokeCollision(dst *ebiten.Image, view render.View, kind string, pts []asset.Point, radius float64) {
	switch kind {
	case asset.CollisionCircle:
		if len(pts) >= 1 {
			cx, cy := view.Project(pts[0].X, pts[0].Y)
			vector.StrokeCircle(dst, cx, cy, float32(radius*view.Scale), 1, colorCollision, true)
		}
	case asset.CollisionRect:
		if len(pts) >= 2 {
			x0, y0 := view.Project(pts[0].X, pts[0].Y)
			x1, y1 := view.Project(pts[1].X, pts[1].Y)
			vector.StrokeRect(dst, min(x0, x1), min(y0, y1), abs32(x1-x0), abs32(y1-y0), 1, colorCollision, true)
		}
	case asset.CollisionTriangle:
		strokePolyline(dst, view, pts, len(pts) >= 3)
	}
}

// strokePolyline strokes connected segments through the points, optionally
// closing the last point back to the first.
func strokePolyline(dst *ebiten.Image, view render.View, pts []asset.Point, closed bool) {
	if len(pts) < 2 {
		return
	}
	for i := 1; i < len(pts); i++ {
		x0, y0 := view.Project(pts[i-1].X, pts[i-1].Y)
		x1, y1 := view.Project(pts[i].X, pts[i].Y)
		vector.StrokeLine(dst, x0, y0, x1, y1, 1, colorCollision, true)
	}
	if closed {
		x0, y0 := view.Project(pts[len(pts)-1].X, pts[len(pts)-1].Y)
		x1, y1 := view.Project(pts[0].X, pts[0].Y)
		vector.StrokeLine(dst, x0, y0, x1, y1, 1, colorCollision, true)
	}
}

// abs32 returns the absolute value of a float32.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// drawToolbar draws the clickable top tool bar; the buttons themselves are built
// each frame in runToolbar, so here we only paint the strip and flush them.
func (e *Editor) drawToolbar(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, toolbarHeight, colorToolbarBg, false)
	e.bar.Render(screen)
}

// drawPanel draws the right-hand preview and properties.
func (e *Editor) drawPanel(screen *ebiten.Image) {
	px := screenWidth - panelWidth
	vector.FillRect(screen, float32(px), toolbarHeight, panelWidth, screenHeight-toolbarHeight, colorPanelBg, false)

	ebitenutil.DebugPrintAt(screen, "PREVIEW (1:1)", px+16, toolbarHeight+6)

	e.drawPreview(screen)
	vector.StrokeRect(screen, previewBoxX, previewBoxY, previewW, previewH, 1, colorBox, false)

	// Properties text.
	ty := previewBoxY + previewH + 12
	ebitenutil.DebugPrintAt(screen, e.propertiesText(), px+16, ty)

	// No shortcut legend: it overflowed the panel and was unreadable. The full
	// reference lives in the README; the toolbar buttons carry the essentials.
}

// drawPreview renders the asset at 1:1 into the preview buffer (with glow) and
// presents it in the preview box, through the subtle CRT when enabled. This is
// the editor's "in-game look" view, so the CRT lives here rather than over the
// whole editing UI.
func (e *Editor) drawPreview(screen *ebiten.Image) {
	if e.previewBuf == nil {
		e.previewBuf = ebiten.NewImage(previewW, previewH)
	}
	buf := e.previewBuf
	buf.Fill(colorCanvasBg)

	local := render.View{
		OffsetX: (previewW - e.asset.Size.W) / 2,
		OffsetY: (previewH - e.asset.Size.H) / 2,
		Scale:   1,
	}
	render.DrawAsset(buf, e.asset, local, true)
	if e.glowEnabled {
		e.glowPreview.Apply(buf, e.asset.Layers, local, buf.Bounds(), e.glowOptions())
	}

	var geo ebiten.GeoM
	geo.Translate(previewBoxX, previewBoxY)
	if e.crtEnabled {
		e.crt.Present(screen, buf, geo, render.DefaultCRTOptions())
	} else {
		screen.DrawImage(buf, &ebiten.DrawImageOptions{GeoM: geo})
	}
}

// propertiesText builds the multi-line properties summary.
func (e *Editor) propertiesText() string {
	layer := e.currentLayer()
	collision := "none"
	if len(e.asset.Collisions) > 0 {
		collision = fmt.Sprintf("%d shape(s)", len(e.asset.Collisions))
	}
	if e.tool == toolCollision {
		collision += " [+" + e.collKind + "]"
	}
	snap := "on"
	if !e.asset.Editor.SnapEnabled {
		snap = "off"
	}
	if e.snapDisabled {
		snap = "off (Alt)"
	}
	grid := fmt.Sprintf("%s (size %g)", onOff(e.asset.Editor.GridEnabled), e.asset.Editor.GridSize)
	if e.asset.Editor.SnapToGrid {
		grid += ", snap"
	}
	bloom := "off"
	if e.glowEnabled {
		bloom = "on (" + e.glowVariant.String() + ")"
	}
	if e.crtEnabled {
		bloom += " +crt"
	}
	saved := "saved"
	if e.dirty {
		saved = "UNSAVED *"
	}
	return fmt.Sprintf(
		"name:   %s\nsize:   %.0fx%.0f\norigin: %.0f,%.0f\ntool:   %s\nsel:    %s\nzoom:   %.2fx\nscope:  %s\n\n%s\nstyle:  %s  w=%.1f\nfill:   %s  glow=%.2f\n\nhardpts: %d\ncollis: %s\nsnap:   %s\ngrid:   %s\nbloom:  %s\n\nfile:   %s (%s)\nstatus: %s",
		e.asset.Name,
		e.asset.Size.W, e.asset.Size.H,
		e.asset.Origin.X, e.asset.Origin.Y,
		toolName(e.tool),
		e.selectionLabel(),
		e.cam.View.Scale,
		e.scope,
		e.layersText(),
		layer.Stroke, layer.StrokeWidth,
		layer.Fill, layer.Glow,
		len(e.asset.Hardpoints),
		collision,
		snap,
		grid,
		bloom,
		e.savePath, saved,
		e.status,
	)
}

// toolName returns the display name of a tool.
func toolName(t toolID) string {
	switch t {
	case toolSelect:
		return "Select"
	case toolLine:
		return "Line"
	case toolOrigin:
		return "Origin"
	case toolWeapon:
		return "Weapon"
	case toolThruster:
		return "Thruster"
	case toolCollision:
		return "Collision"
	case toolDelete:
		return "Delete"
	default:
		return "?"
	}
}
