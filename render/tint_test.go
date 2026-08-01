package render

import (
	"image/color"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

func tintTestMesh(t *testing.T) *Mesh {
	t.Helper()
	m := BuildLayersMesh([]asset.Layer{{
		Stroke: "#ff8040", StrokeWidth: 2, Fill: "#802010",
		Paths: []asset.Path{{Commands: []asset.Command{
			{Op: asset.OpMoveTo, X: 0, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 10},
			{Op: asset.OpClose},
		}}},
	}})
	if m.Empty() {
		t.Fatal("test mesh built empty")
	}
	return m
}

// The whole point of retinting by hue: the faction owns the hue, the art keeps
// its own saturation and value — a straight blend would drag saturation and
// wash the fleet out (the regression that taught us this).
func TestRetintedMovesHueKeepsSaturationAndValue(t *testing.T) {
	m := tintTestMesh(t)
	cyan := color.RGBA{0x55, 0xff, 0xff, 0xff}
	th, _, _ := rgbToHSV(cyan)

	got := m.Retinted(cyan)
	if len(got.paths) != len(m.paths) {
		t.Fatalf("retint changed the path count: %d != %d", len(got.paths), len(m.paths))
	}
	for i := range m.paths {
		before, after := m.paths[i].stroke, got.paths[i].stroke
		h, s, v := rgbToHSV(after)
		if diff := abs(h - th); diff > 1 && diff < 359 {
			t.Errorf("stroke hue %f, want the target's %f", h, th)
		}
		_, s0, v0 := rgbToHSV(before)
		if abs(s-s0) > 0.02 || abs(v-v0) > 0.02 {
			t.Errorf("stroke S/V drifted: %f/%f -> %f/%f", s0, v0, s, v)
		}
		if after.A != before.A {
			t.Errorf("alpha changed: %d -> %d", before.A, after.A)
		}
	}
}

// An achromatic target has no hue, so the art desaturates instead — the white
// faction flies greyscale hulls rather than keeping the original colors.
func TestRetintedAchromaticTargetDesaturates(t *testing.T) {
	m := tintTestMesh(t)
	got := m.Retinted(color.RGBA{0xff, 0xff, 0xff, 0xff})
	for i := range got.paths {
		c := got.paths[i].stroke
		if c.R != c.G || c.G != c.B {
			t.Errorf("stroke %v is not greyscale", c)
		}
	}
}

// The original mesh must be untouched: skins share geometry, never colors.
func TestRetintedLeavesTheOriginalAlone(t *testing.T) {
	m := tintTestMesh(t)
	before := m.paths[0].stroke
	m.Retinted(color.RGBA{0x55, 0xff, 0x55, 0xff})
	if m.paths[0].stroke != before {
		t.Error("retint mutated the original mesh")
	}
	if got := (*Mesh)(nil).Retinted(color.RGBA{}); got != nil {
		t.Error("a nil mesh should retint to nil")
	}
}
