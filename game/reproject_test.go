package game

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestReprojectionMatchesRerender proves the motion-blur optimization is exact: a
// sub-frame produced by reprojecting the single padded frame lands every world
// point exactly where re-rendering at that camera pose would have. This is what
// lets one render + N blits replace N full renders.
func TestReprojectionMatchesRerender(t *testing.T) {
	g := &Game{sw: 900, sh: 700, dpr: 2}
	g.shakeX, g.shakeY = 5, -3 // shake must cancel out of the reprojection
	g.prevX, g.prevY, g.prevAngle = 100, 50, 10
	g.x, g.y, g.angle = 130, 60, 25 // moved and turned this frame

	pad := float64(g.frameMargin())

	// The camera the frame was rendered with (current pose, padded), and the plain
	// on-screen camera used to build the reprojection.
	g.camPad = pad
	frameCam := g.cameraGeoM()
	g.camPad = 0
	camFinal := g.cameraGeoM()
	inv := camFinal
	inv.Invert()

	points := [][2]float64{{100, 50}, {200, -30}, {0, 0}, {130, 60}, {-140, 220}}
	for _, tt := range []float64{0.06, 0.5, 0.94} {
		camS := g.cameraGeoMAt(
			lerp(g.prevX, g.x, tt), lerp(g.prevY, g.y, tt), lerp(g.prevAngle, g.angle, tt))

		reproj := ebiten.GeoM{}
		reproj.Translate(-pad, -pad)
		reproj.Concat(inv)
		reproj.Concat(camS)

		for _, p := range points {
			fx, fy := frameCam.Apply(p[0], p[1]) // where p lands in the frame
			rx, ry := reproj.Apply(fx, fy)       // reprojected onto the screen
			sx, sy := camS.Apply(p[0], p[1])     // where a re-render would place it
			if math.Abs(rx-sx) > 1e-6 || math.Abs(ry-sy) > 1e-6 {
				t.Fatalf("t=%.2f p=%v: reproject (%.4f,%.4f) != rerender (%.4f,%.4f)", tt, p, rx, ry, sx, sy)
			}
		}
	}
}

// TestFrameMarginCoversTurn checks the padding is at least the worst-case sweep of
// a screen corner, so no reprojected sub-frame can uncover an unrendered edge.
func TestFrameMarginCoversTurn(t *testing.T) {
	g := &Game{sw: 1600, sh: 1200, dpr: 2}
	radius := 0.5 * math.Hypot(float64(g.sw), float64(g.sh))
	sweep := radius * turnSpeed * math.Pi / 180 // a corner's travel during a max turn
	if float64(g.frameMargin()) < sweep {
		t.Fatalf("frame margin %d must cover the corner sweep %.1f", g.frameMargin(), sweep)
	}
}
