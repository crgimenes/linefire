package game

import "testing"

func TestNavgridPathRoutesAroundWall(t *testing.T) {
	// 5x5 grid, cell 10. Block column 2 for rows 1..4, leaving a gap at row 0.
	n := &navgrid{cols: 5, rows: 5, cell: 10, blocked: make([]bool, 25)}
	for cy := 1; cy < 5; cy++ {
		n.blocked[cy*5+2] = true
	}

	// From the left of the wall (cell 0,2) to the right (cell 4,2).
	path := n.findPath(5, 25, 45, 25)
	if len(path) == 0 {
		t.Fatal("expected a path around the wall")
	}
	last := path[len(path)-1]
	if last.x < 40 {
		t.Fatalf("path did not reach the goal side, last=%v", last)
	}
	// It must route through the top gap (some waypoint in row 0, y < 10).
	top := false
	for _, p := range path {
		if p.y < 10 {
			top = true
		}
	}
	if !top {
		t.Fatal("path should route through the top gap")
	}
}

func TestNavgridDirectPathWhenClear(t *testing.T) {
	n := &navgrid{cols: 5, rows: 5, cell: 10, blocked: make([]bool, 25)}
	path := n.findPath(5, 5, 45, 5) // straight line, no walls
	if len(path) == 0 {
		t.Fatal("expected a path")
	}
	last := path[len(path)-1]
	if last.x < 40 {
		t.Fatalf("path should reach the goal, last=%v", last)
	}
}
