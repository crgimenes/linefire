package game

import "math"

// Combat mods are the bullet-hell power-ups that STACK over a run: each pickup
// nudges a combat stat up — no menu, no slot, just fly over it (like the drop-swap
// and the combat computer). This file holds the family: fire power (multishot fan),
// fire rate (shorter cooldowns) and damage (stronger shots); drones and shield
// variants join it under the same pattern later. State lives on the Game, preserved
// across maps (enterMap) and reset on restart (until checkpoints land — see T24).

const (
	maxFireLevel   = 4                 // fire-power pickups past this are wasted (capped)
	maxFireShots   = 1 + maxFireLevel  // projectiles in the widest fan
	fireSpreadStep = 7 * math.Pi / 180 // radians between adjacent fan projectiles

	maxRateLevel   = 4    // fire-rate pickups past this are wasted
	maxDamageLevel = 4    // damage pickups past this are wasted
	rateFactor     = 0.82 // cooldown multiplier per rate level (<1 = faster fire)
	minCooldown    = 3    // frame floor, so rapid fire never collapses into a solid stream
	damageStep     = 0.25 // damage added per level, as a fraction of the base

	maxSeekLevel = 4     // homing pickups past this are wasted
	seekTurnStep = 0.045 // homing turn rate added per level, radians/frame
	homingRange  = 520.0 // a homing shot only chases an enemy within this range
)

// seekTurnForLevel is the homing turn rate (radians/frame) a shot gets at the given
// seek level: 0 (straight) until the first pickup, then steeper each level.
func seekTurnForLevel(level int) float64 {
	if level <= 0 {
		return 0
	}
	return float64(level) * seekTurnStep
}

// steerToward rotates velocity (vx,vy) toward direction (dx,dy) by at most maxTurn
// radians, preserving speed — the per-frame homing turn.
func steerToward(vx, vy, dx, dy, maxTurn float64) (float64, float64) {
	speed := math.Hypot(vx, vy)
	if speed == 0 || (dx == 0 && dy == 0) {
		return vx, vy
	}
	cur := math.Atan2(vy, vx)
	want := math.Atan2(dy, dx)
	diff := math.Atan2(math.Sin(want-cur), math.Cos(want-cur)) // shortest signed turn
	if diff > maxTurn {
		diff = maxTurn
	}
	if diff < -maxTurn {
		diff = -maxTurn
	}
	a := cur + diff
	return math.Cos(a) * speed, math.Sin(a) * speed
}

// cooldownForLevel scales a weapon's base cooldown down by the fire-rate level (each
// level fires faster), floored so it never drops to every-frame — and never rises
// above the base, even if the base is already under the floor.
func cooldownForLevel(base, level int) int {
	if level <= 0 {
		return base
	}
	floor := min(base, minCooldown)
	cd := int(math.Round(float64(base) * math.Pow(rateFactor, float64(level))))
	if cd < floor {
		return floor
	}
	return cd
}

// damageForLevel scales a shot's base damage up by the damage level (each level adds
// damageStep of the base), never below the base.
func damageForLevel(base, level int) int {
	if level <= 0 {
		return base
	}
	d := int(math.Round(float64(base) * (1 + damageStep*float64(level))))
	if d < base {
		return base
	}
	return d
}

// shotsForLevel is how many projectiles a direct weapon fires at a fire-power level
// (1 at level 0, growing to the cap).
func shotsForLevel(level int) int {
	s := 1 + level
	if s < 1 {
		return 1
	}
	if s > maxFireShots {
		return maxFireShots
	}
	return s
}

// spreadOffset is the angular offset (radians) of fan projectile i of shots,
// centered on the aim direction (an odd count keeps one shot dead straight).
func spreadOffset(i, shots int) float64 {
	if shots <= 1 {
		return 0
	}
	return (float64(i) - float64(shots-1)/2) * fireSpreadStep
}

// addFirePower raises the fire-power level (multishot) up to the cap, narrating it.
// Reports whether it actually rose, so a capped pickup does not over-promise.
func (g *Game) addFirePower() bool {
	if g.fireLevel >= maxFireLevel {
		g.logf("FIRE POWER  maxed")
		return false
	}
	g.fireLevel++
	g.logf("FIRE POWER  %d  (%d shots)", g.fireLevel, shotsForLevel(g.fireLevel))
	return true
}

// addRatePower raises the fire-rate level (shorter cooldowns) up to the cap,
// narrating it. Reports whether it actually rose.
func (g *Game) addRatePower() bool {
	if g.rateLevel >= maxRateLevel {
		g.logf("FIRE RATE  maxed")
		return false
	}
	g.rateLevel++
	g.logf("FIRE RATE  %d", g.rateLevel)
	return true
}

// addDamagePower raises the damage level (stronger shots) up to the cap, narrating
// it. Reports whether it actually rose.
func (g *Game) addDamagePower() bool {
	if g.damageLevel >= maxDamageLevel {
		g.logf("DAMAGE  maxed")
		return false
	}
	g.damageLevel++
	g.logf("DAMAGE  %d", g.damageLevel)
	return true
}

// addSeek raises the homing level (shots curve toward enemies) up to the cap.
func (g *Game) addSeek() bool {
	if g.seekLevel >= maxSeekLevel {
		g.logf("HOMING  maxed")
		return false
	}
	g.seekLevel++
	g.logf("HOMING  %d", g.seekLevel)
	return true
}
