package game

import "github.com/crgimenes/linefire/weapon"

// Power-up effect kinds (the level Spawn.Kind value).
const (
	powerHeal     = "heal"
	powerScore    = "score"
	powerShield   = "shield"
	powerComputer = "computer" // grants the combat computer (auto-fire, G toggles)
	powerFire     = "fire"     // raises fire power: direct weapons fan more shots
	powerRate     = "rate"     // raises fire rate: shorter weapon cooldowns
	powerDamage   = "damage"   // raises damage: stronger shots
	powerAlly     = "ally"     // adds a formation escort (friendly auto-firing ship)
	powerDrone    = "drone"    // adds an orbiting drone (friendly auto-firing satellite)
	powerBubble   = "bubble"   // grants timed full damage immunity (bubble shield)
	powerReflect  = "reflect"  // grants timed shot reflection (reflector shield)
	powerSeek     = "seek"     // raises homing: direct shots curve toward enemies
)

const (
	healAmount   = 40 // health restored by a heal power-up
	scoreBonus   = 5  // score gained by a score power-up
	shieldAmount = 60 // shield points granted by a shield power-up
	maxShield    = 100
	pickupSparks = 12 // particles in the pickup burst
)

// resolvePickups collects any power-up the player is touching, applying its
// effect, spawning a burst and removing it.
func (g *Game) resolvePickups() {
	if g.skirmishMode || g.over {
		return // no player: loot lies where it fell until a ship can want it (Filo, later)
	}
	kept := g.entities[:0]
	for i := range g.entities {
		e := g.entities[i]
		if e.kind == kindPowerUp {
			rr := g.radius + e.radius
			dx, dy := g.x-e.x, g.y-e.y
			if dx*dx+dy*dy <= rr*rr {
				g.applyPickup(&e)
				g.emitBurst(e.x, e.y, pickupMotes)
				g.markConsumed(e.spawn) // stays collected if the player revisits this map
				continue                // consumed: drop it
			}
		}
		kept = append(kept, e)
	}
	g.entities = kept
}

// applyPickup applies a power-up's effect to the player.
func (g *Game) applyPickup(e *entity) {
	g.playEvent(e.a, "pickup")
	// Every power-up carries a charge: whatever else it does, collecting one is
	// how the weapons get their energy back. That is what makes a pickup worth a
	// detour once the heavy guns have been leaning on the pool.
	g.energyPool().Add(weapon.PickupEnergy)
	switch e.power {
	case powerComputer:
		g.hasComputer = true
		g.autoFire = true // comes online firing; G toggles from here on
		g.logf("PICKUP  combat computer online (G toggles)")
	case powerFire:
		g.addFirePower() // multishot: stacks over the run
	case powerRate:
		g.addRatePower() // shorter cooldowns: stacks over the run
	case powerDamage:
		g.addDamagePower() // stronger shots: stacks over the run
	case powerAlly:
		g.addAlly(modeEscort) // a formation escort joins the run
	case powerDrone:
		g.addAlly(modeOrbit) // an orbiting drone joins the run
	case powerBubble:
		g.addBubble() // timed damage immunity
	case powerReflect:
		g.addReflect() // timed shot reflection
	case powerSeek:
		g.addSeek() // homing shots: stacks over the run
	case powerScore:
		g.score += scoreBonus
		g.logf("PICKUP  score +%d", scoreBonus)
	case powerShield:
		g.shield += shieldAmount
		if g.shield > maxShield {
			g.shield = maxShield
		}
		g.logf("PICKUP  shield +%d", shieldAmount)
	default: // heal
		g.health += healAmount
		if g.health > maxHealth {
			g.health = maxHealth
		}
		g.logf("PICKUP  heal +%d", healAmount)
	}
}
