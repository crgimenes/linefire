package editor

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

func vertex(e *Editor) *asset.Command {
	return &e.asset.Layers[0].Paths[0].Commands[0]
}

func setupVertex(e *Editor, x, y float64) {
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{{Op: asset.OpMoveTo, X: x, Y: y}}}}
}

func TestFlipHorizontalAroundOrigin(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 32, Y: 32}
	setupVertex(e, 40, 10)
	e.asset.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, X: 40, Y: 10, Angle: -90}}

	e.flipHorizontal() // scope defaults to asset

	if vertex(e).X != 24 || vertex(e).Y != 10 { // 2*32 - 40 = 24
		t.Fatalf("vertex = (%g,%g), want (24,10)", vertex(e).X, vertex(e).Y)
	}
	if e.asset.Hardpoints[0].X != 24 {
		t.Fatalf("hardpoint X = %g, want 24", e.asset.Hardpoints[0].X)
	}
	// -90 mirrored horizontally stays pointing up: 180 - (-90) = 270 -> -90.
	if e.asset.Hardpoints[0].Angle != -90 {
		t.Fatalf("angle = %g, want -90", e.asset.Hardpoints[0].Angle)
	}
}

func TestFlipVerticalAroundOrigin(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 32, Y: 32}
	setupVertex(e, 40, 10)

	e.flipVertical()
	if vertex(e).X != 40 || vertex(e).Y != 54 { // 2*32 - 10 = 54
		t.Fatalf("vertex = (%g,%g), want (40,54)", vertex(e).X, vertex(e).Y)
	}
}

func TestRotate90KeepsIntegers(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 0, Y: 0}
	setupVertex(e, 10, 0)

	e.rotate(90)
	// (10,0) rotated +90 around origin -> (0,10) in screen coords (y down).
	if vertex(e).X != 0 || vertex(e).Y != 10 {
		t.Fatalf("vertex = (%g,%g), want (0,10)", vertex(e).X, vertex(e).Y)
	}
}

func TestRotateFourTimesReturnsToStart(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 5, Y: 7}
	setupVertex(e, 12, 3)

	for range 4 {
		e.rotate(90)
	}
	if math.Abs(vertex(e).X-12) > 1e-9 || math.Abs(vertex(e).Y-3) > 1e-9 {
		t.Fatalf("vertex = (%g,%g), want (12,3)", vertex(e).X, vertex(e).Y)
	}
}

func TestScopeLayerLeavesHardpointsAlone(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 0, Y: 0}
	setupVertex(e, 10, 0)
	e.asset.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, X: 10, Y: 0, Angle: 0}}

	e.scope = scopeLayer
	e.flipHorizontal()

	if vertex(e).X != -10 {
		t.Fatalf("layer geometry should flip, got X=%g", vertex(e).X)
	}
	if e.asset.Hardpoints[0].X != 10 {
		t.Fatalf("layer scope must not touch hardpoints, got X=%g", e.asset.Hardpoints[0].X)
	}
}

func TestScopePathRequiresSelection(t *testing.T) {
	e := newTestEditor()
	setupVertex(e, 10, 0)
	e.scope = scopePath
	e.hasActive = false

	e.flipHorizontal() // no selected path: must be a no-op
	if vertex(e).X != 10 {
		t.Fatalf("path scope without selection should not transform, got X=%g", vertex(e).X)
	}
}

func TestCycleScopeWraps(t *testing.T) {
	e := newTestEditor()
	if e.scope != scopeAsset {
		t.Fatalf("default scope should be asset")
	}
	e.cycleScope()
	e.cycleScope()
	e.cycleScope()
	if e.scope != scopeAsset {
		t.Fatalf("scope should wrap back to asset, got %s", e.scope)
	}
}
