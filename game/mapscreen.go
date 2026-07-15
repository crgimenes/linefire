package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	mapBaseScale = 0.45 // world units -> overlay pixels at zoom 1
	mapZoomMin   = 0.25
	mapZoomMax   = 3.0
	mapZoomStep  = 1.12 // multiplier per key press
)

var (
	mapBg     = color.RGBA{0x04, 0x07, 0x0c, 0xf2}
	mapWall   = color.RGBA{0x70, 0xe0, 0xf0, 0xff}
	mapPlayer = color.RGBA{0xa0, 0xff, 0xff, 0xff}
	mapTunnel = color.RGBA{0x18, 0x28, 0x30, 0xff} // carved rock: filled under the wall lines
)

// drawMapScreen draws the full-screen automap (toggled with M): a north-up
// wireframe of the walls the player has charted, centered on the ship, with the
// ship as a heading arrow and any in-sight enemies and charted power-ups. It
// draws into the logical HUD overlay so it scales with the display.
func (g *Game) drawMapScreen(dst *ebiten.Image) {
	w, h := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	vector.FillRect(dst, 0, 0, float32(w), float32(h), mapBg, false)

	cx, cy := w/2, h/2
	scale := mapBaseScale * g.mapZoom
	toScreen := func(wx, wy float64) (float32, float32) {
		return float32((wx-g.x)*scale + cx), float32((wy-g.y)*scale + cy)
	}

	// Tunnels the player carved through the rock, painted UNDER the wall lines so the
	// charted layout still reads on top. Digging a cell means the player was there, so
	// no fog gating is needed.
	g.drawMapTunnels(dst, toScreen)

	// Charted walls: only the spans revealed by vision/brush (not whole segments).
	for i := range g.segs {
		for _, sp := range g.seenSpans(g.segs[i]) {
			ax, ay := toScreen(sp[0], sp[1])
			bx, by := toScreen(sp[2], sp[3])
			vector.StrokeLine(dst, ax, ay, bx, by, 1.5, mapWall, true)
		}
	}

	// Enemies and items in cleared (fog-free) areas, colored by alignment
	// (green=good, red=bad, blue=ally, yellow=neutral).
	for i := range g.entities {
		e := &g.entities[i]
		if g.fogHidden(e.x, e.y) {
			continue
		}
		sx, sy := toScreen(e.x, e.y)
		vector.FillCircle(dst, sx, sy, 3, markerColor(e.align), true)
	}

	// Objective (exit) zones, once the fog has revealed them.
	for i := range g.objectives {
		o := g.objectives[i]
		if o.kind != objReach {
			continue
		}
		z, ok := g.zoneByName(o.zone)
		if !ok {
			continue
		}
		zx, zy := zoneCenter(z)
		if g.fogHidden(zx, zy) {
			continue
		}
		sx, sy := toScreen(zx, zy)
		col := goalColor
		if o.done {
			col = goalDoneColor
		}
		r := float32(z.Radius * scale)
		if r < 5 {
			r = 5
		}
		vector.StrokeCircle(dst, sx, sy, r, 1.5, col, true)
	}

	drawMapArrow(dst, cx, cy, g.angle)
	ebitenutil.DebugPrintAt(dst, "MAP   M close   +/- zoom", 12, 12)
}

// dugRun is a horizontal run of dug cells in one row: cells [x0, x1) of row y.
type dugRun struct{ y, x0, x1 int }

// dugRuns coalesces the dug cells into per-row horizontal runs, so a long tunnel is
// one rectangle to draw instead of one per cell. Pure, so the coalescing is testable.
func dugRuns(dug []bool, cols, rows int) []dugRun {
	var runs []dugRun
	for j := range rows {
		base := j * cols
		i := 0
		for i < cols {
			if !dug[base+i] {
				i++
				continue
			}
			start := i
			for i < cols && dug[base+i] {
				i++
			}
			runs = append(runs, dugRun{y: j, x0: start, x1: i})
		}
	}
	return runs
}

// drawMapTunnels fills the cells the player has dug out of the rock, so the automap
// shows the passages that carry no wall segments (they would otherwise be invisible).
func (g *Game) drawMapTunnels(dst *ebiten.Image, toScreen func(float64, float64) (float32, float32)) {
	f := g.flood
	if f == nil || f.dug == nil {
		return
	}
	w, h := float32(dst.Bounds().Dx()), float32(dst.Bounds().Dy())
	for _, r := range dugRuns(f.dug, f.cols, f.rows) {
		sx0, sy0 := toScreen(f.originX+float64(r.x0)*f.cell, f.originY+float64(r.y)*f.cell)
		sx1, sy1 := toScreen(f.originX+float64(r.x1)*f.cell, f.originY+float64(r.y+1)*f.cell)
		if sx1 < 0 || sx0 > w || sy1 < 0 || sy0 > h {
			continue
		}
		vector.FillRect(dst, sx0, sy0, sx1-sx0, sy1-sy0, mapTunnel, false)
	}
}

// drawMapArrow draws the player marker as a triangle pointing along the world
// heading (degrees, -90 = up).
func drawMapArrow(dst *ebiten.Image, cx, cy, angleDeg float64) {
	a := angleDeg * math.Pi / 180
	const r = 9.0
	var p vector.Path
	p.MoveTo(float32(cx+math.Cos(a)*r), float32(cy+math.Sin(a)*r))
	p.LineTo(float32(cx+math.Cos(a+2.5)*r), float32(cy+math.Sin(a+2.5)*r))
	p.LineTo(float32(cx+math.Cos(a-2.5)*r), float32(cy+math.Sin(a-2.5)*r))
	p.Close()

	var cs ebiten.ColorScale
	cs.ScaleWithColor(mapPlayer)
	vector.FillPath(dst, &p, &vector.FillOptions{}, &vector.DrawPathOptions{
		AntiAlias:  true,
		ColorScale: cs,
	})
}

// zoomMap multiplies the automap zoom by f, clamped to its range.
func (g *Game) zoomMap(f float64) {
	g.mapZoom *= f
	if g.mapZoom < mapZoomMin {
		g.mapZoom = mapZoomMin
	} else if g.mapZoom > mapZoomMax {
		g.mapZoom = mapZoomMax
	}
}
