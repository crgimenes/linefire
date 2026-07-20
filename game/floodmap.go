package game

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"linefire/asset"
)

const (
	// SPIKE (visual proof of destructible rock): the grid is the map's region model —
	// interior = the black navigable space, everything else is ROCK that glows. It is
	// fine enough (4 world units) that a shot's bite reads as a round hole.
	fillCell = 4.0 // flood-fill grid cell (world units); finer = crisper silhouette
	// A cell is rock when a wall line passes within this of its center. It must be at
	// least half the cell diagonal (cell*0.71 = 2.83) or a line could slip between two
	// cell centers and leave a GAP the ship would fly through. Kept at that minimum so
	// the rock hugs the line and corridors do not shrink under the ship's radius.
	fillMargin = 3.0
	gridPad    = 420.0 // world padding around the level: the rock behind the outer wall
	glowRange  = 32.0  // world distance the silhouette glow spreads before settling to the base

	// A shot eats this much rock; a blast eats more. The ship is radius 22, so one
	// shot never opens a passage — you keep firing and DIG.
	digBase     = 6.0
	digPerDmg   = 2.0
	digMax      = 22.0
	digJitter   = 0.30 // the bite's radius wobbles by angle, so holes are not stamped circles
	blastDigMul = 0.5  // an explosion's crater, as a fraction of its blast radius

	// Both distance fields have a USEFUL CEILING, and that is what makes an
	// incremental rebuild possible: `clear` is only ever read up to the A* heat band
	// (navClearance+navWarmBand = 154) and `dist` only up to glowRange. Clamping there
	// means a dig can only change values within `distCap` of itself, so recomputing a
	// WINDOW of that margin around the bite is exact — no full-grid transform per frame.
	clearCap = 160.0
	distCap  = glowRange + fillCell

	// Freshly cut rock glows hot and cools back to the wall colour. The band is the
	// depth of rock a bite leaves scorched; the decay is per frame.
	hotBand  = 10.0  // world units of rock face heated by a bite
	hotDecay = 0.965 // per-frame cooling: ~2.5s from white-hot to cold
	hotEps   = 0.02  // below this a cell is cold and leaves the hot list
)

var (
	// floodFillColor is the intermediate base the whole non-navigable area settles
	// to: the glow fades down to this (never to black) so the exterior keeps a
	// gentle teal all the way to the window edge.
	floodFillColor = color.RGBA{0x14, 0x38, 0x44, 0xff}
	// floodGlowColor is the glow added over the base: brightest at the silhouette
	// (wall edges), fading gradually outward over glowRange down to the base.
	floodGlowColor = color.RGBA{0x2a, 0x6e, 0x82, 0xff}
	// hotRockColor is what a face looks like the instant it is cut: molten orange.
	// It cools back to floodGlowColor. Because the glow is drawn additively over the
	// cold teal base, this reads as heat bleeding out of the wound.
	hotRockColor = color.RGBA{0xff, 0x5a, 0x1e, 0xff}
)

// floodmap implements the negative-space map model: a flood fill seeded at the
// player start marks the reachable interior (the playable area, kept black);
// everything else (rock, exterior, enclosed pockets) is solid and glows, brightest
// at the silhouette and fading over glowRange down to the base fill.
//
// SPIKE: the glow is now the distance to the nearest INTERIOR cell (a chamfer
// distance transform over the region), not the distance to a wall LINE. That is what
// lets a dug hole carry its own glow: carve interior into the rock and the silhouette
// simply moves. `dug` remembers what the player blasted, so it can be painted black
// over the hand-authored corridors.
type floodmap struct {
	cols, rows       int
	cell             float64
	originX, originY float64   // world coordinate of cell (0,0); negative due to padding
	interior         []bool    // reachable free cells (authored corridors + everything dug)
	dug              []bool    // cells the player blasted out of the rock
	dist             []float32 // rock cell -> world distance to the nearest interior cell (the GLOW)
	clear            []float32 // any cell -> world distance to the nearest rock cell (COLLISION clearance; 0 inside rock)
	pix              []byte    // RGBA backing for img: glow intensity per cell
	dugPix           []byte    // RGBA backing for dugImg: opaque black where dug
	img, dugImg      *ebiten.Image
	dirty            bool // a carve happened: the field and its images need a rebuild
	holes            int  // carves landed (debug telemetry)

	// hotAge is how freshly cut each rock cell is (1 = just melted, 0 = cold). It is a
	// pure RENDER channel — nothing in collision or nav reads it. hotCells is the short
	// list of cells still cooling, so a frame only touches those instead of the grid.
	hotAge   []float32
	hotCells []int32

	// Reused scratch for the windowed rebuild, so a dig allocates nothing.
	scratchD, scratchC     []float32
	scratchPix, scratchDug []byte

	// World-space bounding box of everything carved since the last rebuild, so the A*
	// grid only re-derives the cells a dig could have freed.
	dirtyMinX, dirtyMinY, dirtyMaxX, dirtyMaxY float64
}

