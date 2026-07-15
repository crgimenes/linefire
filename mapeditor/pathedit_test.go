package mapeditor

import (
	"testing"

	"linefire/asset"
)

// openPath builds an open polyline (M + L…) from world points.
func openPath(pts ...[2]float64) asset.Path {
	cmds := []asset.Command{{Op: asset.OpMoveTo, X: pts[0][0], Y: pts[0][1]}}
	for _, p := range pts[1:] {
		cmds = append(cmds, asset.Command{Op: asset.OpLineTo, X: p[0], Y: p[1]})
	}
	return asset.Path{Commands: cmds}
}

func wantCmds(t *testing.T, got []asset.Command, want [][3]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d commands, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].X != w[1] || got[i].Y != w[2] {
			t.Fatalf("cmd %d = (%.0f,%.0f), want (%.0f,%.0f)", i, got[i].X, got[i].Y, w[1], w[2])
		}
	}
}

// TestReversePathFlipsSegments: a reversed path visits points end-to-start and a cubic's two
// handles swap.
func TestReversePathFlipsSegments(t *testing.T) {
	c1 := asset.Point{X: 12, Y: 1}
	c2 := asset.Point{X: 18, Y: 9}
	in := []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0},
		{Op: asset.OpLineTo, X: 10, Y: 0},
		{Op: asset.OpCubicTo, X: 20, Y: 10, Ctrl: []asset.Point{c1, c2}},
	}
	got := reversePath(in)
	wantCmds(t, got, [][3]float64{{0, 20, 10}, {0, 10, 0}, {0, 0, 0}})
	if got[0].Op != asset.OpMoveTo {
		t.Fatalf("reversed path must start with a move, got %q", got[0].Op)
	}
	if got[1].Op != asset.OpCubicTo || got[1].Ctrl[0] != c2 || got[1].Ctrl[1] != c1 {
		t.Fatalf("cubic handles must swap on reverse, got %+v", got[1])
	}
}

// TestJoinMergesTwoOpenPaths: joining A's END to B's START splices them into one path with a
// connecting line, dropping B.
func TestJoinMergesTwoOpenPaths(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{
		openPath([2]float64{0, 0}, [2]float64{10, 0}),
		openPath([2]float64{20, 0}, [2]float64{30, 0}),
	}
	e.doJoin(
		handle{kind: hWallVertex, layer: 0, path: 0, cmd: 1, x: 10, y: 0}, // A end
		handle{kind: hWallVertex, layer: 0, path: 1, cmd: 0, x: 20, y: 0}, // B start
	)
	if len(e.level.Walls[0].Paths) != 1 {
		t.Fatalf("join should merge into 1 path, got %d", len(e.level.Walls[0].Paths))
	}
	wantCmds(t, e.level.Walls[0].Paths[0].Commands, [][3]float64{{0, 0, 0}, {0, 10, 0}, {0, 20, 0}, {0, 30, 0}})
}

// TestJoinReversesToConnect: joining A's START to B's END reverses both so the free ends meet.
func TestJoinReversesToConnect(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{
		openPath([2]float64{0, 0}, [2]float64{10, 0}),
		openPath([2]float64{20, 0}, [2]float64{30, 0}),
	}
	e.doJoin(
		handle{kind: hWallVertex, layer: 0, path: 0, cmd: 0, x: 0, y: 0},  // A start
		handle{kind: hWallVertex, layer: 0, path: 1, cmd: 1, x: 30, y: 0}, // B end
	)
	wantCmds(t, e.level.Walls[0].Paths[0].Commands, [][3]float64{{0, 10, 0}, {0, 0, 0}, {0, 30, 0}, {0, 20, 0}})
}

// TestJoinClosesSamePath: joining a single path's two ends closes it.
func TestJoinClosesSamePath(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{openPath([2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 10})}
	e.doJoin(
		handle{kind: hWallVertex, layer: 0, path: 0, cmd: 0, x: 0, y: 0},   // start
		handle{kind: hWallVertex, layer: 0, path: 0, cmd: 2, x: 10, y: 10}, // end
	)
	if !e.level.Walls[0].Paths[0].Closed() {
		t.Fatalf("joining a path's two ends should close it: %+v", e.level.Walls[0].Paths[0].Commands)
	}
}

// TestDeleteVertexOpensClosedLoop: deleting a vertex from a closed wall REOPENS it (a gap where the
// vertex was) instead of keeping it closed / bridging the neighbours.
func TestDeleteVertexOpensClosedLoop(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0}, {Op: asset.OpLineTo, X: 10, Y: 0},
		{Op: asset.OpLineTo, X: 10, Y: 10}, {Op: asset.OpLineTo, X: 0, Y: 10}, {Op: asset.OpClose},
	}}}
	e.active = handle{kind: hWallVertex, layer: 0, path: 0, cmd: 2} // the (10,10) corner
	e.hasActive = true
	e.deleteSelected()

	if len(e.level.Walls[0].Paths) != 1 {
		t.Fatalf("cutting a closed loop should leave one open path, got %d", len(e.level.Walls[0].Paths))
	}
	if e.level.Walls[0].Paths[0].Closed() {
		t.Fatal("deleting a vertex must OPEN the loop, not keep it closed")
	}
	// Reopened, rotated to start just past the deleted corner: (0,10) (0,0) (10,0).
	wantCmds(t, e.level.Walls[0].Paths[0].Commands, [][3]float64{{0, 0, 10}, {0, 0, 0}, {0, 10, 0}})
}

