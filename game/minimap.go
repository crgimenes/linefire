package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	miniMargin = 14.0  // gap from the screen corner (logical px)
	miniRadius = 84.0  // minimap scope radius (logical px)
	miniRange  = 520.0 // world units across the minimap radius (a local view)
)

var (
	miniBg     = color.RGBA{0x06, 0x0d, 0x12, 0xd0}
	miniBorder = color.RGBA{0x80, 0xff, 0xff, 0xa0}
	miniWall   = color.RGBA{0x80, 0xff, 0xff, 0xe0}
	miniPlayer = color.RGBA{0xa0, 0xff, 0xff, 0xff}
)

// drawMinimap renders a round, local minimap in the top-right corner: the
// discovered walls around the ship (clipped to the circle), enemies in cleared
// areas, and the ship at the center. North-up, so it reads as a stable map.
func (g *Game) drawMinimap(dst *ebiten.Image) {
	if g.disc == nil {
		return
	}
	cx := float64(dst.Bounds().Dx()) - miniMargin - miniRadius
	cy := miniMargin + miniRadius
	scale := miniRadius / miniRange

	vector.FillCircle(dst, float32(cx), float32(cy), miniRadius, miniBg, true)

	toMap := func(wx, wy float64) (float64, float64) {
		return (wx-g.x)*scale + cx, (wy-g.y)*scale + cy
	}

	// Walls revealed by vision/brush, each span clipped to the scope circle. The
	// spans are CACHED in world coordinates and rebuilt only when discovery gains
	// a cell — re-sampling every wall against the fog grid each frame (seenSpans)
	// was a per-frame cost proportional to the whole map.
	if d := g.disc; d.rev != g.miniSpansRev {
		g.miniSpansRev = d.rev
		g.miniSpans = g.miniSpans[:0]
		for i := range g.segs {
			g.miniSpans = append(g.miniSpans, g.seenSpans(g.segs[i])...)
		}
	}
	for _, sp := range g.miniSpans {
		ax, ay := toMap(sp[0], sp[1])
		bx, by := toMap(sp[2], sp[3])
		nx0, ny0, nx1, ny1, ok := clipSegToCircle(ax, ay, bx, by, cx, cy, miniRadius)
		if ok {
			strokeLine(dst, nx0, ny0, nx1, ny1, 1, miniWall)
		}
	}

	// Enemies and items in cleared (fog-free) areas, inside the scope, colored by
	// alignment (green=good, red=bad, blue=ally, yellow=neutral).
	for i := range g.entities {
		e := &g.entities[i]
		if g.fogHidden(e.x, e.y) {
			continue
		}
		mx, my := toMap(e.x, e.y)
		if math.Hypot(mx-cx, my-cy) <= miniRadius {
			fillCircle(dst, mx, my, 2, markerColor(e.align))
		}
	}

	drawMiniShip(dst, cx, cy, g.angle)
	vector.StrokeCircle(dst, float32(cx), float32(cy), miniRadius, 1.5, miniBorder, true)
}

// clipSegToCircle clips segment (x0,y0)-(x1,y1) to the disc of radius r centered
// at (cx,cy), returning the inside portion and whether any of it lies inside.
func clipSegToCircle(x0, y0, x1, y1, cx, cy, r float64) (float64, float64, float64, float64, bool) {
	dx, dy := x1-x0, y1-y0
	fx, fy := x0-cx, y0-cy
	a := dx*dx + dy*dy
	if a == 0 {
		if fx*fx+fy*fy <= r*r {
			return x0, y0, x1, y1, true
		}
		return 0, 0, 0, 0, false
	}
	b := 2 * (fx*dx + fy*dy)
	c := fx*fx + fy*fy - r*r
	disc := b*b - 4*a*c
	if disc < 0 {
		return 0, 0, 0, 0, false
	}
	sq := math.Sqrt(disc)
	t0 := (-b - sq) / (2 * a)
	t1 := (-b + sq) / (2 * a)
	if t0 < 0 {
		t0 = 0
	}
	if t1 > 1 {
		t1 = 1
	}
	if t0 > t1 {
		return 0, 0, 0, 0, false
	}
	return x0 + t0*dx, y0 + t0*dy, x0 + t1*dx, y0 + t1*dy, true
}

// drawMiniShip draws the player marker as a small triangle pointing along the
// ship heading (degrees, -90 = up).
func drawMiniShip(dst *ebiten.Image, cx, cy, angleDeg float64) {
	a := angleDeg * math.Pi / 180
	const r = 5.0
	var p vector.Path
	p.MoveTo(float32(cx+math.Cos(a)*r), float32(cy+math.Sin(a)*r))
	p.LineTo(float32(cx+math.Cos(a+2.4)*r), float32(cy+math.Sin(a+2.4)*r))
	p.LineTo(float32(cx+math.Cos(a-2.4)*r), float32(cy+math.Sin(a-2.4)*r))
	p.Close()

	var cs ebiten.ColorScale
	cs.ScaleWithColor(miniPlayer)
	vector.FillPath(dst, &p, &vector.FillOptions{}, &vector.DrawPathOptions{
		AntiAlias:  true,
		ColorScale: cs,
	})
}