// markDirty grows the pending dirty box around a carve.
func (f *floodmap) markDirty(x, y, r float64) {
	if !f.dirty {
		f.dirtyMinX, f.dirtyMinY = x-r, y-r
		f.dirtyMaxX, f.dirtyMaxY = x+r, y+r
		f.dirty = true
		return
	}
	f.dirtyMinX = math.Min(f.dirtyMinX, x-r)
	f.dirtyMinY = math.Min(f.dirtyMinY, y-r)
	f.dirtyMaxX = math.Max(f.dirtyMaxX, x+r)
	f.dirtyMaxY = math.Max(f.dirtyMaxY, y+r)
}

// buildFloodmap classifies cells (rock vs reachable interior) and bakes the boundary
// glow, once at load. The grid is padded beyond the level so the rock behind the
// outer wall is thick enough to dig a tunnel into. Returns nil for an empty level.
func buildFloodmap(segs []segment, startX, startY float64, b bounds) *floodmap {
	w, h := b.w(), b.h()
	if w <= 0 || h <= 0 {
		return nil
	}
	ox, oy := b.minX-gridPad, b.minY-gridPad
	cols := int(math.Ceil((w+2*gridPad)/fillCell)) + 1
	rows := int(math.Ceil((h+2*gridPad)/fillCell)) + 1
	n := cols * rows
	f := &floodmap{
		cols: cols, rows: rows, cell: fillCell, originX: ox, originY: oy,
		dug:    make([]bool, n),
		dist:   make([]float32, n),
		clear:  make([]float32, n),
		hotAge: make([]float32, n),
		pix:    make([]byte, n*4), dugPix: make([]byte, n*4),
	}

	solid := markSolidCells(segs, cols, rows, ox, oy)
	f.interior = floodFrom(solid, cols, rows, seedCell(solid, cols, rows, startX, startY, ox, oy))
	f.rebuild()
	return f
}

var neigh4 = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// markSolidCells flags every cell whose center is within fillMargin of a wall.
func markSolidCells(segs []segment, cols, rows int, ox, oy float64) []bool {
	solid := make([]bool, cols*rows)
	m2 := fillMargin * fillMargin
	for cy := range rows {
		for cx := range cols {
			px, py := ox+(float64(cx)+0.5)*fillCell, oy+(float64(cy)+0.5)*fillCell
			for _, s := range segs {
				if distPointSegmentSq(px, py, s.ax, s.ay, s.bx, s.by) <= m2 {
					solid[cy*cols+cx] = true
					break
				}
			}
		}
	}
	return solid
}

// seedCell returns the start cell for the flood fill, nudged off a wall if the
// raw start landed on one.
func seedCell(solid []bool, cols, rows int, startX, startY, ox, oy float64) int {
	scx, scy := int((startX-ox)/fillCell), int((startY-oy)/fillCell)
	if scx >= 0 && scy >= 0 && scx < cols && scy < rows && !solid[scy*cols+scx] {
		return scy*cols + scx
	}
	return nearestNonSolid(solid, cols, rows, scx, scy)
}

// floodFrom returns the cells reachable from seed across non-solid cells (4-way).
func floodFrom(solid []bool, cols, rows, seed int) []bool {
	interior := make([]bool, cols*rows)
	if seed < 0 {
		return interior
	}
	queue := []int{seed}
	interior[seed] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		ccx, ccy := cur%cols, cur/cols
		for _, d := range neigh4 {
			nx, ny := ccx+d[0], ccy+d[1]
			if nx < 0 || ny < 0 || nx >= cols || ny >= rows {
				continue
			}
			ni := ny*cols + nx
			if solid[ni] || interior[ni] {
				continue
			}
			interior[ni] = true
			queue = append(queue, ni)
		}
	}
	return interior
}