// TestDeleteVertexSplitsOpenPath: deleting a mid vertex of an open path splits it into two pieces
// rather than joining the neighbours.
func TestDeleteVertexSplitsOpenPath(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{openPath(
		[2]float64{0, 0}, [2]float64{10, 0}, [2]float64{20, 0}, [2]float64{30, 0}, [2]float64{40, 0})}
	e.active = handle{kind: hWallVertex, layer: 0, path: 0, cmd: 2} // the (20,0) vertex
	e.hasActive = true
	e.deleteSelected()

	if len(e.level.Walls[0].Paths) != 2 {
		t.Fatalf("cutting a mid vertex should split into 2 paths, got %d", len(e.level.Walls[0].Paths))
	}
	wantCmds(t, e.level.Walls[0].Paths[0].Commands, [][3]float64{{0, 0, 0}, {0, 10, 0}})
	wantCmds(t, e.level.Walls[0].Paths[1].Commands, [][3]float64{{0, 30, 0}, {0, 40, 0}})
}

// TestClickLooseEndArmsThenWelds: the implicit two-click join — first click arms, second welds the
// two open paths into one (kutta's click-two-red-ends model, no tool switch).
func TestClickLooseEndArmsThenWelds(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{
		openPath([2]float64{0, 0}, [2]float64{10, 0}),
		openPath([2]float64{20, 0}, [2]float64{30, 0}),
	}

	e.clickLooseEnd(handle{kind: hWallVertex, layer: 0, path: 0, cmd: 1, x: 10, y: 0})
	if !e.hasJoinFrom {
		t.Fatal("the first click on a loose end should arm the join")
	}
	e.clickLooseEnd(handle{kind: hWallVertex, layer: 0, path: 1, cmd: 0, x: 20, y: 0})
	if e.hasJoinFrom {
		t.Fatal("the second click should complete the join")
	}
	if len(e.level.Walls[0].Paths) != 1 {
		t.Fatalf("the two open paths should be welded into one, got %d", len(e.level.Walls[0].Paths))
	}
}

// TestInsertEdgeVertexOnSquare: Shift+click near an edge splits it with a new vertex — including a
// closed path's implicit closing edge — leaving the path's shape intact.
func TestInsertEdgeVertexOnSquare(t *testing.T) {
	e := newTestMapEditor()
	e.cam.View.Scale, e.cam.View.OffsetX, e.cam.View.OffsetY = 1, 0, 0 // screen == world
	e.level.Editor.SnapToGrid = false
	e.level.Walls[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0}, {Op: asset.OpLineTo, X: 10, Y: 0},
		{Op: asset.OpLineTo, X: 10, Y: 10}, {Op: asset.OpLineTo, X: 0, Y: 10}, {Op: asset.OpClose},
	}}}

	h, ok := e.insertEdgeVertexAt(5, 1) // near the middle of the top edge
	if !ok {
		t.Fatal("a click 1px from an edge should insert a vertex")
	}
	if h.x != 5 || h.y != 0 {
		t.Fatalf("vertex should land on the edge at (5,0), got (%.0f,%.0f)", h.x, h.y)
	}
	cmds := e.level.Walls[0].Paths[0].Commands
	if len(cmds) != 6 || cmds[1].X != 5 || cmds[1].Y != 0 {
		t.Fatalf("the top edge should be split at (5,0): %+v", cmds)
	}
	if !e.level.Walls[0].Paths[0].Closed() {
		t.Fatal("inserting on an edge must not open the path")
	}

	// The closing edge (left side, from (0,10) back to (0,0)) is insertable too.
	_, ok = e.insertEdgeVertexAt(1, 5)
	if !ok {
		t.Fatal("the implicit closing edge should accept an inserted vertex")
	}

	// Far from any edge: nothing happens.
	if _, ok := e.insertEdgeVertexAt(50, 50); ok {
		t.Fatal("a click far from every edge must not insert")
	}
}

// TestWallEndpointClassifies: only the start/end vertex of an OPEN path is a joinable endpoint.
func TestWallEndpointClassifies(t *testing.T) {
	e := newTestMapEditor()
	closed := asset.Path{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0}, {Op: asset.OpLineTo, X: 10, Y: 0},
		{Op: asset.OpLineTo, X: 10, Y: 10}, {Op: asset.OpClose},
	}}
	e.level.Walls[0].Paths = []asset.Path{closed, openPath([2]float64{0, 0}, [2]float64{5, 0}, [2]float64{10, 0})}

	if _, ok := e.wallEndpoint(handle{kind: hWallVertex, layer: 0, path: 0, cmd: 0}); ok {
		t.Fatal("a closed path has no joinable endpoint")
	}
	if _, ok := e.wallEndpoint(handle{kind: hWallVertex, layer: 0, path: 1, cmd: 1}); ok {
		t.Fatal("a mid vertex is not an endpoint")
	}
	if atStart, ok := e.wallEndpoint(handle{kind: hWallVertex, layer: 0, path: 1, cmd: 0}); !ok || !atStart {
		t.Fatalf("open-path start should be the start endpoint, got atStart=%v ok=%v", atStart, ok)
	}
	if atStart, ok := e.wallEndpoint(handle{kind: hWallVertex, layer: 0, path: 1, cmd: 2}); !ok || atStart {
		t.Fatalf("open-path end should be the end endpoint, got atStart=%v ok=%v", atStart, ok)
	}
}
