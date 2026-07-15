package game

import (
	"math"
	"testing"

	"linefire/asset"
	"linefire/level"
)

// boxLevel builds a level whose walls form a closed rectangle, with an optional
// extra wall path, so the flood fill is contained.
func boxLevel(w, h float64, extra []asset.Command) *level.Level {
	lvl := level.New()
	lvl.Size = asset.Size{W: w, H: h}
	paths := []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 20, Y: 20},
		{Op: asset.OpLineTo, X: w - 20, Y: 20},
		{Op: asset.OpLineTo, X: w - 20, Y: h - 20},
		{Op: asset.OpLineTo, X: 20, Y: h - 20},
		{Op: asset.OpClose},
	}}}
	if extra != nil {
		paths = append(paths, asset.Path{Commands: extra})
	}
	lvl.Walls[0].Paths = paths
	return lvl
}

func TestFloodInteriorReachableNotOutside(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	segs := wallSegments(lvl)
	f := buildFloodmap(segs, 200, 200, mapBounds(lvl, segs)) // seed at the center
	if f == nil {
		t.Fatal("expected a floodmap")
	}

	at := func(wx, wy float64) bool {
		cx := int((wx - f.originX) / f.cell)
		cy := int((wy - f.originY) / f.cell)
		return f.interior[cy*f.cols+cx]
	}
	if !at(200, 200) {
		t.Fatal("center should be reachable interior")
	}
	if at(5, 5) {
		t.Fatal("a cell outside the box wall must not be interior")
	}
}

// TestApplyDugMarksHolesSoTunnelsRender guards a real playtest bug: returning to a map replays
// the remembered excavation with applyDug onto a freshly built field (holes == 0). drawDug only
// paints the black tunnel mask when holes > 0, so without bumping the counter the old tunnels
// showed the teal exterior ("shadow") until the next carve. applyDug must set holes.
func TestApplyDugMarksHolesSoTunnelsRender(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	segs := wallSegments(lvl)
	f := buildFloodmap(segs, 200, 200, mapBounds(lvl, segs))
	if f == nil {
		t.Fatal("expected a floodmap")
	}
	if f.holes != 0 {
		t.Fatalf("a fresh field has no holes, got %d", f.holes)
	}

	// Remember a couple of blasted cells (rock just outside the box wall), then replay them.
	dug := make([]bool, len(f.dug))
	rockCells := 0
	for i := range f.interior {
		if !f.interior[i] {
			dug[i] = true
			rockCells++
			if rockCells == 2 {
				break
			}
		}
	}
	if !f.applyDug(dug) {
		t.Fatal("replaying blasted rock should change the field")
	}
	if f.holes == 0 {
		t.Fatal("applyDug must set holes so drawDug renders the replayed tunnels")
	}
}

func TestFloodColumnIsNotInterior(t *testing.T) {
	// A small closed column near the center: the bucket can't reach its inside.
	col := []asset.Command{
		{Op: asset.OpMoveTo, X: 180, Y: 180},
		{Op: asset.OpLineTo, X: 220, Y: 180},
		{Op: asset.OpLineTo, X: 220, Y: 220},
		{Op: asset.OpLineTo, X: 180, Y: 220},
		{Op: asset.OpClose},
	}
	lvl := boxLevel(400, 400, col)
	segs := wallSegments(lvl)
	f := buildFloodmap(segs, 60, 60, mapBounds(lvl, segs)) // seed in a corner, away from the column

	idx := func(wx, wy float64) int {
		cx := int((wx - f.originX) / f.cell)
		cy := int((wy - f.originY) / f.cell)
		return cy*f.cols + cx
	}
	if f.interior[idx(200, 200)] { // inside the column
		t.Fatal("inside of a closed column must not be reachable interior")
	}
	if !f.interior[idx(60, 60)] { // open area around the column
		t.Fatal("open area should be reachable interior")
	}
}

func TestFloodGlowIsotropic(t *testing.T) {
	// A centered square block: the glow band must read the same on every side and
	// corner (Euclidean distance), not pinch on some sides (the old grid-BFS bug).
	block := []asset.Command{
		{Op: asset.OpMoveTo, X: 200, Y: 200},
		{Op: asset.OpLineTo, X: 400, Y: 200},
		{Op: asset.OpLineTo, X: 400, Y: 400},
		{Op: asset.OpLineTo, X: 200, Y: 400},
		{Op: asset.OpClose},
	}
	lvl := boxLevel(600, 600, block)
	segs := wallSegments(lvl)
	f := buildFloodmap(segs, 80, 80, mapBounds(lvl, segs)) // seed in a corner

	// The glow INTENSITY lives in the alpha channel; RGB carries the (per-cell) colour,
	// premultiplied by it, so a freshly cut face can run hot beside cold rock.
	glow := func(wx, wy float64) float64 {
		cx := int((wx - f.originX) / f.cell)
		cy := int((wy - f.originY) / f.cell)
		return float64(f.pix[(cy*f.cols+cx)*4+3]) / 255
	}
	// 10 units in from each edge: the glow follows the Euclidean distance to the
	// wall, so every side reads near the same falloff value — unlike the old
	// grid-BFS metric, which pinched some sides. The tolerance allows for one
	// cell of sampling quantization (fillCell over glowRange).
	want := (glowRange - 10) / glowRange
	const tol = 0.3
	for _, p := range [][2]float64{{300, 210}, {300, 390}, {210, 300}, {390, 300}} {
		g := glow(p[0], p[1])
		if math.Abs(g-want) > tol {
			t.Fatalf("glow at %v = %v, want ~%v (isotropic Euclidean falloff)", p, g, want)
		}
	}
}
