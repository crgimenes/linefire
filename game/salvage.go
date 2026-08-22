package game

import "slices"

import "math"

// Salvage: in skirmish there is no player to collect what a kill leaves on the
// field, so the SHIPS do. That turns loot into the thing worth fighting over —
// a damaged fleet has somewhere to go, a winning one can be denied its
// resupply, and a program that watches the field can decide between a kill and
// a canister. It is the same drop table the campaign uses (loot.Table) and the
// same pickups drawn the same way; what is new is who reaches them.
//
// What a ship can take, and what it does to that hull:
//
//	heal    repairs hull, never past what the archetype was built with
//	shield  a pool that absorbs damage before the hull does
//	damage  every bolt it fires hits harder
//	rate    it fires more often
//	fire    it fires more bolts at once, in a narrow fan
//
// Weapons are salvaged too, and what a hull then does with one lives in
// shiparms.go: the aimed ones are fired at a point, the dropped ones are laid
// where the ship stands. The only thing on the field a hull will not stoop for
// is the front gun, which is what it already fires.
const (
	shipHealAmount   = 2 // hull points a repair restores
	shipShieldAmount = 3 // shield points a canister grants
	shipMaxShield    = 6 // a ship can hold two canisters' worth
	shipMaxMod       = 3 // how far one combat mod can stack on a single hull

	shipRateStep    = 12 // frames a rate mod cuts from the fire interval
	shipMinInterval = 18 // however many mods it holds, a hull needs this long between volleys
	shipFanDegrees  = 7  // angle between the bolts of a fanned volley

	// The field keeps producing: a battle where the only loot came from kills
	// would starve exactly the fleet that is losing and needs it most.
	salvageDropEvery = 420 // ticks between spontaneous drops (7s)
	salvageDropInset = 120 // world units kept clear of the walls
	salvageDropTries = 8
	// What the field may hold UNCOLLECTED at once. The drip has no other
	// brake — nothing removes a canister nobody flew over — so a long battle
	// silted up until the arena was a carpet of pickups (crg watched a decided
	// field fill with them). At the cap the field simply stops producing until
	// somebody collects: scarcity is what made loot worth crossing the map for.
	salvageMaxLoose = 12
)

// salvageDrops is what the field spontaneously produces, in the proportion the
// campaign's own table favours: mostly repairs, sometimes an edge — and, rarely,
// a weapon. The laser and the devourer are the rarest things out there, and
// equally rare on purpose: one is the prize and the other is the trap, and a
// trap has to be as much of a surprise as a prize or nobody ever falls for it.
// A fleet that reaches either changes the battle — one of them in the direction
// it was hoping for.
var salvageDrops = []string{
	"powerup", "powerup", "powerup", "powerup", "powerup", "powerup",
	"shield", "shield", "shield", "shield",
	"firepower", "firepower",
	"ratepower", "ratepower",
	"damagepower", "damagepower",
	"wpn_missile", "wpn_missile",
	"wpn_mine", "wpn_mine",
	"wpn_laser",
	"wpn_devourer",
}

// looseSalvage counts what is lying on the field waiting to be collected.
func (g *Game) looseSalvage() int {
	n := 0
	for i := range g.entities {
		if k := g.entities[i].kind; k == kindPowerUp || k == kindWeapon {
			n++
		}
	}
	return n
}

// resolveSalvage hands every pickup a ship is touching to that ship. Pickups
// are removed after the whole pass, so no index shifts under it.
func (g *Game) resolveSalvage() {
	if !g.skirmishMode {
		return
	}
	var taken []int
	for i := range g.entities {
		switch g.entities[i].kind {
		case kindPowerUp:
		case kindWeapon:
			if !shipCanCarry(g.entities[i].power) {
				continue // nothing a hull can fire: it stays where it fell
			}
		default:
			continue
		}
		j := g.shipReaching(i)
		if j < 0 {
			continue
		}
		g.salvage(j, i)
		taken = append(taken, i)
	}
	for _, i := range slices.Backward(taken) {

		g.entities = append(g.entities[:i], g.entities[i+1:]...)
	}
}

// shipReaching returns the index of the live ship overlapping the pickup at
// index pi, or -1. The nearest one wins a contested canister, which is what
// makes racing for it worth a program's while.
func (g *Game) shipReaching(pi int) int {
	p := &g.entities[pi]
	best, bestDist := -1, math.Inf(1)
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 {
			continue
		}
		d := math.Hypot(e.x-p.x, e.y-p.y)
		if d > e.radius+p.radius || d >= bestDist {
			continue
		}
		best, bestDist = i, d
	}
	return best
}

// salvage applies the pickup at index pi to the ship at index si.
func (g *Game) salvage(si, pi int) {
	e, p := &g.entities[si], &g.entities[pi]
	if p.kind == kindWeapon {
		e.weaponKey = p.power // a fresh weapon replaces whatever it was carrying
		g.match.recordSalvage(e.faction)
		g.emitBurst(p.x, p.y, pickupMotes)
		g.playEvent(p.a, "pickup")
		g.traceSalvage(e, p.power)
		return
	}
	switch p.power {
	case powerShield:
		e.shield = min(e.shield+shipShieldAmount, shipMaxShield)
	case powerDamage:
		e.dmgMod = min(e.dmgMod+1, shipMaxMod)
	case powerRate:
		e.rateMod = min(e.rateMod+1, shipMaxMod)
	case powerFire:
		e.fireMod = min(e.fireMod+1, shipMaxMod)
	default: // heal, and anything a ship has no other use for
		e.hp = min(e.hp+shipHealAmount, e.hullMax())
	}
	g.match.recordSalvage(e.faction)
	g.emitBurst(p.x, p.y, pickupMotes)
	g.playEvent(p.a, "pickup")
	g.traceSalvage(e, p.power)
}

// hullMax is what this hull was built with: a repair restores toward it and
// never past it.
func (e *entity) hullMax() int {
	if e.hpMax > 0 {
		return e.hpMax
	}
	return enemyHP
}

// fireInterval is how long this hull waits between volleys, shortened by every
// rate mod it has salvaged and floored so no fleet can machine-gun.
func (e *entity) fireInterval() int {
	eff := e.fireEvery
	if eff <= 0 {
		eff = enemyFireInterval
	}
	return max(eff-e.rateMod*shipRateStep, shipMinInterval)
}

// stepSalvageDrops sprinkles the battlefield with fresh loot on a timer.
func (g *Game) stepSalvageDrops() {
	if !g.skirmishMode || g.horde == nil {
		return
	}
	g.salvageCD--
	if g.salvageCD > 0 {
		return
	}
	g.salvageCD = salvageDropEvery
	if g.looseSalvage() >= salvageMaxLoose {
		return // the field is already littered; let the fleets clear some
	}

	minX, minY := g.bounds.minX+salvageDropInset, g.bounds.minY+salvageDropInset
	spanX := max(g.bounds.maxX-salvageDropInset-minX, 1)
	spanY := max(g.bounds.maxY-salvageDropInset-minY, 1)
	for range salvageDropTries {
		x := minX + g.simFloat()*spanX
		y := minY + g.simFloat()*spanY
		if !g.hordeReachable(x, y, salvageDropInset/2) {
			continue // inside rock, or in a pocket nothing can reach
		}
		g.spawnReward(salvageDrops[g.simIntN(len(salvageDrops))], x, y)
		return
	}
}
