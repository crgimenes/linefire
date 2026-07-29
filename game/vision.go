package game

import (
	"cmp"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// visHit is one visibility ray: its angle and how far it reached before rock.
type visHit struct{ ang, dist float64 }

const (
	visRays  = 64   // fill-in rays around the view circle for open directions
	fogCell  = 24.0 // discovery grid cell size (the logic grid for fogHidden)
	seenStep = 12.0 // sampling step when clipping walls to the cleared region (maps)
)

// fogTexScale (texels per world unit) lives in the platform profile: the fog
// texture covers the WHOLE map (34 MB on map0001 at 1.5) and one is retained per
// visited map, so its resolution is a memory knob, not just a quality one.

// fogColor is the fog-of-war layer over never-explored areas. It uses the map's
// exterior tint (floodFillColor) so unexplored space reads as "solid" negative
// space rather than a foreign gray; the alpha lets a little of the world show.
var fogColor = color.RGBA{floodFillColor.R, floodFillColor.G, floodFillColor.B, 0xd8}

// discovery is the coarse logic grid of which cells the brush has cleared (fog of
// war). It drives fogHidden (entity culling) and the minimap; the smooth fog
// rendering uses the high-res fogTex instead. It persists, so re-entering a
// cleared area reuses what was already opened.
type discovery struct {
	cols, rows       int
	cell             float64
	originX, originY float64 // world coordinate of cell (0,0)
	seen             []bool
	rev              int // bumped on every newly seen cell, so caches derived from seen can invalidate
}

// newDiscovery sizes a discovery grid to the map bounds (with an origin), or
// returns nil for an empty map (keeps construction safe without a window).
func newDiscovery(b bounds) *discovery {
	w, h := b.w(), b.h()
	if w <= 0 || h <= 0 {
		return nil
	}
	cols := int(math.Ceil(w/fogCell)) + 1
	rows := int(math.Ceil(h/fogCell)) + 1
	return &discovery{cols: cols, rows: rows, cell: fogCell, originX: b.minX, originY: b.minY, seen: make([]bool, cols*rows), rev: 1}
}

// markSeen records a cell as cleared.
func (d *discovery) markSeen(cx, cy int) {
	i := cy*d.cols + cx
	if d.seen[i] {
		return
	}
	d.seen[i] = true
	d.rev++
}

// cellAt returns the grid cell containing world point (wx,wy).
func (d *discovery) cellAt(wx, wy float64) (int, int) {
	return int((wx - d.originX) / d.cell), int((wy - d.originY) / d.cell)
}

// discoveredAt reports whether the cell containing world point (wx,wy) has ever
// been cleared.
func (d *discovery) discoveredAt(wx, wy float64) bool {
	cx, cy := d.cellAt(wx, wy)
	if cx < 0 || cy < 0 || cx >= d.cols || cy >= d.rows {
		return false
	}
	return d.seen[cy*d.cols+cx]
}

// updateDiscovery clears the fog the ship reveals: every cell it has a clear line of sight
// to, at any distance (the vision reaches until a wall blocks it). Line-of-sight ONLY — it
// never reveals through a wall. The cleared region persists (fog-of-war memory).
// discoveryIdleEvery is how often a STATIONARY ship still re-scans, so a fresh dig's new sight
// lines are discovered within a fraction of a second even without moving.
const discoveryIdleEvery = 12

func (g *Game) updateDiscovery() {
	d := g.disc
	if d == nil {
		return
	}
	// The old scan raycast to EVERY unseen cell on the whole map, every frame — O(map area). Two
	// cheap bounds fix it without changing what the player sees on screen: (1) rotation never
	// changes line-of-sight from a point, so a still ship only re-scans on a slow tick; (2) reveal
	// only cells within the on-screen reach, so the cost is bounded by SCREEN size, not map size.
	g.discTick++
	moved := math.Hypot(g.x-g.discX, g.y-g.discY) >= d.cell*0.5
	if !moved && g.discTick%discoveryIdleEvery != 0 {
		return
	}
	g.discX, g.discY = g.x, g.y

	reach := g.discoveryReach()
	r2 := reach * reach
	cx0 := max(int((g.x-reach-d.originX)/d.cell), 0)
	cy0 := max(int((g.y-reach-d.originY)/d.cell), 0)
	cx1 := min(int((g.x+reach-d.originX)/d.cell), d.cols-1)
	cy1 := min(int((g.y+reach-d.originY)/d.cell), d.rows-1)
	for cy := cy0; cy <= cy1; cy++ {
		for cx := cx0; cx <= cx1; cx++ {
			if d.seen[cy*d.cols+cx] {
				continue
			}
			wx := d.originX + (float64(cx)+0.5)*d.cell
			wy := d.originY + (float64(cy)+0.5)*d.cell
			if (wx-g.x)*(wx-g.x)+(wy-g.y)*(wy-g.y) > r2 {
				continue // outside the on-screen reach
			}
			if g.lineOfSight(g.x, g.y, wx, wy) { // discovery is line-of-sight only: no seeing through walls
				d.markSeen(cx, cy)
			}
		}
	}
}

// discoveryReach is how far the fog discovery reaches from the ship: the world distance to the
// screen corner plus a margin, so everything on screen (and a little past it) is revealed while
// the scan stays bounded by the viewport, not the map. Falls back to the map diagonal pre-Layout.
func (g *Game) discoveryReach() float64 {
	scale := g.camPixelScale()
	if scale <= 0 {
		return g.bounds.diagonal()
	}
	sw, sh := g.screenSize()
	return 0.5*math.Hypot(sw, sh)/scale + 2*fogCell
}

// clearedNear reports whether the cell at (wx,wy) or any of its 8 neighbours has
// been cleared. Used to test whether a wall point has been revealed: the wall's
// own cell can be line-of-sight-blocked, but an adjacent corridor cell is cleared.
func (g *Game) clearedNear(wx, wy float64) bool {
	d := g.disc
	if d == nil {
		return false
	}
	cx, cy := d.cellAt(wx, wy)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx, ny := cx+dx, cy+dy
			if nx >= 0 && ny >= 0 && nx < d.cols && ny < d.rows && d.seen[ny*d.cols+nx] {
				return true
			}
		}
	}
	return false
}

