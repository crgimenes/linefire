package game

import (
	"fmt"
	"slices"
)

// The arsenal is everything the player has collected, in pickup order — no mounts, no
// hangar, no drop-swap. Two slots each point at one collected weapon; slot 0 fires with
// the LEFT mouse, slot 1 with the RIGHT. Keys 1 and 2 cycle each slot through the whole
// arsenal, DOOM-style; when a slot would cycle onto the weapon the OTHER slot holds, the
// two trade mounts instead — so the two slots never mirror each other, cycling always
// changes the slot, and swapping the nose and turret weapons needs no extra key. Firing,
// cooldown and the laser's overheat come from the weapon; the AIM (forward vs cursor)
// comes from the SLOT (see slotAim), so the same weapon is a forward gun in slot 1 and an
// aimed one in slot 2.

// numSlots is how many weapons can be live at once (primary + secondary).
const numSlots = 2

// initArsenal launches the ship with the front gun live in slot 1 and slot 2 empty
// (armed once a second weapon is collected and cycled to).
func (g *Game) initArsenal() {
	g.arsenal = []int{catFront}
	g.slotArsIdx = [numSlots]int{0, -1}
	g.syncSlots()
}

// syncSlots rebuilds each slot's live weapon from its pointer into the arsenal.
func (g *Game) syncSlots() {
	for s := range g.slots {
		i := g.slotArsIdx[s]
		if i < 0 || i >= len(g.arsenal) {
			g.slots[s] = weaponSlot{}
			continue
		}
		g.slots[s] = weaponSlot{w: weaponCatalog[g.arsenal[i]], filled: true}
	}
}

// hasWeapon reports whether the arsenal already holds a catalog weapon.
func (g *Game) hasWeapon(cat int) bool {
	return slices.Contains(g.arsenal, cat)
}

// collectWeapon adds a weapon on pickup: a brand-new type joins the arsenal and arms the
// first EMPTY slot (grab-and-go) without displacing a slot the player already set up — so
// your first pickup becomes the secondary and the forward gun stays primary. Once both
// slots are full it just joins the collection, ready to be cycled in. A duplicate is
// ignored. Reports whether it was new.
func (g *Game) collectWeapon(cat int) bool {
	if cat < 0 || cat >= len(weaponCatalog) || g.hasWeapon(cat) {
		return false
	}
	g.arsenal = append(g.arsenal, cat)
	for s := range g.slots {
		if !g.slots[s].filled {
			g.slotArsIdx[s] = len(g.arsenal) - 1
			break
		}
	}
	g.syncSlots()
	return true
}

// cycleSlot advances a slot to the next collected weapon, wrapping. If that next weapon
// is the one mounted on the OTHER slot, the two trade mounts (the current weapon slides
// over there) instead of colliding — a nose<->turret swap folded into the same key, with
// no slot ever left empty or holding a duplicate. With a single weapon it is a no-op.
func (g *Game) cycleSlot(s int) {
	if s < 0 || s >= numSlots || len(g.arsenal) == 0 {
		return
	}
	n := len(g.arsenal)
	other := 1 - s // the two-slot pair
	idx := (g.slotArsIdx[s] + 1) % n
	if g.slots[other].filled && g.arsenal[idx] == g.arsenal[g.slotArsIdx[other]] {
		g.slotArsIdx[other] = g.slotArsIdx[s] // the sibling takes what this slot is leaving
	}
	g.slotArsIdx[s] = idx
	g.syncSlots()
	g.logf("SLOT %d  %s", s+1, weaponKeys[g.arsenal[idx]])
}

// tickWeapons ages both slots' cooldowns one frame.
func (g *Game) tickWeapons() {
	for s := range g.slots {
		if g.slots[s].cd > 0 {
			g.slots[s].cd--
		}
	}
}

// slotAim is the firing direction the SLOT imposes, not the weapon: the primary (slot 0)
// always shoots FORWARD, the secondary (slot 1) toward the cursor/target. So whatever you
// cycle into a slot behaves the same way — a laser in the primary is a forward beam, a
// front gun in the secondary is an aimed fan. A mine ignores aim regardless.
func slotAim(s int) aimMode {
	if s == 0 {
		return aimForward
	}
	return aimMouse
}

// fireWeaponSlot fires the weapon live in slot s (its button is held): the laser is
// aimed and gated on overheat every frame, everything else fires on the slot's cooldown.
func (g *Game) fireWeaponSlot(s int) {
	if s < 0 || s >= numSlots || !g.slots[s].filled {
		return
	}
	sl := &g.slots[s]
	aim := slotAim(s)
	if sl.w.kind == wkLaser {
		if g.laserHot {
			return // overheated: the beam stays off until it cools
		}
		g.aimLaser(&sl.w, aim)
	}
	if sl.cd > 0 {
		return
	}
	if g.useWeapon(&sl.w, aim) {
		sl.cd = cooldownForLevel(sl.w.cooldown, g.rateLevel) // the fire-rate mod shortens it
	}
}

// slotWeaponName is the key of the weapon live in slot s, or "-" when empty.
func (g *Game) slotWeaponName(s int) string {
	if s < 0 || s >= numSlots || !g.slots[s].filled {
		return "-"
	}
	return weaponKeys[g.arsenal[g.slotArsIdx[s]]]
}

// slotLine is the HUD readout of the two live weapons (the icon strip is drawn
// separately); it also surfaces the laser's heat when a slot holds the beam.
func (g *Game) slotLine() string {
	line := "1:" + g.slotWeaponName(0) + "  2:" + g.slotWeaponName(1)
	holdsLaser := false
	for s := range g.slots {
		if g.slots[s].filled && g.slots[s].w.kind == wkLaser {
			holdsLaser = true
		}
	}
	if holdsLaser {
		switch {
		case g.laserHot:
			line += "   HEAT !!"
		case g.laserHeat > 0:
			line += fmt.Sprintf("   HEAT %d%%", g.laserHeat*100/laserHeatMax)
		}
	}
	if g.hasWeapon(catDevourer) {
		line += fmt.Sprintf("   DEV x%d", g.devourerAmmo)
	}
	return line
}
