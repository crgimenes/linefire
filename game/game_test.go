package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

func TestAssetRadius(t *testing.T) {
	r := assetRadius(nil)
	if r != 6 {
		t.Errorf("nil asset radius = %g, want 6", r)
	}
	a := asset.New() // no collisions
	r = assetRadius(a)
	if r != 6 {
		t.Errorf("default radius = %g, want 6", r)
	}
	a.Collisions = []asset.CollisionShape{{Kind: asset.CollisionCircle, Points: []asset.Point{{X: 0, Y: 0}}, Radius: 20}}
	r = assetRadius(a)
	if r != 20 {
		t.Errorf("radius = %g, want 20", r)
	}
}

func TestWallSegments(t *testing.T) {
	l := level.New()
	l.Walls[0].Paths = []asset.Path{
		{Commands: []asset.Command{ // open polyline: 2 segments
			{Op: asset.OpMoveTo, X: 0, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 10},
		}},
		{Commands: []asset.Command{ // closed triangle: 3 segments
			{Op: asset.OpMoveTo, X: 0, Y: 0},
			{Op: asset.OpLineTo, X: 10, Y: 0},
			{Op: asset.OpLineTo, X: 5, Y: 8},
			{Op: asset.OpClose},
		}},
	}
	segs := wallSegments(l)
	if len(segs) != 5 {
		t.Fatalf("expected 5 segments (2 open + 3 closed), got %d", len(segs))
	}
}

func TestDistPointSegmentSq(t *testing.T) {
	// Point (5,5) to segment (0,0)-(10,0): distance 5 -> sq 25.
	d := distPointSegmentSq(5, 5, 0, 0, 10, 0)
	if d != 25 {
		t.Errorf("dist sq = %g, want 25", d)
	}
	// Beyond the segment end: nearest is the endpoint (10,0).
	d = distPointSegmentSq(13, 4, 0, 0, 10, 0)
	if d != 25 { // (3,4) -> 9+16
		t.Errorf("dist sq = %g, want 25", d)
	}
}

func TestTryMoveBlocksAndSlides(t *testing.T) {
	g := &Game{
		radius: 10,
		x:      85,
		y:      100,
		segs:   []segment{{ax: 100, ay: 0, bx: 100, by: 200}}, // vertical wall at x=100
	}
	g.vx, g.vy = 20, 5
	g.tryMove(20, 5)

	// X move into the wall is blocked; Y move slides freely.
	if g.x != 85 {
		t.Errorf("x = %g, want 85 (blocked by wall)", g.x)
	}
	if g.y != 105 {
		t.Errorf("y = %g, want 105 (slid along wall)", g.y)
	}
}

func TestCameraCentersShipAndFacesUp(t *testing.T) {
	g := &Game{x: 100, y: 200, angle: 40}
	cam := g.cameraGeoM()

	// The ship maps to the screen center.
	sx, sy := cam.Apply(g.x, g.y)
	if math.Abs(sx-screenW/2) > 1e-6 || math.Abs(sy-screenH/2) > 1e-6 {
		t.Fatalf("ship not centered: (%v,%v)", sx, sy)
	}

	// A point straight ahead of the ship maps directly above the center.
	rad := g.angle * math.Pi / 180
	ahead := 20.0
	px, py := cam.Apply(g.x+math.Cos(rad)*ahead, g.y+math.Sin(rad)*ahead)
	if math.Abs(px-screenW/2) > 1e-6 {
		t.Errorf("ahead point not vertically above center: x=%v", px)
	}
	if py >= screenH/2 {
		t.Errorf("ahead point should be above center (smaller y), got y=%v", py)
	}
}

func TestLogicalSizeDefaultsAndOverride(t *testing.T) {
	g := &Game{}
	w, h := g.logicalSize()
	if w != screenW || h != screenH {
		t.Fatalf("default logical size = %dx%d, want %dx%d", w, h, screenW, screenH)
	}
	g.winW, g.winH = 1200, 900
	w, h = g.logicalSize()
	if w != 1200 || h != 900 {
		t.Fatalf("override logical size = %dx%d, want 1200x900", w, h)
	}
}