// rebuild recomputes both distance fields and both images over the WHOLE grid. Used
// once at load; a dig uses rebuildWindow instead.
func (f *floodmap) rebuild() {
	f.chamferInto(f.dist, true, distCap)    // seeds = interior -> how deep into the rock we are (glow)
	f.chamferInto(f.clear, false, clearCap) // seeds = rock -> how much room the ship has (collision)
	f.bakeAll()
	if f.img != nil {
		f.img.WritePixels(f.pix)
	}
	if f.dugImg != nil {
		f.dugImg.WritePixels(f.dugPix)
	}
	f.dirty = false
}

// marginCells is how far a dig can change either field, in cells. Both fields are
// clamped at their useful ceiling, so nothing beyond this margin can move.
func (f *floodmap) marginCells() int {
	return int(math.Ceil(math.Max(clearCap, distCap)/f.cell)) + 2
}

// rebuildWindow recomputes the fields and images only where a dig could have changed
// them. WRITE rect = the dirty box grown by the margin (everything that can move);
// COMPUTE rect = that grown by the margin again, so every seed a written cell might
// need is inside. Values outside the compute rect are never read, and values in the
// compute rect's outer ring are never written back — they may be wrong there.
func (f *floodmap) rebuildWindow(x0, y0, x1, y1 float64) {
	m := f.marginCells()
	dx0 := int(math.Floor((x0 - f.originX) / f.cell))
	dy0 := int(math.Floor((y0 - f.originY) / f.cell))
	dx1 := int(math.Ceil((x1 - f.originX) / f.cell))
	dy1 := int(math.Ceil((y1 - f.originY) / f.cell))

	wx0, wy0 := max(dx0-m, 0), max(dy0-m, 0)
	wx1, wy1 := min(dx1+m, f.cols-1), min(dy1+m, f.rows-1)
	cx0, cy0 := max(wx0-m, 0), max(wy0-m, 0)
	cx1, cy1 := min(wx1+m, f.cols-1), min(wy1+m, f.rows-1)

	cw, ch := cx1-cx0+1, cy1-cy0+1
	if cw <= 0 || ch <= 0 {
		f.dirty = false
		return
	}
	f.scratchD = growF32(f.scratchD, cw*ch)
	f.scratchC = growF32(f.scratchC, cw*ch)
	f.chamferRect(f.scratchD, true, cx0, cy0, cw, ch, distCap)
	f.chamferRect(f.scratchC, false, cx0, cy0, cw, ch, clearCap)

	// Copy back only the write rect, then re-bake and re-upload just that.
	for cy := wy0; cy <= wy1; cy++ {
		for cx := wx0; cx <= wx1; cx++ {
			li := (cy-cy0)*cw + (cx - cx0)
			gi := cy*f.cols + cx
			f.dist[gi] = f.scratchD[li]
			f.clear[gi] = f.scratchC[li]
			f.bakeCell(gi)
		}
	}
	f.uploadRect(wx0, wy0, wx1, wy1)
	f.dirty = false
}

// growF32 returns a slice of at least n elements, reusing the backing array.
func growF32(s []float32, n int) []float32 {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]float32, n)
}

// chamferRect seeds a cell rect into a local buffer and runs the transform there.
func (f *floodmap) chamferRect(d []float32, seedInterior bool, cx0, cy0, w, h int, capV float32) {
	const big = float32(1e9)
	for ly := range h {
		for lx := range w {
			gi := (cy0+ly)*f.cols + (cx0 + lx)
			d[ly*w+lx] = big
			if f.interior[gi] == seedInterior {
				d[ly*w+lx] = 0
			}
		}
	}
	chamferSweeps(d, w, h)
	scaleClamp(d, float32(f.cell/3), capV)
}

