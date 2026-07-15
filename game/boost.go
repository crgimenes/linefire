package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// The booster is a held afterburner (Left Shift): it adds forward thrust and lifts the speed cap
// for a burst, draining a SHORT fuel bar so it cannot be cruised on — it recharges when released.
// It is the deliberate escape from the DEVOURER's pull, and it throws a hot jet flame.

const (
	boostMax      = 84.0 // fuel capacity (~1.4s of boost at full drain)
	boostDrain    = 1.4  // fuel spent per boosting frame
	boostRegen    = 0.28 // fuel recovered per non-boosting frame (~5s to refill from empty)
	boostThrust   = 0.34 // extra forward acceleration while boosting (on top of normal thrust)
	boostSpeedMul = 1.75 // while boosting the speed cap is raised to maxSpeed*this
	// boostRearmFrac gates re-use: once emptied, the booster stays LOCKED until the fuel
	// recharges past this fraction of full, so it is a periodic burst, not a tap on fumes. At a
	// low value the sputter as it re-arms reads as a misfiring engine — a happy accident kept on.
	boostRearmFrac = 0.10

	boostFlameCount    = 4
	boostFlameSpeedMin = 3.0
	boostFlameSpeedMax = 6.5
	boostFlameSpread   = 0.14
	boostFlameLifeMin  = 10
	boostFlameLifeMax  = 20
	boostFlameSize     = 1.7
)

// boostFlameColor is the afterburner jet: a hot orange-white streak.
var boostFlameColor = color.RGBA{0xff, 0xc0, 0x50, 0xff}

// hudBoostFill is the booster gauge fill (afterburner amber); hudBoostCharging is the dim fill
// while it is locked out, recharging past the re-arm threshold.
var (
	hudBoostFill     = color.RGBA{0xff, 0xa0, 0x30, 0xff}
	hudBoostCharging = color.RGBA{0x70, 0x50, 0x28, 0xff}
)

// updateBoost runs the afterburner for this frame from the Shift key. While held with fuel it adds
// forward thrust, burns fuel and throws a flame; otherwise the fuel recharges. fx,fy is the ship's
// forward unit vector. Returns whether it boosted, so the caller counts it as thrust for the engine
// loop. g.boosting is left set for clampSpeed to read.
func (g *Game) updateBoost(fx, fy float64) bool {
	// Re-arm gate: empty locks the booster; it only frees up again after recharging past the
	// threshold, so you cannot ride it on a trickle of fuel.
	switch {
	case g.boostFuel <= 0:
		g.boostArmed = false
	case g.boostFuel >= boostRearmFrac*boostMax:
		g.boostArmed = true
	}
	g.boosting = g.boostArmed && g.boostFuel > 0 && ebiten.IsKeyPressed(ebiten.KeyShiftLeft)
	if g.boosting {
		g.boostFuel = math.Max(g.boostFuel-boostDrain, 0)
		g.vx += fx * boostThrust
		g.vy += fy * boostThrust
		g.emitBoostFlame(fx, fy)
		return true
	}
	g.boostFuel = math.Min(g.boostFuel+boostRegen, boostMax)
	return false
}

// emitBoostFlame throws a hot jet out the ship's tail — bigger and faster than the idle thruster,
// so the afterburner reads at a glance.
func (g *Game) emitBoostFlame(fx, fy float64) {
	rearX, rearY := g.x-fx*g.radius, g.y-fy*g.radius
	px, py := -fy, fx
	for range boostFlameCount {
		speed := boostFlameSpeedMin + randFloat()*(boostFlameSpeedMax-boostFlameSpeedMin)
		fan := (randFloat()*2 - 1) * boostFlameSpread * speed
		life := boostFlameLifeMin + randIntN(boostFlameLifeMax-boostFlameLifeMin+1)
		g.emit(particle{
			x: rearX, y: rearY,
			vx:      -fx*speed + px*fan,
			vy:      -fy*speed + py*fan,
			drag:    0.90,
			life:    life,
			maxLife: life,
			size:    boostFlameSize,
			col:     boostFlameColor,
			style:   styleStreak,
		})
	}
}