func TestMenuOriginCentersOnWindow(t *testing.T) {
	g := &Game{winW: 1000, winH: 600, arsenal: []int{catFront, catMissile}, slotArsIdx: [numSlots]int{0, 1}}
	g.syncSlots()
	mx, my := g.menuOrigin()
	// The panel's top-left is the origin; its (estimated) center should sit at the
	// window center.
	panelCX := mx + g.menuW()/2
	panelCY := my + g.menuH()/2
	if panelCX != 1000.0/2 || panelCY != 600.0/2 {
		t.Fatalf("loadout panel center = (%v,%v), want centered on 1000x600", panelCX, panelCY)
	}
}

func TestCameraCentersOnResizedScreen(t *testing.T) {
	// A non-square drawable still maps the ship to the middle of the screen.
	g := &Game{x: 50, y: 70, angle: 30, sw: 1200, sh: 600, dpr: 1}
	cam := g.cameraGeoM()
	sx, sy := cam.Apply(g.x, g.y)
	if math.Abs(sx-600) > 1e-6 || math.Abs(sy-300) > 1e-6 {
		t.Fatalf("ship not centered on resized screen: (%v,%v), want (600,300)", sx, sy)
	}
}

func TestEntityGeoMPlacesOriginAtEntity(t *testing.T) {
	g := &Game{x: 0, y: 0, angle: -90}
	cam := g.cameraGeoM()
	a := asset.New() // origin (32,32)
	e := entity{kind: kindEnemy, x: 120, y: 80, angle: 0, a: a}

	// The asset's origin must land on the entity's camera-projected position.
	wantX, wantY := cam.Apply(e.x, e.y)
	em := g.entityGeoM(&e)
	gotX, gotY := em.Apply(a.Origin.X, a.Origin.Y)
	if math.Abs(gotX-wantX) > 1e-6 || math.Abs(gotY-wantY) > 1e-6 {
		t.Fatalf("entity origin = (%v,%v), want (%v,%v)", gotX, gotY, wantX, wantY)
	}
}

func TestBlurSamplesScalesWithMotion(t *testing.T) {
	g := &Game{sw: screenW, sh: screenH, dpr: 1}

	// No motion: a single crisp frame.
	n := g.blurSamples()
	if n != 1 {
		t.Fatalf("still camera: blurSamples = %d, want 1", n)
	}

	// A small turn needs a few samples; a large turn is capped.
	g.prevAngle = 1
	small := g.blurSamples()
	g.prevAngle = 90
	large := g.blurSamples()
	if small < 1 || small >= large {
		t.Fatalf("expected small(%d) < large(%d) turn sample counts", small, large)
	}
	if large > maxBlurSamples {
		t.Fatalf("blurSamples = %d exceeds cap %d", large, maxBlurSamples)
	}
}

func TestLerp(t *testing.T) {
	got := lerp(10, 20, 0.25)
	if math.Abs(got-12.5) > 1e-9 {
		t.Fatalf("lerp = %v, want 12.5", got)
	}
}

func TestNewInitialState(t *testing.T) {
	l := level.New()
	l.PlayerStart = level.Start{X: 50, Y: 60, Angle: -90}
	g := New(asset.New(), l, "", false)
	if g.x != 50 || g.y != 60 || g.angle != -90 {
		t.Fatalf("ship not placed at start: %+v", g)
	}
}

func TestHeadingDirsForwardAndRight(t *testing.T) {
	// Facing up (-90, y-down): forward points to -Y, right points to +X.
	g := &Game{angle: -90}
	fx, fy, rx, ry := g.headingDirs()
	if math.Abs(fx) > 1e-9 || math.Abs(fy+1) > 1e-9 {
		t.Fatalf("forward = (%v,%v), want (0,-1)", fx, fy)
	}
	if math.Abs(rx-1) > 1e-9 || math.Abs(ry) > 1e-9 {
		t.Fatalf("right = (%v,%v), want (1,0)", rx, ry)
	}
	// Forward and right are perpendicular unit vectors.
	if math.Abs(fx*rx+fy*ry) > 1e-9 {
		t.Fatal("forward and right should be perpendicular")
	}
}
