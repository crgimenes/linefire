package svgimport

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// TestPathSquareImportsClosedAndYDown: a square path becomes one closed wall path in the
// drawing's own coordinates — crucially y is NOT flipped (the game is y-down like SVG).
func TestPathSquareImportsClosedAndYDown(t *testing.T) {
	paths, err := Paths([]byte(`<svg><path d="M 0 0 L 100 0 L 100 100 L 0 100 Z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 wall path, got %d", len(paths))
	}
	cmds := paths[0].Commands
	if len(cmds) != 5 { // M + 3 L + Z
		t.Fatalf("want 5 commands (M,L,L,L,Z), got %d: %+v", len(cmds), cmds)
	}
	if cmds[0].Op != asset.OpMoveTo || cmds[0].X != 0 || cmds[0].Y != 0 {
		t.Fatalf("first command should be MoveTo (0,0), got %+v", cmds[0])
	}
	if cmds[len(cmds)-1].Op != asset.OpClose {
		t.Fatalf("last command should be Close, got %+v", cmds[len(cmds)-1])
	}
	// The bottom edge sits at y=+100 (down), proving no y-flip: a normalizing import would
	// have moved it to a negative, centered coordinate.
	if cmds[3].Y != 100 {
		t.Fatalf("y should be preserved down-positive, got %v", cmds[3].Y)
	}
}

func TestRectImportsFourCorners(t *testing.T) {
	paths, err := Paths([]byte(`<svg><rect x="10" y="20" width="30" height="40"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 path, got %d", len(paths))
	}
	corners := endpoints(paths[0])
	want := []asset.Point{{X: 10, Y: 20}, {X: 40, Y: 20}, {X: 40, Y: 60}, {X: 10, Y: 60}}
	if len(corners) != 4 {
		t.Fatalf("a rect should be 4 corners, got %d", len(corners))
	}
	for i, w := range want {
		if corners[i] != w {
			t.Fatalf("corner %d = %+v, want %+v", i, corners[i], w)
		}
	}
}

func TestCircleFlattensToPolygonOnRadius(t *testing.T) {
	paths, err := Paths([]byte(`<svg><circle cx="50" cy="50" r="10"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	pts := endpoints(paths[0])
	if len(pts) != ellipseSegments {
		t.Fatalf("a circle should flatten to %d points, got %d", ellipseSegments, len(pts))
	}
	for _, p := range pts {
		if r := math.Hypot(p.X-50, p.Y-50); math.Abs(r-10) > 1e-6 {
			t.Fatalf("every point should sit on radius 10, got %.4f", r)
		}
	}
}

// TestRelativeCurveFlattens: a relative moveto plus a cubic curve import as a many-point
// polyline anchored at the right place (exercises relative coords and curve flattening).
func TestRelativeCurveFlattens(t *testing.T) {
	// m 100 100 -> start (100,100); c relative cubic ending at (100+60,100) = (160,100).
	paths, err := Paths([]byte(`<svg><path d="m 100 100 c 20 -40 40 -40 60 0 z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	pts := endpoints(paths[0])
	if len(pts) < curveSegments {
		t.Fatalf("a cubic should flatten to many points, got %d", len(pts))
	}
	if pts[0] != (asset.Point{X: 100, Y: 100}) {
		t.Fatalf("subpath should start at the relative moveto (100,100), got %+v", pts[0])
	}
	last := pts[len(pts)-1]
	if math.Abs(last.X-160) > 1e-6 || math.Abs(last.Y-100) > 1e-6 {
		t.Fatalf("cubic should end at (160,100), got %+v", last)
	}
}

// TestShapesCarryLabelAndCentroid: a shape's marker label is its inkscape:label (preferred)
// or its id, and Centroid is the shape's center — what spawn-marker placement needs.
func TestShapesCarryLabelAndCentroid(t *testing.T) {
	svg := `<svg xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
		<rect id="autoid1234" inkscape:label="turret" x="0" y="0" width="100" height="100"/>
		<circle id="portal:map0002" cx="200" cy="80" r="10"/>
		<rect x="0" y="0" width="10" height="10"/>
	</svg>`
	shapes, err := Shapes([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 3 {
		t.Fatalf("want 3 shapes, got %d", len(shapes))
	}
	if shapes[0].Label != "turret" {
		t.Fatalf("inkscape:label should win over id, got %q", shapes[0].Label)
	}
	if shapes[1].Label != "portal:map0002" {
		t.Fatalf("id should be the label when there is no inkscape:label, got %q", shapes[1].Label)
	}
	if shapes[2].Label != "" {
		t.Fatalf("an unmarked shape should have an empty label, got %q", shapes[2].Label)
	}
	if c := Centroid(shapes[0].Points); c.X != 50 || c.Y != 50 {
		t.Fatalf("the 100x100 rect's centroid should be (50,50), got %+v", c)
	}
}

func TestHiddenElementSkipped(t *testing.T) {
	svg := `<svg>
		<rect x="0" y="0" width="10" height="10" style="display:none"/>
		<rect x="0" y="0" width="20" height="20"/>
	</svg>`
	paths, err := Paths([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("the display:none rect must be skipped, got %d paths", len(paths))
	}
}

func TestEmptyDrawingErrors(t *testing.T) {
	_, err := Paths([]byte(`<svg></svg>`))
	if err == nil {
		t.Fatal("an SVG with no shapes should error, not return an empty map")
	}
}

// endpoints returns the (X,Y) of every non-close command in a path.
func endpoints(p asset.Path) []asset.Point {
	var out []asset.Point
	for _, c := range p.Commands {
		if c.Op == asset.OpClose {
			continue
		}
		out = append(out, asset.Point{X: c.X, Y: c.Y})
	}
	return out
}
