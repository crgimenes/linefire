package game

import (
	"reflect"
	"testing"
)

// TestDugRuns: the automap coalesces dug cells into per-row horizontal runs — one
// rect per contiguous span, split by rock, never crossing a row boundary.
func TestDugRuns(t *testing.T) {
	const cols, rows = 4, 3
	// row 0: cells 1..2 dug; row 1: none; row 2: cell 0 and cells 2..3 dug (two runs).
	dug := make([]bool, cols*rows)
	dug[0*cols+1] = true
	dug[0*cols+2] = true
	dug[2*cols+0] = true
	dug[2*cols+2] = true
	dug[2*cols+3] = true

	got := dugRuns(dug, cols, rows)
	want := []dugRun{
		{y: 0, x0: 1, x1: 3},
		{y: 2, x0: 0, x1: 1},
		{y: 2, x0: 2, x1: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dugRuns = %+v, want %+v", got, want)
	}

	// No dug cells -> no runs (an authored map that was never carved shows no tunnels).
	if runs := dugRuns(make([]bool, cols*rows), cols, rows); len(runs) != 0 {
		t.Fatalf("empty dug should yield no runs, got %+v", runs)
	}
}

func TestZoomMapClamps(t *testing.T) {
	g := &Game{mapZoom: 1}

	for range 50 {
		g.zoomMap(mapZoomStep)
	}
	if g.mapZoom > mapZoomMax {
		t.Fatalf("zoom should clamp to max %v, got %v", mapZoomMax, g.mapZoom)
	}

	for range 100 {
		g.zoomMap(1 / mapZoomStep)
	}
	if g.mapZoom < mapZoomMin {
		t.Fatalf("zoom should clamp to min %v, got %v", mapZoomMin, g.mapZoom)
	}
}
