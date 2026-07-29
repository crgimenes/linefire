package game

import (
	"math"

	"github.com/crgimenes/linefire/weapon"
)

// The weapon model and the catalogue live in the weapon package, shared with
// Linefire Skirmish: a missile has to hit as hard and reload as slowly there as
// it does here. What stays in this file is the firing — how a shot is spawned,
// aimed and resolved in a cave with walls.

// weaponSlot is a live weapon in one of the two arsenal slots: the weapon plus its
// cooldown. An empty slot (filled=false) never fires. See arsenal.go.
type weaponSlot struct {
	w      weapon.Weapon
	cd     int
	filled bool
}

// useWeapon dispatches a weapon by kind and reports whether it actually fired, so
// the cooldown only starts on a real use (not when the mouse aim is degenerate or
// the mine cap is reached). aim is the firing direction style (a mine ignores it).
func (g *Game) useWeapon(w *weapon.Weapon, aim weapon.Aim) bool {
	switch w.Kind {
	case weapon.KindProjectile:
		return g.fireProjectileWeapon(w, aim)
	case weapon.KindMine:
		return g.deployMineWeapon(w)
	case weapon.KindLaser:
		return g.fireLaserTick(w)
	case weapon.KindDevourer:
		return g.deployDevourerWeapon(w)
	}
	return false
}

// fireProjectileWeapon spawns a projectile carrying the weapon's payload and look,
// fired forward (from the muzzle) or toward the cursor/target (from the ship center)
// per aim. The ship velocity is added so shots track the player's drift.
func (g *Game) fireProjectileWeapon(w *weapon.Weapon, aim weapon.Aim) bool {
	var ox, oy, dx, dy float64
	switch aim {
	case weapon.AimCursor:
		adx, ady, ok := g.weaponAimDir()
		if !ok {
			return false
		}
		ox, oy, dx, dy = g.x, g.y, adx, ady
	default: // weapon.AimForward
		ox, oy = g.muzzleWorld()
		rad := g.angle * math.Pi / 180
		dx, dy = math.Cos(rad), math.Sin(rad)
		g.muzzleFlash = muzzleFlashFrames
	}
	// The fire-power upgrade fans a direct shot into several projectiles; an AoE
	// weapon (a missile) still fires one, so its blast is not multiplied.
	shots := 1
	if w.AOE == 0 {
		shots = shotsForLevel(g.fireLevel)
	}
	base := math.Atan2(dy, dx)
	for i := range shots {
		a := base + spreadOffset(i, shots)
		sdx, sdy := math.Cos(a), math.Sin(a)
		g.projectiles = append(g.projectiles, projectile{
			x: ox, y: oy, px: ox, py: oy,
			vx:    sdx*w.Speed + g.vx,
			vy:    sdy*w.Speed + g.vy,
			life:  w.Life,
			dmg:   damageForLevel(w.Damage, g.damageLevel),
			col:   w.Col,
			aoe:   w.AOE,
			seek:  seekTurnForLevel(g.seekLevel),
			rcol:  w.Core,
			rglow: w.Glow,
			width: w.Width,
			glowW: w.GlowWidth,
		})
	}
	g.sfx.play(soundOf(w.Fire))
	return true
}

// deployMineWeapon drops a mine carrying the weapon's payload, if below its cap.
func (g *Game) deployMineWeapon(w *weapon.Weapon) bool {
	if len(g.mines) >= w.MaxLive {
		return false
	}
	g.mines = append(g.mines, mine{
		x: g.x, y: g.y, arm: w.ArmFrames,
		dmg: damageForLevel(w.Damage, g.damageLevel), radius: w.AOE, trigger: w.TriggerRadius, col: w.Col,
	})
	g.sfx.play(soundOf(w.Fire))
	return true
}
