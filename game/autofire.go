package game

import (
	"math"

	"github.com/crgimenes/linefire/weapon"
)

// The combat computer (auto-fire) fires whichever equipped weapons are mouse-aimed
// (turret/missile/laser) at the nearest visible enemy, on their own cooldowns. A
// forward gun or a mine in a slot stays manual.

// autoFireRange caps how far the combat computer engages, in world units. Line of
// sight alone let it snipe enemies before they ever reached the screen — the kill
// must happen where the player can watch it. The default view shows ~667 units of
// half-width, so 520 keeps every computer kill comfortably in frame (and enemy
// snipers, at 576, still get to shoot first).
const autoFireRange = 520.0

// nearestEnemyDir returns the unit direction from the ship to the nearest enemy in
// line of sight within the computer's engagement range, if any.
func (g *Game) nearestEnemyDir() (float64, float64, bool) {
	bx, by, ok := g.nearestEnemyFrom(g.x, g.y, autoFireRange)
	if !ok {
		return 0, 0, false
	}
	dx, dy := bx-g.x, by-g.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return 0, 0, false
	}
	return dx / d, dy / d, true
}

// updateAutoTarget picks the auto-fire target direction for this frame (the nearest
// visible enemy), so every aimed weapon — manual or computer-fired — points at it.
func (g *Game) updateAutoTarget() {
	g.autoAimOK = false
	if !g.autoFire {
		return
	}
	tx, ty, ok := g.nearestEnemyDir()
	if ok {
		g.autoAimX, g.autoAimY, g.autoAimOK = tx, ty, true
	}
}

// runAutoFire fires every equipped mouse-aimed weapon at the auto target. Slot cooldowns
// gate the rate, so it is safe to call every frame.
func (g *Game) runAutoFire() {
	if !g.autoFire || !g.autoAimOK {
		return
	}
	// The computer assists the SECONDARY slot only — it is the mouse-aimed one, so the
	// computer can point it at the target. The forward primary stays under manual control,
	// and a mine is never auto-deployed.
	const secondary = 1
	k := g.slots[secondary].w.Kind
	if g.slots[secondary].filled && k != weapon.KindMine && k != weapon.KindDevourer {
		g.fireWeaponSlot(secondary)
	}
}

// weaponAimDir is the direction aimed weapons fire in: the auto-fire target when the
// combat computer is engaged, else the mouse cursor.
func (g *Game) weaponAimDir() (float64, float64, bool) {
	if g.autoFire && g.autoAimOK {
		return g.autoAimX, g.autoAimY, true
	}
	return g.aimDir()
}
