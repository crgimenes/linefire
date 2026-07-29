package effects

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/render"
)

const (
	portalMotes     = 72   // particles converging on the vortex
	portalSpeed     = 0.6  // how fast a mote travels rim -> centre (cycles per second)
	portalRingWidth = 1.4  // ring stroke, logical px
	portalMoteSize  = 1.6  // mote radius, world units
	portalRingPulse = 3.0  // ring brightness cycles per second
	portalMoteAlpha = 235  // peak mote alpha, half way down the spoke
	portalRingBase  = 0x70 // ring alpha floor
)

// Portal is one frame of a vortex, in screen space.
type Portal struct {
	X, Y   float64 // centre, screen pixels
	Radius float64 // rim, screen pixels
	Col    color.RGBA

	// Seconds is the animation clock. It only has to advance smoothly: the motes
	// are a function of it, so nothing is stored between frames.
	Seconds float64

	// Fade scales the whole thing out, 1 for a portal at full strength. A
	// permanent portal leaves it at 1; a transient one — a ship arriving — rides
	// it down as the vortex closes.
	Fade float64

	DPR   float64 // device pixels per logical px, for the ring stroke
	Scale float64 // screen pixels per world unit, for the mote size
}

// DrawPortal renders a portal as a tunnel: a thin outer ring, and motes born ON
// that ring travelling STRAIGHT to the centre, where they die. The radial
// convergence — no spiral, no inner rings — is what reads as looking down a
// tunnel. Each mote takes a fresh random spoke every cycle, so the stream
// scatters instead of tracing a few thick arms.
//
// Nothing is stored: the whole vortex is a function of Seconds, so a caller can
// draw one wherever it likes without owning any state.
func DrawPortal(dst *ebiten.Image, p Portal) {
	fade := p.Fade
	if fade == 0 {
		fade = 1
	}
	if fade <= 0 || p.Radius <= 0 {
		return
	}

	cx, cy := float32(p.X), float32(p.Y)
	ring := p.Col
	ring.A = scaleAlpha(portalRingBase+0x80*(0.5+0.5*math.Sin(p.Seconds*portalRingPulse)), fade)
	vector.StrokeCircle(dst, cx, cy, float32(p.Radius), float32(portalRingWidth*p.DPR), ring, true)

	for i := range portalMotes {
		phase := p.Seconds*portalSpeed + float64(i)/portalMotes
		cycle := math.Floor(phase)
		u := 1 - (phase - cycle) // 1 at the rim, 0 at the centre
		angle := SpokeAngle(i, int(cycle))
		r := p.Radius * u
		mote := p.Col
		mote.A = scaleAlpha(portalMoteAlpha*math.Sin(math.Pi*u), fade) // fade in at the rim, out at the centre
		render.FillCircle(dst,
			p.X+r*math.Cos(angle),
			p.Y+r*math.Sin(angle),
			portalMoteSize*p.Scale, mote)
	}
}

// SpokeAngle is a stable pseudo-random spoke for mote i on cycle c: constant
// while the mote falls inward, fresh when it respawns, so the stream never
// repeats a fixed arm.
func SpokeAngle(i, c int) float64 {
	h := uint32(i)*73856093 ^ uint32(c)*19349663 // #nosec G115 -- small non-negative indices; the hash mixing is intentional
	h ^= h >> 13
	h *= 0x85ebca6b
	h ^= h >> 16
	return float64(h) / float64(1<<32) * 2 * math.Pi
}

// scaleAlpha clamps a computed alpha into a byte.
func scaleAlpha(a, fade float64) uint8 {
	return uint8(max(min(a*fade, 255), 0))
}