// chamferSweeps runs the forward and backward 3-4 chamfer passes in place: every cell
// ends up holding its distance (in chamfer units) to the nearest zero-valued seed.
func chamferSweeps(d []float32, w, h int) {
	for y := range h {
		for x := range w {
			i := y*w + x
			if d[i] == 0 {
				continue
			}
			if x > 0 && d[i-1]+3 < d[i] {
				d[i] = d[i-1] + 3
			}
			if y > 0 && d[i-w]+3 < d[i] {
				d[i] = d[i-w] + 3
			}
			if x > 0 && y > 0 && d[i-w-1]+4 < d[i] {
				d[i] = d[i-w-1] + 4
			}
			if x < w-1 && y > 0 && d[i-w+1]+4 < d[i] {
				d[i] = d[i-w+1] + 4
			}
		}
	}
	for y := h - 1; y >= 0; y-- {
		for x := w - 1; x >= 0; x-- {
			i := y*w + x
			if x < w-1 && d[i+1]+3 < d[i] {
				d[i] = d[i+1] + 3
			}
			if y < h-1 && d[i+w]+3 < d[i] {
				d[i] = d[i+w] + 3
			}
			if x < w-1 && y < h-1 && d[i+w+1]+4 < d[i] {
				d[i] = d[i+w+1] + 4
			}
			if x > 0 && y < h-1 && d[i+w-1]+4 < d[i] {
				d[i] = d[i+w-1] + 4
			}
		}
	}
}

// scaleClamp converts chamfer units to world units and caps at the useful ceiling.
func scaleClamp(d []float32, k, capV float32) {
	for i := range d {
		v := d[i] * k
		if v > capV {
			v = capV
		}
		d[i] = v
	}
}

// uploadRect re-uploads only the changed texels of both images.
func (f *floodmap) uploadRect(x0, y0, x1, y1 int) {
	if f.img == nil || f.dugImg == nil {
		return // not synced to the GPU yet; the next sync uploads everything
	}
	w, h := x1-x0+1, y1-y0+1
	f.scratchPix = growBytes(f.scratchPix, w*h*4)
	f.scratchDug = growBytes(f.scratchDug, w*h*4)
	for ly := range h {
		src := ((y0+ly)*f.cols + x0) * 4
		dst := ly * w * 4
		copy(f.scratchPix[dst:dst+w*4], f.pix[src:src+w*4])
		copy(f.scratchDug[dst:dst+w*4], f.dugPix[src:src+w*4])
	}
	r := image.Rect(x0, y0, x1+1, y1+1)
	f.img.SubImage(r).(*ebiten.Image).WritePixels(f.scratchPix)
	f.dugImg.SubImage(r).(*ebiten.Image).WritePixels(f.scratchDug)
}

func growBytes(s []byte, n int) []byte {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]byte, n)
}

// chamferInto seeds the WHOLE grid and runs the transform there (load time).
// seedInterior picks which side seeds it:
//
//	true  -> seeds are the interior: d = how deep into the rock a cell sits (the glow)
//	false -> seeds are the rock:     d = how much clearance a point has (collision)
func (f *floodmap) chamferInto(d []float32, seedInterior bool, capV float32) {
	const big = float32(1e9)
	for i := range d {
		d[i] = big
		if f.interior[i] == seedInterior {
			d[i] = 0
		}
	}
	chamferSweeps(d, f.cols, f.rows)
	scaleClamp(d, float32(f.cell/3), capV)
}

// bakeCell writes one cell's texels: the silhouette glow (brightest at the rock face,
// fading to nothing over glowRange) and the black mask wherever the player has blasted
// the rock away.
func (f *floodmap) bakeCell(i int) {
	o := i * 4
	if f.interior[i] {
		f.pix[o], f.pix[o+1], f.pix[o+2], f.pix[o+3] = 0, 0, 0, 0
	} else {
		v := (glowRange - float64(f.dist[i])) / glowRange
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		// The COLOUR is baked per cell (premultiplied by the glow intensity) rather
		// than applied as one ColorScale at draw time — that is the whole reason a
		// freshly cut face can run hot while the rock beside it stays cold.
		h := float64(f.hotAge[i])
		f.pix[o] = uint8(v * lerp(float64(floodGlowColor.R), float64(hotRockColor.R), h))
		f.pix[o+1] = uint8(v * lerp(float64(floodGlowColor.G), float64(hotRockColor.G), h))
		f.pix[o+2] = uint8(v * lerp(float64(floodGlowColor.B), float64(hotRockColor.B), h))
		f.pix[o+3] = uint8(v * 255)
	}
	if f.dug[i] {
		f.dugPix[o], f.dugPix[o+1], f.dugPix[o+2] = colorBg.R, colorBg.G, colorBg.B
		f.dugPix[o+3] = 0xff
		return
	}
	f.dugPix[o], f.dugPix[o+1], f.dugPix[o+2], f.dugPix[o+3] = 0, 0, 0, 0
}

