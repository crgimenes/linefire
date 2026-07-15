package render

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"linefire/asset"
)

func TestBuildLayersMesh(t *testing.T) {
	layers := []asset.Layer{{
		Name: "main", Stroke: "#80ffff", StrokeWidth: 2, Fill: "#001820", Glow: 0.8,
		Paths: []asset.Path{{Commands: []asset.Command{
			{Op: asset.OpMoveTo, X: 0, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 10},
			{Op: asset.OpClose},
		}}},
	}}
	m := BuildLayersMesh(layers)
	if m.Empty() {
		t.Fatal("expected a non-empty mesh for a filled, stroked, closed path")
	}
}

func TestBuildLayersMeshEmpty(t *testing.T) {
	if !BuildLayersMesh(nil).Empty() {
		t.Error("nil layers should make an empty mesh")
	}
	// Hidden layer contributes nothing.
	layers := []asset.Layer{{Name: "h", Stroke: "#fff", StrokeWidth: 2, Hidden: true,
		Paths: []asset.Path{{Commands: []asset.Command{{Op: asset.OpMoveTo}, {Op: asset.OpLineTo, X: 5}}}}}}
	if !BuildLayersMesh(layers).Empty() {
		t.Error("hidden layer should make an empty mesh")
	}
}

func TestBuildGlowMeshSkipsZeroGlow(t *testing.T) {
	layers := []asset.Layer{{
		Name: "w", Stroke: "#80ffff", StrokeWidth: 2, Fill: "transparent", Glow: 0,
		Paths: []asset.Path{{Commands: []asset.Command{{Op: asset.OpMoveTo}, {Op: asset.OpLineTo, X: 10}}}},
	}}
	if !BuildGlowMesh(layers).Empty() {
		t.Error("glow=0 layer should not emit")
	}
	layers[0].Glow = 0.8
	if BuildGlowMesh(layers).Empty() {
		t.Error("glow>0 layer should emit a stroke mesh")
	}
}

func TestBuildLayersMeshCachesVectorPaths(t *testing.T) {
	layers := []asset.Layer{{
		Name: "wall", Stroke: "#80ffff", StrokeWidth: 2, Fill: "#001820",
		Paths: []asset.Path{{Commands: []asset.Command{
			{Op: asset.OpMoveTo, X: 0, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 10},
			{Op: asset.OpClose},
		}}},
	}}
	m := BuildLayersMesh(layers)
	if m.Empty() {
		t.Fatal("expected non-empty mesh")
	}
	if len(m.paths) != 1 {
		t.Fatalf("paths = %d, want 1", len(m.paths))
	}
	if !m.paths[0].fillVisible {
		t.Fatal("expected fill")
	}
	if m.paths[0].strokeWidth == 0 {
		t.Fatal("expected stroke")
	}
}

func TestMeshPathVisibleAppliesGeoM(t *testing.T) {
	bounds := image.Rect(0, 0, 8, 8)
	var geo ebiten.GeoM
	geo.Translate(12, 12)
	screen := image.Rect(10, 10, 24, 24)

	if !meshPathVisible(bounds, geo, screen) {
		t.Fatal("translated bounds should be visible")
	}
}

func TestGeoScale(t *testing.T) {
	var geo ebiten.GeoM
	geo.Scale(0.6, 0.6)
	geo.Rotate(0.7)
	got := geoScale(geo)
	if math.Abs(got-0.6) > 1e-9 {
		t.Fatalf("geoScale = %f, want 0.6", got)
	}
}

func TestScaleColor(t *testing.T) {
	base := color.RGBA{R: 100, G: 50, B: 10, A: 255}
	c := scaleColor(base, 2)
	if c.R != 200 || c.A != 255 {
		t.Errorf("scaleColor x2 = %+v, want R=200 A=255", c)
	}
	c = scaleColor(base, 4)
	if c.R != 255 { // 400 clamps to 255
		t.Errorf("scaleColor clamp R = %d, want 255", c.R)
	}
}
