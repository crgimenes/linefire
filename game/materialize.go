package game

import (
	"image/color"
	"math"
)

// When the ship arrives somewhere — through a portal, a return warp, or by digging past an edge —
// it MATERIALIZES: particles rush inward from a ring and coalesce at the centre (the reverse of the
// death explosion), with a shimmering sound. It marks "the ship appeared here" instead of the ship
// just popping into existence.

const (
	materializeCount  = 52    // inbound particles (exaggerated, from far out)
	materializeRadius = 260.0 // ring they spawn on, world units — big, so the convergence reads clearly
	materializeLife   = 30    // frames to fall to the centre (~0.5s)
	materializeSize   = 1.8
)

var (
	materializeColor = color.RGBA{0x90, 0xe0, 0xff, 0xff} // teleport cyan-white
	materializeSound = soundReq{"arrival_low", 400, 0.5}  // a soft, low "flush" — short, not shrill
)

// materializeShip plays the arrival effect at the ship's current position. Silent/effectless in the
// attract demo so it does not intrude on the credits.
func (g *Game) materializeShip() {
	if g.creditsMode {
		return
	}
	g.emitMaterialize(g.x, g.y)
	g.sfx.play(materializeSound)
}

// emitMaterialize throws a ring of particles that converge on (x,y): each starts on the rim and
// travels straight in at a speed that lands it at the centre as its life ends (no drag), so the
// stream reads as matter rushing in to form the ship.
func (g *Game) emitMaterialize(x, y float64) {
	for range materializeCount {
		ang := randFloat() * 2 * math.Pi
		r := materializeRadius * (0.8 + 0.4*randFloat())
		px, py := x+r*math.Cos(ang), y+r*math.Sin(ang)
		sp := r / float64(materializeLife) // reaches the centre exactly at end of life
		g.emit(particle{
			x: px, y: py, px: px, py: py,
			vx:      -math.Cos(ang) * sp,
			vy:      -math.Sin(ang) * sp,
			drag:    1.0,
			life:    materializeLife,
			maxLife: materializeLife,
			size:    materializeSize,
			col:     materializeColor,
			style:   styleStreak,
		})
	}
}