// bakeAll re-bakes every texel (load time).
func (f *floodmap) bakeAll() {
	for i := range f.interior {
		f.bakeCell(i)
	}
}

// cellOf returns the cell index for a world point, or -1 outside the grid.
func (f *floodmap) cellOf(x, y float64) int {
	cx := int(math.Floor((x - f.originX) / f.cell))
	cy := int(math.Floor((y - f.originY) / f.cell))
	if cx < 0 || cy < 0 || cx >= f.cols || cy >= f.rows {
		return -1
	}
	return cy*f.cols + cx
}

// rockAt reports whether (x,y) sits in rock. Beyond the grid the world is solid, so
// nothing can escape past the padding.
func (f *floodmap) rockAt(x, y float64) bool {
	if f == nil {
		return false
	}
	i := f.cellOf(x, y)
	return i < 0 || !f.interior[i]
}

// dugAt reports whether (x,y) sits in rock the player blasted open — a tunnel or a
// crater, as opposed to an authored corridor.
func (f *floodmap) dugAt(x, y float64) bool {
	if f == nil {
		return false
	}
	i := f.cellOf(x, y)
	return i >= 0 && f.dug[i]
}

// clearanceAt is how far (world units) the point is from the nearest rock, sampled
// bilinearly so the ship slides along a rock face instead of stepping over cells.
// Zero inside rock and beyond the grid.
func (f *floodmap) clearanceAt(x, y float64) float64 {
	if f == nil {
		return math.Inf(1)
	}
	fx := (x-f.originX)/f.cell - 0.5
	fy := (y-f.originY)/f.cell - 0.5
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	at := func(cx, cy int) float64 {
		if cx < 0 || cy < 0 || cx >= f.cols || cy >= f.rows {
			return 0 // outside the grid is solid rock
		}
		return float64(f.clear[cy*f.cols+cx])
	}
	top := at(x0, y0)*(1-tx) + at(x0+1, y0)*tx
	bot := at(x0, y0+1)*(1-tx) + at(x0+1, y0+1)*tx
	return top*(1-ty) + bot*ty
}

// digPhase is a stable per-hole angle, so the same hit always bites the same shape.
func digPhase(x, y float64) float64 {
	h := uint64(int64(x*16))*0x9E3779B97F4A7C15 ^ uint64(int64(y*16))*0xC2B2AE3D27D4EB4F // #nosec G115 -- hashing the bits is the intent
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	return float64(h>>11) / float64(uint64(1)<<53) * 2 * math.Pi
}

// digRadiusAt is the bite's radius at a given angle: a couple of harmonics so it is
// organic. It stays STAR-SHAPED on purpose — a probabilistic rim would leave isolated
// teeth of rock inside the hole.
func digRadiusAt(r, ang, phase float64) float64 {
	w := 0.6*math.Sin(ang*3+phase) + 0.4*math.Sin(ang*5-phase*1.7)
	return r * (1 + digJitter*0.5*w)
}

// digRadius is how much rock a shot of the given damage eats.
func digRadius(dmg int) float64 {
	return math.Min(digBase+float64(dmg)*digPerDmg, digMax)
}

