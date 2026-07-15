package asset

import (
	"math"
	"testing"
)

func TestFlattenNoCurvesUnchanged(t *testing.T) {
	p := Path{Commands: []Command{
		{Op: OpMoveTo, X: 0, Y: 0},
		{Op: OpLineTo, X: 10, Y: 0},
		{Op: OpClose},
	}}
	got := p.Flatten()
	if len(got.Commands) != 3 {
		t.Fatalf("a curveless path should pass through unchanged, got %d commands", len(got.Commands))
	}
}

func TestFlattenQuad(t *testing.T) {
	// Quadratic from (0,0) with control (10,10) to (20,0); apex at t=0.5 is (10,5).
	p := Path{Commands: []Command{
		{Op: OpMoveTo, X: 0, Y: 0},
		{Op: OpQuadTo, X: 20, Y: 0, Ctrl: []Point{{X: 10, Y: 10}}},
	}}
	got := p.Flatten()

	if got.Commands[0].Op != OpMoveTo || got.Commands[0].X != 0 || got.Commands[0].Y != 0 {
		t.Fatalf("flatten should keep the move start: %+v", got.Commands[0])
	}
	if len(got.Commands) < 3 {
		t.Fatalf("curve should subdivide into several segments, got %d", len(got.Commands))
	}
	last := got.Commands[len(got.Commands)-1]
	if last.Op != OpLineTo || last.X != 20 || last.Y != 0 {
		t.Fatalf("flatten should end at the curve endpoint, got %+v", last)
	}
	maxY := 0.0
	for _, c := range got.Commands {
		if c.Y > maxY {
			maxY = c.Y
		}
	}
	if math.Abs(maxY-5) > 1 {
		t.Fatalf("flattened apex should be near y=5, got %v", maxY)
	}
}

func TestFlattenCubic(t *testing.T) {
	p := Path{Commands: []Command{
		{Op: OpMoveTo, X: 0, Y: 0},
		{Op: OpCubicTo, X: 30, Y: 0, Ctrl: []Point{{X: 10, Y: 20}, {X: 20, Y: 20}}},
	}}
	got := p.Flatten()
	if len(got.Commands) < 3 {
		t.Fatalf("cubic should subdivide, got %d", len(got.Commands))
	}
	for _, c := range got.Commands {
		if c.Op != OpMoveTo && c.Op != OpLineTo {
			t.Fatalf("flattened path should be only M/L, found %q", c.Op)
		}
	}
}

func TestValidatePathCurves(t *testing.T) {
	ok := Path{Commands: []Command{
		{Op: OpMoveTo, X: 0, Y: 0},
		{Op: OpQuadTo, X: 10, Y: 0, Ctrl: []Point{{X: 5, Y: 5}}},
		{Op: OpCubicTo, X: 20, Y: 0, Ctrl: []Point{{X: 12, Y: 5}, {X: 18, Y: 5}}},
		{Op: OpClose},
	}}
	err := ValidatePath(ok)
	if err != nil {
		t.Fatalf("valid curves rejected: %v", err)
	}

	noCtrl := Path{Commands: []Command{{Op: OpMoveTo}, {Op: OpQuadTo, X: 10}}}
	if ValidatePath(noCtrl) == nil {
		t.Fatal("a quad without a control point should fail validation")
	}
	wrongCtrl := Path{Commands: []Command{{Op: OpMoveTo}, {Op: OpCubicTo, X: 10, Ctrl: []Point{{X: 5, Y: 5}}}}}
	if ValidatePath(wrongCtrl) == nil {
		t.Fatal("a cubic with one control point should fail validation")
	}
}

func TestPathCloneDeepCopiesCtrl(t *testing.T) {
	p := Path{Commands: []Command{{Op: OpCubicTo, X: 1, Y: 1, Ctrl: []Point{{X: 2, Y: 2}, {X: 3, Y: 3}}}}}
	c := p.Clone()
	c.Commands[0].Ctrl[0].X = 99
	if p.Commands[0].Ctrl[0].X != 2 {
		t.Fatal("Clone must deep-copy control points so edits do not corrupt the original")
	}
}