// seenSpans returns the world-space sub-segments of s that have been revealed
// (lie in or next to cleared cells), so the maps only show the walls the ship has
// actually touched with its vision or near circle, not whole segments at once.
func (g *Game) seenSpans(s segment) [][4]float64 {
	dx, dy := s.bx-s.ax, s.by-s.ay
	length := math.Hypot(dx, dy)
	if length == 0 {
		if g.clearedNear(s.ax, s.ay) {
			return [][4]float64{{s.ax, s.ay, s.bx, s.by}}
		}
		return nil
	}
	n := int(length/seenStep) + 1
	var spans [][4]float64
	inRun := false
	var rx, ry, px, py float64
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, y := s.ax+dx*t, s.ay+dy*t
		switch {
		case g.clearedNear(x, y) && !inRun:
			inRun, rx, ry = true, x, y
		case !g.clearedNear(x, y) && inRun:
			inRun = false
			spans = append(spans, [4]float64{rx, ry, px, py})
		}
		px, py = x, y
	}
	if inRun {
		spans = append(spans, [4]float64{rx, ry, px, py})
	}
	return spans
}

// rayHitsSegment returns the distance from (ox,oy) along the unit direction
// (dx,dy) to the first intersection with segment s, and whether it hits.
func rayHitsSegment(ox, oy, dx, dy float64, s segment) (float64, bool) {
	sx, sy := s.bx-s.ax, s.by-s.ay
	denom := dx*sy - dy*sx
	if math.Abs(denom) < 1e-12 {
		return 0, false // parallel
	}
	ex, ey := s.ax-ox, s.ay-oy
	t := (ex*sy - ey*sx) / denom // distance along the ray
	u := (ex*dy - ey*dx) / denom // position along the segment
	if t >= 0 && u >= 0 && u <= 1 {
		return t, true
	}
	return 0, false
}