// carve eats an organic bite of rock at (x,y): rock cells inside it become interior
// (and are remembered as dug). Reports whether anything changed.
func (f *floodmap) carve(x, y, r float64) bool {
	if f == nil || r <= 0 {
		return false
	}
	phase := digPhase(x, y)
	maxR := r * (1 + digJitter*0.5)
	minCX := int(math.Floor((x - maxR - f.originX) / f.cell))
	maxCX := int(math.Ceil((x + maxR - f.originX) / f.cell))
	minCY := int(math.Floor((y - maxR - f.originY) / f.cell))
	maxCY := int(math.Ceil((y + maxR - f.originY) / f.cell))
	changed := false
	for cy := max(minCY, 0); cy <= min(maxCY, f.rows-1); cy++ {
		for cx := max(minCX, 0); cx <= min(maxCX, f.cols-1); cx++ {
			i := cy*f.cols + cx
			if f.interior[i] {
				continue // already open space
			}
			px := f.originX + (float64(cx)+0.5)*f.cell
			py := f.originY + (float64(cy)+0.5)*f.cell
			dx, dy := px-x, py-y
			d := math.Hypot(dx, dy)
			if d > maxR || d > digRadiusAt(r, math.Atan2(dy, dx), phase) {
				continue
			}
			f.interior[i] = true
			f.dug[i] = true
			f.hotAge[i] = 0 // it is open space now, not a glowing face
			changed = true
		}
	}
	if !changed {
		return false // a beam already pointing down its own bore-hole scorches nothing
	}
	f.scorch(x, y, maxR+hotBand)
	f.markDirty(x, y, maxR)
	f.holes++
	return true
}

// applyDug replays a remembered excavation onto a freshly built field: every cell the
// player once blasted becomes open space again, and both distance fields and images
// are re-derived from it. hotAge is deliberately not restored — those wounds went cold
// long ago. Idempotent (re-applying the field's own slice changes nothing) and it
// ignores a grid of a different shape, so a resized map cannot corrupt the field.
// Reports whether rock actually moved, so the caller knows to re-derive the A* grid.
func (f *floodmap) applyDug(dug []bool) bool {
	if f == nil || len(dug) != len(f.dug) {
		return false
	}
	changed := false
	dugCount := 0
	for i, d := range dug {
		if !d {
			continue
		}
		dugCount++
		if f.interior[i] {
			continue
		}
		f.interior[i] = true
		f.dug[i] = true
		changed = true
	}
	if changed {
		// drawDug only paints when holes > 0; without this a revisited map shows its old
		// tunnels as the teal exterior (a "shadow") until the next carve bumps the counter.
		f.holes = dugCount
		f.rebuild()
	}
	return changed
}

// scorch sets the rock within reach of a fresh bite white-hot. Only rock glows, so
// only rock is heated; the cells are remembered so cooling touches them alone.
func (f *floodmap) scorch(x, y, reach float64) {
	minCX := max(int(math.Floor((x-reach-f.originX)/f.cell)), 0)
	maxCX := min(int(math.Ceil((x+reach-f.originX)/f.cell)), f.cols-1)
	minCY := max(int(math.Floor((y-reach-f.originY)/f.cell)), 0)
	maxCY := min(int(math.Ceil((y+reach-f.originY)/f.cell)), f.rows-1)
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			i := cy*f.cols + cx
			if f.interior[i] {
				continue
			}
			px := f.originX + (float64(cx)+0.5)*f.cell
			py := f.originY + (float64(cy)+0.5)*f.cell
			if math.Hypot(px-x, py-y) > reach {
				continue
			}
			if f.hotAge[i] < hotEps {
				f.hotCells = append(f.hotCells, int32(i)) // #nosec G115 -- cell count fits an int32 by construction
			}
			f.hotAge[i] = 1
		}
	}
}

// coolHotRock ages every glowing wound one frame and re-bakes just those cells,
// uploading the box they span. Cold cells leave the list, so an untouched map costs
// nothing. Nothing here feeds collision or nav — it is pure light.
func (f *floodmap) coolHotRock() {
	if len(f.hotCells) == 0 {
		return
	}
	x0, y0 := f.cols, f.rows
	x1, y1 := 0, 0
	kept := f.hotCells[:0]
	for _, ci := range f.hotCells {
		i := int(ci)
		f.hotAge[i] *= hotDecay
		if f.hotAge[i] < hotEps {
			f.hotAge[i] = 0 // one last bake below wipes the tint
		} else {
			kept = append(kept, ci)
		}
		f.bakeCell(i)
		cx, cy := i%f.cols, i/f.cols
		x0, y0 = min(x0, cx), min(y0, cy)
		x1, y1 = max(x1, cx), max(y1, cy)
	}
	f.hotCells = kept
	f.uploadRect(x0, y0, x1, y1)
}

// minRockCells is the smallest island of rock left standing. Anything under it is
// erased: a single leftover cell blocks a disc of the SHIP's radius (22) around it,
// so a one-cell chip of wall is an invisible snag. Islands connected to the main mass
// are never touched — only chips fully enclosed by what was just dug.
const minRockCells = 40

