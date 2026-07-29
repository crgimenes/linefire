package filoio

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// firstPath pulls the sole layer's sole path out of a one-layer asset, failing loudly
// if the shape did not materialize.
func firstPath(t *testing.T, src string) []asset.Command {
	t.Helper()
	a, err := ParseAsset(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Layers) != 1 || len(a.Layers[0].Paths) != 1 {
		t.Fatalf("want one layer with one path, got %+v", a.Layers)
	}
	return a.Layers[0].Paths[0].Commands
}

func genAsset(shape string) string {
	return `(asset "g" (version 2) (size 100 100) (origin 50 50)
		(layer "main" (stroke "#fff") (width 1) (fill "transparent") (glow 1) ` + shape + `)
		(editor (snap-radius 8) (grid-size 8)))`
}

// TestRectGenerator: (rect ...) is sugar for a closed rectangle. This is the form that
// pays for the whole language — the shipped map0001 is four rectangles by hand.
func TestRectGenerator(t *testing.T) {
	cmds := firstPath(t, genAsset(`(rect 10 20 30 40)`))
	want := []asset.Command{
		{Op: asset.OpMoveTo, X: 10, Y: 20},
		{Op: asset.OpLineTo, X: 40, Y: 20},
		{Op: asset.OpLineTo, X: 40, Y: 60},
		{Op: asset.OpLineTo, X: 10, Y: 60},
		{Op: asset.OpClose},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("rect made %+v, want %+v", cmds, want)
	}
}

// TestCircleGenerator: (circle ...) is four cubic arcs whose endpoints sit on the
// cardinal points, so render and flood fill (which flatten curves) see a real circle.
func TestCircleGenerator(t *testing.T) {
	cmds := firstPath(t, genAsset(`(circle 100 100 50)`))
	if cmds[0].Op != asset.OpMoveTo || cmds[0].X != 150 || cmds[0].Y != 100 {
		t.Fatalf("circle must start at the rightmost point, got %+v", cmds[0])
	}
	cardinals := [][2]float64{{100, 150}, {50, 100}, {100, 50}, {150, 100}}
	for i, want := range cardinals {
		c := cmds[i+1]
		if c.Op != asset.OpCubicTo || len(c.Ctrl) != 2 {
			t.Fatalf("arc %d is not a cubic: %+v", i, c)
		}
		if math.Abs(c.X-want[0]) > 1e-9 || math.Abs(c.Y-want[1]) > 1e-9 {
			t.Fatalf("arc %d ends at (%v,%v), want (%v,%v)", i, c.X, c.Y, want[0], want[1])
		}
	}
	if cmds[len(cmds)-1].Op != asset.OpClose {
		t.Fatal("circle must be closed")
	}

	// Flattening the arcs must trace something a hair's breadth from radius 50 all the
	// way round — the real proof it reads as a circle, not four random curves.
	flat := (asset.Path{Commands: cmds}).Flatten()
	for _, c := range flat.Commands {
		if c.Op == asset.OpClose {
			continue
		}
		d := math.Hypot(c.X-100, c.Y-100)
		if math.Abs(d-50) > 0.5 {
			t.Fatalf("flattened point (%v,%v) is %v from center, not ~50", c.X, c.Y, d)
		}
	}
}

// TestGeneratorsCompose is the reason the language beats a fixed schema: a generator's
// arguments are ordinary expressions, so a symbol and arithmetic feed it directly. An
// inset border is (rect m m (- w 2m) (- h 2m)) with no new field and no engine change.
func TestGeneratorsCompose(t *testing.T) {
	src := `
(def w 200)
(def m 20)
(asset "framed" (version 2) (size w w) (origin 100 100)
  (layer "walls" (stroke "#fff") (width 1) (fill "transparent") (glow 1)
    (rect m m (- w (* 2 m)) (- w (* 2 m))))
  (editor (snap-radius 8) (grid-size 8)))
`
	cmds := firstPath(t, src)
	// corner (20,20), far corner (20+160, 20+160) = (180,180).
	if cmds[0].X != 20 || cmds[0].Y != 20 || cmds[2].X != 180 || cmds[2].Y != 180 {
		t.Fatalf("computed inset rect wrong: %+v", cmds)
	}
}

// TestGeneratorsMaterializeOnSave locks the kutta rule: a generator is authoring sugar,
// not a stored form. Once loaded it is explicit geometry, so Emit writes (path ...) and
// the round-trip test never has to know rect/circle existed.
func TestGeneratorsMaterializeOnSave(t *testing.T) {
	a, err := ParseAsset(genAsset(`(rect 0 0 10 10) (circle 50 50 5)`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := EmitAsset(a)
	if strings.Contains(out, "(rect") || strings.Contains(out, "(circle") {
		t.Fatalf("a saved document must hold resolved geometry, not the generator:\n%s", out)
	}
	if !strings.Contains(out, "(move 0 0)") || !strings.Contains(out, "(cubic ") {
		t.Fatalf("the resolved paths are missing:\n%s", out)
	}
	// And the resolved document still parses to the same model.
	back, err := ParseAsset(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(back.Layers[0].Paths) != 2 {
		t.Fatalf("want two materialized paths, got %d", len(back.Layers[0].Paths))
	}
}

// TestGeneratorsInLevelWalls: the sugar works in a map, not just an asset, because both
// register the shared shape grammar.
func TestGeneratorsInLevelWalls(t *testing.T) {
	l, err := ParseLevel(`(level "g" (version 2) (size 500 500) (player-start 250 250 -90)
		(wall "box" (stroke "#fff") (width 2) (fill "transparent") (glow 1) (rect 20 20 460 460))
		(editor (snap-radius 8) (grid-size 8)))`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(l.Walls[0].Paths[0].Commands) != 5 {
		t.Fatalf("rect wall did not expand: %+v", l.Walls[0].Paths)
	}
}

// TestGeneratorsRejectBadArity: a generator with the wrong argument count fails loudly.
func TestGeneratorsRejectBadArity(t *testing.T) {
	for _, shape := range []string{`(rect 1 2 3)`, `(circle 1 2)`, `(rect 1 2 3 4 5)`} {
		_, err := ParseAsset(genAsset(shape))
		if err == nil {
			t.Fatalf("%s should have failed", shape)
		}
	}
}
