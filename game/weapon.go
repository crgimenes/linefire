package game

import (
	"image/color"
	"math"
)

// weaponKind is how a weapon delivers damage.
type weaponKind int

const (
	wkProjectile weaponKind = iota // a bullet or missile (aoe>0 makes it explode)
	wkMine                         // a deployed proximity mine
	wkLaser                        // a continuous beam that pierces every enemy it crosses
	wkDevourer                     // a deployed black hole: arms, then drags everything in and collapses
)

// aimMode is how a weapon is aimed when used.
type aimMode int

const (
	aimForward aimMode = iota // along the ship heading, from the gun muzzle
	aimMouse                  // toward the cursor, from the ship center
	aimDrop                   // no direction; placed at the ship (mines)
)

// weapon is a data-driven weapon definition. The same struct describes the gun, the
// missile and the mine; a new weapon (laser, etc.) is new data plus a case in
// useWeapon, not a new fire function.
type weapon struct {
	name     string
	kind     weaponKind
	aim      aimMode
	cooldown int // frames between uses

	damage int        // hull damage (center damage for an AoE shot)
	col    color.RGBA // floating damage-number color

	fire soundReq // gion sound played when this weapon fires (empty = silent, e.g. the laser)

	// projectile params (wkProjectile)
	speed float64
	life  int
	aoe   float64    // explosion radius; 0 = direct hit
	rcol  color.RGBA // bullet core render color
	rglow color.RGBA // bullet glow render color
	width float64    // crisp stroke width, logical px
	glowW float64    // glow stroke width, logical px

	// mine params (wkMine)
	armFrames     int
	triggerRadius float64
	maxLive       int // cap on this weapon's live mines

	// laser params (wkLaser)
	reach float64 // max beam length, world units (clipped at the first wall)
}

// The default loadout's weapons reuse the existing per-weapon constants and
// colors, so extracting the model does not change how any weapon plays.
var (
	weaponFrontGun = weapon{
		name: "Front Gun", kind: wkProjectile, aim: aimForward, cooldown: fireInterval,
		damage: playerShotDamage, col: damageColor, fire: soundReq{"shot_soft", 101, 0.3}, // low, dark pu — bursty, kept quiet so it won't mask the music
		speed: bulletSpeed, life: bulletLife, rcol: bulletColor, rglow: bulletGlowColor, width: bulletWidth, glowW: bulletGlowWidth,
	}
	weaponMissile = weapon{
		name: "Missile", kind: wkProjectile, aim: aimMouse, cooldown: missileInterval,
		damage: missileDamage, col: missileDamageColor, aoe: missileRadius, fire: soundReq{"shot_launch", 130, 0.5}, // low rising launch whoosh
		speed: missileSpeed, life: missileLife, rcol: missileColor, rglow: missileGlowColor, width: missileWidth, glowW: missileGlowWidth,
	}
	weaponMine = weapon{
		name: "Mine", kind: wkMine, aim: aimDrop, cooldown: 16, // spaces out a held deploy
		damage: mineDamage, col: mineDamageColor, aoe: mineRadius, fire: soundReq{"shot_thunk", 140, 0.45}, // dull low deploy thunk
		armFrames: mineArmFrames, triggerRadius: mineTriggerRadius, maxLive: maxMines,
	}
	weaponLaser = weapon{
		name: "Laser", kind: wkLaser, aim: aimMouse, cooldown: laserTickEvery,
		damage: laserDamage, col: laserDamageColor, reach: laserRange,
		fire: soundReq{"beam", 150, 0.3}, // a loop recipe: hums while the beam is held
	}
	weaponDevourer = weapon{
		name: "Devourer", kind: wkDevourer, aim: aimDrop, cooldown: devourerCooldown,
		damage: devourerCollapseDmg, col: devourerColor,
		fire: soundReq{"shot_thunk", 70, 0.7}, // a deep, ominous drop
	}
)

// Catalog indices: which weapon TYPE, used to key the arsenal and pickups.
const (
	catFront = iota
	catMissile
	catMine
	catLaser
	catDevourer
)

// weaponCatalog is every weapon type the player can collect and mount. The turret is not
// a weapon — it is the slot-2 mount, and any weapon there aims (see slotAim).
var weaponCatalog = []weapon{weaponFrontGun, weaponMissile, weaponMine, weaponLaser, weaponDevourer}

// weaponSlot is a live weapon in one of the two arsenal slots: the weapon plus its
// cooldown. An empty slot (filled=false) never fires. See arsenal.go.
type weaponSlot struct {
	w      weapon
	cd     int
	filled bool
}

// useWeapon dispatches a weapon by kind and reports whether it actually fired, so
// the cooldown only starts on a real use (not when the mouse aim is degenerate or
// the mine cap is reached). aim is the firing direction style (a mine ignores it).
func (g *Game) useWeapon(w *weapon, aim aimMode) bool {
	switch w.kind {
	case wkProjectile:
		return g.fireProjectileWeapon(w, aim)
	case wkMine:
		return g.deployMineWeapon(w)
	case wkLaser:
		return g.fireLaserTick(w)
	case wkDevourer:
		return g.deployDevourerWeapon(w)
	}
	return false
}

// fireProjectileWeapon spawns a projectile carrying the weapon's payload and look,
// fired forward (from the muzzle) or toward the cursor/target (from the ship center)
// per aim. The ship velocity is added so shots track the player's drift.
func (g *Game) fireProjectileWeapon(w *weapon, aim aimMode) bool {
	var ox, oy, dx, dy float64
	switch aim {
	case aimMouse:
		adx, ady, ok := g.weaponAimDir()
		if !ok {
			return false
		}
		ox, oy, dx, dy = g.x, g.y, adx, ady
	default: // aimForward
		ox, oy = g.muzzleWorld()
		rad := g.angle * math.Pi / 180
		dx, dy = math.Cos(rad), math.Sin(rad)
		g.muzzleFlash = muzzleFlashFrames
	}
	// The fire-power upgrade fans a direct shot into several projectiles; an AoE
	// weapon (a missile) still fires one, so its blast is not multiplied.
	shots := 1
	if w.aoe == 0 {
		shots = shotsForLevel(g.fireLevel)
	}
	base := math.Atan2(dy, dx)
	for i := range shots {
		a := base + spreadOffset(i, shots)
		sdx, sdy := math.Cos(a), math.Sin(a)
		g.projectiles = append(g.projectiles, projectile{
			x: ox, y: oy, px: ox, py: oy,
			vx:    sdx*w.speed + g.vx,
			vy:    sdy*w.speed + g.vy,
			life:  w.life,
			dmg:   damageForLevel(w.damage, g.damageLevel),
			col:   w.col,
			aoe:   w.aoe,
			seek:  seekTurnForLevel(g.seekLevel),
			rcol:  w.rcol,
			rglow: w.rglow,
			width: w.width,
			glowW: w.glowW,
		})
	}
	g.sfx.play(w.fire)
	return true
}

// deployMineWeapon drops a mine carrying the weapon's payload, if below its cap.
func (g *Game) deployMineWeapon(w *weapon) bool {
	if len(g.mines) >= w.maxLive {
		return false
	}
	g.mines = append(g.mines, mine{
		x: g.x, y: g.y, arm: w.armFrames,
		dmg: damageForLevel(w.damage, g.damageLevel), radius: w.aoe, trigger: w.triggerRadius, col: w.col,
	})
	g.sfx.play(w.fire)
	return true
}