// pruneSlivers erases isolated chips of rock inside the freshly dug box. A component
// that reaches the window border is (assumed) connected to the main rock and kept.
// Reports whether anything was erased.
func (f *floodmap) pruneSlivers(x0, y0, x1, y1 float64) bool {
	const pad = 3 // cells of slack around the dirty box
	cx0 := max(int(math.Floor((x0-f.originX)/f.cell))-pad, 0)
	cy0 := max(int(math.Floor((y0-f.originY)/f.cell))-pad, 0)
	cx1 := min(int(math.Ceil((x1-f.originX)/f.cell))+pad, f.cols-1)
	cy1 := min(int(math.Ceil((y1-f.originY)/f.cell))+pad, f.rows-1)
	if cx1 <= cx0 || cy1 <= cy0 {
		return false
	}

	w, h := cx1-cx0+1, cy1-cy0+1
	seen := make([]bool, w*h)
	changed := false
	for wy := range h {
		for wx := range w {
			if seen[wy*w+wx] {
				continue
			}
			cx, cy := cx0+wx, cy0+wy
			if f.interior[cy*f.cols+cx] {
				continue // open space, not rock
			}
			// Flood this rock island, confined to the window.
			island := []int{}
			queue := []int{wy*w + wx}
			seen[wy*w+wx] = true
			touchesBorder := false
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				qx, qy := cur%w, cur/w
				island = append(island, (cy0+qy)*f.cols+(cx0+qx))
				if qx == 0 || qy == 0 || qx == w-1 || qy == h-1 {
					touchesBorder = true // runs out of the window: part of the main mass
				}
				for _, d := range neigh4 {
					nx, ny := qx+d[0], qy+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h || seen[ny*w+nx] {
						continue
					}
					if f.interior[(cy0+ny)*f.cols+(cx0+nx)] {
						continue
					}
					seen[ny*w+nx] = true
					queue = append(queue, ny*w+nx)
				}
			}
			if touchesBorder || len(island) >= minRockCells {
				continue
			}
			for _, i := range island {
				f.interior[i] = true
				f.dug[i] = true
			}
			changed = true
		}
	}
	return changed
}

// digAt eats a bite of radius r out of the rock and throws the debris. Nil-safe
// (procedural maps have no field). Reports whether any rock actually came away — a
// beam already pointing down its own bore-hole removes nothing and sheds no chips.
func (g *Game) digAt(x, y, r float64) bool {
	if g.flood == nil || !g.flood.carve(x, y, r) {
		return false
	}
	g.emitBurst(x, y, rockChunks(r))
	g.emitBurst(x, y, rockGrit(r))
	return true
}

// dig blasts rock at a shot's impact point, sized by the shot's damage.
func (g *Game) dig(x, y float64, dmg int) {
	g.digAt(x, y, digRadius(dmg))
}

// digBlast eats a crater out of the rock around an explosion.
func (g *Game) digBlast(x, y, radius float64) {
	g.digAt(x, y, radius*blastDigMul)
}

// refreshFlood re-runs the distance transforms, re-uploads the images and re-derives
// the A* cells the dig freed — once, when a dig dirtied them. Called at the top of Draw.
func (g *Game) refreshFlood() {
	f := g.flood
	if f == nil {
		return
	}
	if f.dirty {
		x0, y0, x1, y1 := f.dirtyMinX, f.dirtyMinY, f.dirtyMaxX, f.dirtyMaxY
		f.pruneSlivers(x0, y0, x1, y1)  // erase chips of rock that would snag the ship
		f.rebuildWindow(x0, y0, x1, y1) // clears dirty; clearance is only valid after this
		cx, cy := (x0+x1)/2, (y0+y1)/2
		g.navRefreshAround(cx, cy, math.Hypot(x1-x0, y1-y0)/2)
	}
	f.coolHotRock() // every frame: the wounds cool whether or not anything was dug
}

// nearestNonSolid finds the closest non-solid cell to (cx,cy), used to nudge the
// flood-fill seed off a wall. Returns -1 if none is near.
func nearestNonSolid(solid []bool, cols, rows, cx, cy int) int {
	for r := range 24 {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				nx, ny := cx+dx, cy+dy
				if nx < 0 || ny < 0 || nx >= cols || ny >= rows {
					continue
				}
				if !solid[ny*cols+nx] {
					return ny*cols + nx
				}
			}
		}
	}
	return -1
}