// visibilityPolygon returns the polygon of points visible from (ox,oy), bounded
// by walls and radius, as world-space points sorted by angle. It casts rays
// toward each wall corner (plus a tiny offset to peek just past it) and around
// the view circle, keeping the nearest wall hit (or the radius).
// Only walls within the radius can shape the polygon (a farther segment cannot
// beat the radius cap), so the walls are prefiltered once into a reused scratch
// slice: the cost is quadratic in NEARBY walls, not in the whole map's.
// On a flood map the rays march the REGION GRID — the truth that includes dug
// rock — instead of the authored segments, which outlive the walls they drew: a
// hole blasted through a wall opens a sight cone through it (and a revisited
// map's restored tunnels clear on the first stamp). The corner rays still come
// from the authored geometry, keeping hard wall edges crisp; the fan covers the
// openings the segments know nothing about.
func (g *Game) visibilityPolygon(ox, oy, radius float64) []vec2 {
	r2 := radius * radius
	segs := g.visSegs[:0]
	for _, s := range g.segs {
		if distPointSegmentSq(ox, oy, s.ax, s.ay, s.bx, s.by) <= r2 {
			segs = append(segs, s)
		}
	}
	g.visSegs = segs

	castDist := func(dx, dy float64) float64 {
		best := radius
		for _, s := range segs {
			t, ok := rayHitsSegment(ox, oy, dx, dy, s)
			if ok && t < best {
				best = t
			}
		}
		return best
	}
	if g.flood != nil {
		f := g.flood
		castDist = func(dx, dy float64) float64 { return f.rayToRock(ox, oy, dx, dy, radius) }
	}

	hits := g.visHits[:0]

	cast := func(ang float64) {
		dx, dy := math.Cos(ang), math.Sin(ang)
		hits = append(hits, visHit{ang, castDist(dx, dy)})
	}

	const eps = 0.0006
	for _, s := range segs {
		for _, p := range [2][2]float64{{s.ax, s.ay}, {s.bx, s.by}} {
			a := math.Atan2(p[1]-oy, p[0]-ox)
			cast(a - eps)
			cast(a)
			cast(a + eps)
		}
	}
	for i := range visRays {
		cast(2 * math.Pi * float64(i) / visRays)
	}

	g.visHits = hits // keep the grown capacity for the next stamp
	slices.SortFunc(hits, func(a, b visHit) int { return cmp.Compare(a.ang, b.ang) })
	poly := g.visPoly[:0]
	for _, h := range hits {
		poly = append(poly, vec2{ox + math.Cos(h.ang)*h.dist, oy + math.Sin(h.ang)*h.dist})
	}
	g.visPoly = poly
	return poly
}

// ensureFogMask (re)allocates the device-res mask buffer used to build the fog.
func (g *Game) ensureFogMask() *ebiten.Image {
	if g.fogMask == nil || g.fogMask.Bounds().Dx() != g.sw || g.fogMask.Bounds().Dy() != g.sh {
		g.fogMask = ebiten.NewImage(g.sw, g.sh)
	}
	return g.fogMask
}

// ensureFogTex lazily builds the world-space "cleared" texture: one channel of
// alpha at fogTexScale texels per world unit, big enough that scaling it to the
// screen stays crisp (no pixelation) even on large or high-DPI windows.
// A fresh texture is immediately re-hydrated from the discovery grid, so a
// revisited map (whose texture was deliberately NOT retained — a full-map GPU
// image per visited map ratcheted memory) comes back already cleared where the
// player has been.
func (g *Game) ensureFogTex() {
	if g.fogTex != nil || g.level == nil {
		return
	}
	w := int(math.Ceil(g.bounds.w() * fogTexScale))
	h := int(math.Ceil(g.bounds.h() * fogTexScale))
	if w <= 0 || h <= 0 {
		return
	}
	g.fogTex = ebiten.NewImage(w, h)
	g.rehydrateFogTex()
}

// fogRehydrateRes is the re-hydration mask's resolution, in texels per discovery
// cell axis. The mask is the binary seen grid sampled BILINEARLY and thresholded
// at the 0.5 iso-contour, so the boundary between seen and fogged runs straight
// between cell centers instead of stair-stepping along cell blocks; the final
// linear upscale to the fog texture then only has ~cell/res world units of
// feather to add. 8 leaves ~4 device px of edge softness at the native scale —
// close to the stamped polygons — for a mask a few hundred KB big, built once
// per map entry.
const fogRehydrateRes = 8

// rehydrateFogTex re-paints the cleared texture from the discovery grid — the
// few-KB truth that survives map changes and checkpoints. The edge comes back
// slightly softer than the stamped polygons and refines again as the ship flies.
func (g *Game) rehydrateFogTex() {
	d := g.disc
	if d == nil {
		return
	}
	seenAny := false
	for _, s := range d.seen {
		if s {
			seenAny = true
			break
		}
	}
	if !seenAny {
		return // a fresh map: nothing to restore
	}

	seenAt := func(cx, cy int) float64 { // clamped: border cells extend outward
		cx = min(max(cx, 0), d.cols-1)
		cy = min(max(cy, 0), d.rows-1)
		if d.seen[cy*d.cols+cx] {
			return 1
		}
		return 0
	}

	const res = fogRehydrateRes
	w, h := d.cols*res, d.rows*res
	pix := make([]byte, w*h*4)
	for y := range h {
		cy := (float64(y)+0.5)/res - 0.5
		y0 := int(math.Floor(cy))
		fy := cy - float64(y0)
		for x := range w {
			cx := (float64(x)+0.5)/res - 0.5
			x0 := int(math.Floor(cx))
			fx := cx - float64(x0)
			s00, s10 := seenAt(x0, y0), seenAt(x0+1, y0)
			s01, s11 := seenAt(x0, y0+1), seenAt(x0+1, y0+1)
			if s00 == s10 && s00 == s01 && s00 == s11 { // uniform neighborhood: no contour here
				if s00 == 0 {
					continue
				}
			} else if lerp(lerp(s00, s10, fx), lerp(s01, s11, fx), fy) < 0.5 {
				continue
			}
			p := (y*w + x) * 4
			pix[p], pix[p+1], pix[p+2], pix[p+3] = 0xff, 0xff, 0xff, 0xff
		}
	}

	mask := ebiten.NewImage(w, h)
	mask.WritePixels(pix)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(d.cell*fogTexScale/res, d.cell*fogTexScale/res) // mask texel -> fog texels (both grids share the map origin)
	op.Filter = ebiten.FilterLinear
	g.fogTex.DrawImage(mask, &op)
	mask.Deallocate()
}

