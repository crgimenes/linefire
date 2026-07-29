package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// TestGodModeBlocksDamage: iddqd makes the ship take no damage, so a tester can fly
// through to distant maps without dying.
func TestGodModeBlocksDamage(t *testing.T) {
	g := New(asset.New(), boxLevel(400, 400, nil), "", false)
	g.health = maxHealth

	g.godMode = true
	g.hurtPlayer(9999)
	if g.health != maxHealth || g.over {
		t.Fatalf("god mode must absorb all damage, got health=%d over=%v", g.health, g.over)
	}

	g.godMode = false
	g.hurtPlayer(30)
	if g.health != maxHealth-30 {
		t.Fatalf("without god mode damage lands, got %d", g.health)
	}
}

// TestGrantFullArsenal: idkfa maxes every combat mod and refills vitals — the pickup pile
// in one keystroke.
func TestGrantFullArsenal(t *testing.T) {
	g := New(asset.New(), boxLevel(400, 400, nil), "", false)
	g.health, g.shield = 10, 0

	g.grantFullArsenal()

	if g.fireLevel != maxFireLevel || g.rateLevel != maxRateLevel ||
		g.damageLevel != maxDamageLevel || g.seekLevel != maxSeekLevel {
		t.Fatalf("idkfa must max the combat mods, got %+v", []int{g.fireLevel, g.rateLevel, g.damageLevel, g.seekLevel})
	}
	if !g.hasComputer || !g.autoFire {
		t.Fatal("idkfa must bring the combat computer online")
	}
	if g.health != maxHealth || g.shield != maxShield {
		t.Fatalf("idkfa must refill vitals, got health=%d shield=%d", g.health, g.shield)
	}
	if g.bubbleTime != maxBubbleTime || g.reflectTime != maxReflectTime {
		t.Fatal("idkfa must charge the timed shields")
	}
}

// TestGodModeSurvivesWarp: iddqd persists across a portal, which is the whole point —
// enable it once, then fly through the campaign to reach a far map.
func TestGodModeSurvivesWarp(t *testing.T) {
	dir := saveLevels(t, portalLevel("src", "dst", 300, 200), portalLevel("dst", "src", 300, 200))
	src := portalLevel("src", "dst", 300, 200)

	g := New(asset.New(), src, dir, false)
	g.mapName, g.startMap = "src", "src"
	g.godMode = true

	warp(t, g, 300, 200, "dst")
	if !g.godMode {
		t.Fatal("god mode must carry across a warp")
	}
}

// TestCheatBufferMatchesTypedCode drives the buffer directly (input events cannot be
// simulated headlessly) to prove the suffix match: noise before the code is ignored, and
// the buffer clears on a hit so a held key does not retrigger.
func TestCheatBufferMatchesTypedCode(t *testing.T) {
	g := New(asset.New(), boxLevel(400, 400, nil), "", false)

	feed := func(s string) {
		g.cheatBuf += s
		if len(g.cheatBuf) > cheatBufMax {
			g.cheatBuf = g.cheatBuf[len(g.cheatBuf)-cheatBufMax:]
		}
		if len(g.cheatBuf) >= 5 {
			switch g.cheatBuf[len(g.cheatBuf)-5:] {
			case "iddqd":
				g.cheatBuf = ""
				g.godMode = !g.godMode
			case "idkfa":
				g.cheatBuf = ""
				g.grantFullArsenal()
			}
		}
	}

	feed("wasd")  // movement noise
	feed("iddqd") // ...then the code
	if !g.godMode {
		t.Fatal("typing iddqd after noise should toggle god mode")
	}
	feed("iddqd")
	if g.godMode {
		t.Fatal("typing iddqd again should toggle it back off")
	}
}

// TestFullArsenalCollectsEveryWeapon: idkfa puts every catalog weapon in the arsenal and
// arms both slots.
func TestFullArsenalCollectsEveryWeapon(t *testing.T) {
	g := New(loadoutTestPlayer(), boxLevel(400, 400, nil), "", false)

	g.grantFullArsenal()

	for cat := range weaponCatalog {
		if !g.hasWeapon(cat) {
			t.Fatalf("idkfa must collect weapon %d", cat)
		}
	}
	if !g.slots[0].filled || !g.slots[1].filled {
		t.Fatal("idkfa must arm both weapon slots")
	}
}