// sync lazily uploads the baked images to the GPU (safe to build before the game
// loop starts).
func (f *floodmap) sync() {
	if f.img == nil {
		f.img = ebiten.NewImage(f.cols, f.rows)
		f.img.WritePixels(f.pix)
	}
	if f.dugImg == nil {
		f.dugImg = ebiten.NewImage(f.cols, f.rows)
		f.dugImg.WritePixels(f.dugPix)
	}
}

// gridGeoM maps the cell grid onto the world through the camera.
func (f *floodmap) gridGeoM(cam ebiten.GeoM) ebiten.GeoM {
	var m ebiten.GeoM
	m.Scale(f.cell, f.cell)           // texel -> world units
	m.Translate(f.originX, f.originY) // shift for the grid padding
	m.Concat(cam)                     // world -> screen
	return m
}

// drawFloodGlow adds the baked boundary glow over the world, sampled with linear
// filtering so the cells blur into a smooth gradient.
func (g *Game) drawFloodGlow(dst *ebiten.Image, cam ebiten.GeoM) {
	f := g.flood
	if f == nil {
		return
	}
	f.sync()
	var op ebiten.DrawImageOptions
	op.GeoM = f.gridGeoM(cam)
	op.Blend = ebiten.BlendLighter
	op.Filter = ebiten.FilterLinear
	// No ColorScale: the colour is already baked per texel, so a hot rim can sit
	// beside cold rock in the same image.
	dst.DrawImage(f.img, &op)
}

// drawDug paints the rock the player has blasted away as navigable black, over the
// hand-authored corridors.
func (g *Game) drawDug(dst *ebiten.Image, cam ebiten.GeoM) {
	f := g.flood
	if f == nil || f.holes == 0 {
		return
	}
	f.sync()
	var op ebiten.DrawImageOptions
	op.GeoM = f.gridGeoM(cam)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(f.dugImg, &op)
}

// navigablePath is the flattened wall outline in WORLD space, built once per map
// (walls never move at runtime; digging edits the flood grid, not the walls).
// The old fillNavigable re-flattened every curved wall and rebuilt the path in
// screen space each frame — twice (the carve and the fog mask) — which was pure
// per-frame CPU churn for geometry that never changes.
func (g *Game) navigablePath() *vector.Path {
	if g.navPath != nil {
		return g.navPath
	}
	if g.level == nil {
		return nil
	}
	p := &vector.Path{}
	for li := range g.level.Walls {
		for _, wp := range g.level.Walls[li].Paths {
			fp := wp.Flatten() // curved walls carve as their flattened segments
			for _, c := range fp.Commands {
				switch c.Op {
				case asset.OpMoveTo:
					p.MoveTo(float32(c.X), float32(c.Y))
				case asset.OpLineTo:
					p.LineTo(float32(c.X), float32(c.Y))
				case asset.OpClose:
					p.Close()
				}
			}
		}
	}
	g.navPath = p
	return p
}

// fillNavigable fills the reachable corridors with col, using the even-odd rule
// over the wall paths: inside the boundary but outside the solid blocks (the
// corridors) is the odd-overlap region. Assumes closed wall shapes; enclosed
// pockets would need the flood grid instead (TODO).
func (g *Game) fillNavigable(dst *ebiten.Image, cam ebiten.GeoM, col color.RGBA) {
	src := g.navigablePath()
	if src == nil {
		return
	}
	g.navWork.Reset()
	g.navWork.AddPath(src, &vector.AddPathOptions{GeoM: cam})
	var cs ebiten.ColorScale
	cs.ScaleWithColor(col)
	vector.FillPath(dst, &g.navWork, &vector.FillOptions{FillRule: vector.FillRuleEvenOdd}, &vector.DrawPathOptions{
		ColorScale: cs,
		AntiAlias:  true,
	})
}

// carveNavigable fills the reachable corridors with crisp black, so the inner
// silhouette (glow meeting navigable space) is sharp instead of bled.
func (g *Game) carveNavigable(dst *ebiten.Image, cam ebiten.GeoM) {
	g.fillNavigable(dst, cam, colorBg)
}
