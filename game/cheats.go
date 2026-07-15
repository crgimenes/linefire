package game

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// DOOM-style typed cheat codes, for testing distant maps without grinding through every
// pickup: iddqd toggles god mode (no damage), idkfa grants the full arsenal at once. They
// are matched against a rolling buffer of recently typed letters, so the player just
// types the code during play — no menu, no debug build.

// cheatBufMax caps the rolling buffer; it only has to hold the longest code.
const cheatBufMax = 8

// readCheats appends this frame's typed letters and fires a cheat when the buffer ends
// with its code. Called during active play only, so a code cannot fire on a menu or the
// credits demo. The buffer is cleared on a match so a held key does not retrigger.
func (g *Game) readCheats() {
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if r < 'a' || r > 'z' {
			continue // only letters matter to a cheat; ignore the rest
		}
		g.cheatBuf += string(r)
	}
	if len(g.cheatBuf) > cheatBufMax {
		g.cheatBuf = g.cheatBuf[len(g.cheatBuf)-cheatBufMax:]
	}

	switch {
	case strings.HasSuffix(g.cheatBuf, "iddqd"):
		g.cheatBuf = ""
		g.godMode = !g.godMode
		state := "OFF"
		if g.godMode {
			state = "ON"
		}
		g.logf("IDDQD  god mode %s", state)
	case strings.HasSuffix(g.cheatBuf, "idkfa"):
		g.cheatBuf = ""
		g.grantFullArsenal()
	}
}

// grantFullArsenal maxes every combat mod, arms every mount, tops up the wing and the
// shields, and refills vitals — the equivalent of hoovering up every pickup at once. It
// reuses the real grant paths so a cheated run behaves exactly like an earned one.
func (g *Game) grantFullArsenal() {
	g.fireLevel, g.rateLevel, g.damageLevel, g.seekLevel = maxFireLevel, maxRateLevel, maxDamageLevel, maxSeekLevel
	g.hasComputer, g.autoFire = true, true

	for cat := range weaponCatalog {
		g.collectWeapon(cat) // every weapon into the arsenal
	}
	g.devourerAmmo = devourerCharges * 3    // the DEVOURER is charge-based: stock it, or it reads "no charge"
	g.slotArsIdx[0], g.slotArsIdx[1] = 0, 1 // arm two distinct weapons in the slots
	g.syncSlots()

	for g.addAlly(modeEscort) { // fill the escort formation to its cap
	}
	for g.addAlly(modeOrbit) { // fill the drone ring to its cap
	}
	g.bubbleTime, g.reflectTime = maxBubbleTime, maxReflectTime
	g.health, g.shield, g.lives = maxHealth, maxShield, startLives

	g.logf("IDKFA  full arsenal")
}