// drawBrushFog overlays the fog of war: the map's exterior tint over everything,
// punched fully transparent where the ship's vision has cleared. The cleared area
// is a persistent high-res texture stamped each frame with the smooth visibility
// polygon (the vision cone, blocked by walls), in world space. Drawn before the
// ship/HUD, so the ship — always inside its own vision — stays visible.
func (g *Game) drawBrushFog(screen *ebiten.Image) {
	if g.transparent {
		return // fog is an opaque veil; over the desktop it would just black the screen out
	}
	if g.disc == nil {
		return // simple (procedural) map: no fog of war
	}
	g.ensureFogTex()
	if g.fogTex == nil {
		return
	}
	// The fog texture is in world space offset by the map origin (bounds), so it
	// covers maps anywhere — including negative coordinates.
	ox, oy := g.bounds.minX, g.bounds.minY

	// The cleared texture is PERSISTENT and the stamp depends only on the ship's
	// position and the rock (rotation never changes line of sight), so a ship that
	// has not moved since the last stamp has nothing new to clear — skip the whole
	// polygon + stamp, the dominant per-frame CPU cost of the fog. Sub-texel drift
	// accumulates against the last STAMPED position, so slow motion still re-stamps
	// once it adds up; a DIG resets fogStamped (refreshFlood), since moving the rock
	// face moves the sight lines even under a ship that holds still.
	moved := math.Hypot(g.x-g.fogStampX, g.y-g.fogStampY) >= 0.25
	if !g.fogStamped || moved {
		g.fogStamped = true
		g.fogStampX, g.fogStampY = g.x, g.y

		// A tiny halo the size of the ship keeps fog off its own edge (covering the visibility
		// polygon's apex). It is only as wide as the hull, and collision keeps the hull off walls,
		// so it can never reveal across one — the fog now strictly respects line of sight.
		vector.FillCircle(g.fogTex, float32((g.x-ox)*fogTexScale), float32((g.y-oy)*fogTexScale),
			float32(g.radius*fogTexScale), color.White, true)

		// Beyond: what the ship sees (the line-of-sight polygon, reaching until walls
		// stop it), stamped into the cleared texture in fog-texture space with a smooth
		// anti-aliased edge. The rays are bounded to the ON-SCREEN reach (as discovery
		// is), not the map diagonal: the overlay can only show what is on screen, and
		// the persistent texture keeps everything ever cleared, so the bound changes
		// nothing visible while it caps the ray casting at the viewport's size.
		poly := g.visibilityPolygon(g.x, g.y, g.discoveryReach())
		if len(poly) >= 3 {
			g.fogPath.Reset()
			for i := range poly {
				tx, ty := float32((poly[i].x-ox)*fogTexScale), float32((poly[i].y-oy)*fogTexScale)
				if i == 0 {
					g.fogPath.MoveTo(tx, ty)
				} else {
					g.fogPath.LineTo(tx, ty)
				}
			}
			g.fogPath.Close()
			vector.FillPath(g.fogTex, &g.fogPath, &vector.FillOptions{}, &vector.DrawPathOptions{AntiAlias: true})
		}
	}

	// Fog only over the navigable interior (the black playable area). The exterior
	// solid/off-map is never fogged — there's nothing to discover there — which
	// keeps the fog off the map boundary entirely (the big win on large maps).
	cam := g.cameraGeoM()
	mask := g.ensureFogMask()
	mask.Clear()
	g.fillNavigable(mask, cam, fogColor)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(1/fogTexScale, 1/fogTexScale) // fog texel -> world units
	op.GeoM.Translate(ox, oy)                   // shift by the map origin
	op.GeoM.Concat(cam)                         // world -> screen
	op.Blend = ebiten.BlendDestinationOut
	op.Filter = ebiten.FilterLinear
	mask.DrawImage(g.fogTex, &op) // cleared -> transparent, smooth edge
	screen.DrawImage(mask, nil)
}
