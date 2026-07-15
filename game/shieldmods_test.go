package game

import "testing"

func TestBubbleAbsorbsAllDamage(t *testing.T) {
	g := &Game{health: 100}
	g.bubbleTime = 60
	g.hurtPlayer(30)
	if g.health != 100 {
		t.Fatalf("a live bubble should absorb all damage, health fell to %d", g.health)
	}
	// Without the bubble the same hit lands.
	g.bubbleTime = 0
	g.hurtPlayer(30)
	if g.health != 70 {
		t.Fatalf("without a bubble the hit should land, want 70 got %d", g.health)
	}
}

func TestReflectShotSeeksEnemyElseReverses(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 100, y: 0, radius: 10}}
	p := projectile{x: 0, y: 0, vx: -5, vy: 0} // incoming, moving -x
	g.reflectShot(&p)
	if len(g.projectiles) != 1 {
		t.Fatalf("reflect should spawn one player shot, got %d", len(g.projectiles))
	}
	if g.projectiles[0].vx <= 0 {
		t.Fatal("a reflected shot should seek the enemy (+x)")
	}

	// No enemy in range: the shot flies straight back the way it came (+x here).
	g.projectiles = nil
	g.entities = nil
	p2 := projectile{x: 0, y: 0, vx: -5, vy: 0}
	g.reflectShot(&p2)
	if g.projectiles[0].vx <= 0 {
		t.Fatal("with no target a reflected shot should reverse its direction (+x)")
	}
}

func TestShieldModsAddCapAndDecrement(t *testing.T) {
	g := &Game{}
	if !g.addBubble() || g.bubbleTime != bubbleAddFrames {
		t.Fatalf("a bubble pickup should grant %d frames, got %d", bubbleAddFrames, g.bubbleTime)
	}
	g.bubbleTime = maxBubbleTime
	if g.addBubble() {
		t.Fatal("a bubble past the cap should be refused")
	}
	if !g.addReflect() {
		t.Fatal("a reflector pickup should grant time")
	}

	g.bubbleTime, g.reflectTime = 2, 1
	g.stepShieldMods()
	g.stepShieldMods()
	if g.bubbleTime != 0 || g.reflectTime != 0 {
		t.Fatalf("timed shields should run down to 0, got bubble %d reflect %d", g.bubbleTime, g.reflectTime)
	}
}
