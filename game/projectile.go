package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/effects"
	"github.com/crgimenes/linefire/render"
	"github.com/crgimenes/linefire/ship"
)

const (
	bulletSpeed      = 9.0 // world units per frame, along the ship heading
	bulletLife       = 90  // frames a bullet lives before fading (~1.5s at 60 TPS)
	fireInterval     = 8   // frames between shots while the fire key is held
	playerShotDamage = 1   // hull damage one gun bullet deals

	bulletWidth     = 1.6 // crisp core stroke, logical px (scaled by DPI)
	bulletGlowWidth = 2.0 // emissive stroke feeding the bloom
	bulletHitRadius = 2.0 // world units; bullet-vs-wall collision tolerance

	// The bolt's own halo, added under the core in the crisp pass (the bloom still
	// lays the soft outer glow on top). Bolts used to be a flat painted line that
	// the rock swallowed; adding light instead — kutta's smoke trick — makes them
	// read against any background without turning up the bloom for the whole scene.
	//
	// The halo and the emissive stroke are both kept close to the core width: at
	// 3.2x the core the bolt read as a thick bar of light, out of key with the thin
	// vector art around it. The wake stays — a shot is still light fading out
	// behind itself — it is just narrow now.
	boltHaloWidth  = 2.0 // halo stroke as a multiple of the core width
	boltHaloGain   = 0.3 // how much of the halo color each bolt adds
	boltTrailSteps = 4   // wake segments dragged behind the bolt, each thinner and dimmer
	boltTrailSpan  = 1.6 // travel each wake segment spans, in frames (a frame's step is tiny)

	// Direct-hit impact effect (impactBurst), scaled by the shot's damage so a
	// bullet is a small spark and a heavier round is punchier — the missile blast's
	// small cousin, keeping the same proportions.
	impactSparksBase   = 3   // sparks for a 1-damage ping
	impactSparksPerDmg = 2   // extra sparks per point of damage
	impactMaxSparks    = 12  // cap so a heavy round cannot flood the pool
	impactShakeBase    = 0.6 // shake when a shot connects with an enemy
	impactShakePerDmg  = 0.5 // extra shake per point of damage

	enemyBulletSpeed  = ship.BaseShotSpeed  // world units per frame, aimed at the player
	enemyBulletLife   = 150                 // frames
	enemyFireInterval = ship.BaseFireEvery  // frames between an enemy's shots
	enemyBulletDamage = ship.BaseShotDamage // health lost when an enemy shot connects
)

var (
	bulletColor     = color.RGBA{0xe8, 0xff, 0xff, 0xff} // hot near-white core
	bulletGlowColor = color.RGBA{0x80, 0xff, 0xff, 0xff} // cyan halo

	enemyShotColor     = color.RGBA{0xff, 0xd0, 0xb0, 0xff} // hot orange core
	enemyShotGlowColor = color.RGBA{0xff, 0x50, 0x30, 0xff} // red halo
)

// projectile is a player bullet travelling in world space. px,py is the previous
// position so it can be drawn as a short streak, which gives it its own motion
// blur independent of the camera. Each shot carries its own payload — dmg, the
// damage-number color, and aoe (explosion radius; 0 = direct hit) — so one step
// path serves the front gun, the turret and the missile.
type projectile struct {
	x, y   float64
	px, py float64
	vx, vy float64
	life   int

	dmg  int        // hull damage on hit (the center damage for an AoE shot)
	col  color.RGBA // floating damage-number color for this shot's source
	aoe  float64    // explosion radius in world units; 0 = direct-hit only
	seek float64    // homing turn rate, radians/frame (0 = flies straight)

	rcol  color.RGBA // core render color (per-weapon, so one pool draws every shot)
	rglow color.RGBA // glow render color
	width float64    // crisp stroke width, logical px
	glowW float64    // glow stroke width, logical px
}

// weaponHardpoint returns the asset's first weapon hardpoint.
func weaponHardpoint(a *asset.Asset) (asset.Hardpoint, bool) {
	if a == nil {
		return asset.Hardpoint{}, false
	}
	for _, h := range a.Hardpoints {
		if h.Kind == asset.KindWeapon {
			return h, true
		}
	}
	return asset.Hardpoint{}, false
}

// muzzleWorld is the world position of the player's gun muzzle: the weapon
// hardpoint offset (asset space, relative to the origin) rotated into the
// world by the ship heading. Assets point up (-90), so the asset->world turn is
// angle+90, matching how entities are oriented.
func (g *Game) muzzleWorld() (float64, float64) {
	var ox, oy float64
	hp, ok := weaponHardpoint(g.player)
	if ok {
		ox = hp.X - g.player.Origin.X
		oy = hp.Y - g.player.Origin.Y
	}
	rad := (g.angle + 90) * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	return g.x + ox*cos - oy*sin, g.y + ox*sin + oy*cos
}

// drawMuzzleFlash draws a bright disc at the gun muzzle while it lingers; radius
// is given in logical px and shrinks/fades as the flash decays.
func (g *Game) drawMuzzleFlash(dst *ebiten.Image, cam ebiten.GeoM, col color.RGBA, radius float64) {
	if g.muzzleFlash <= 0 {
		return
	}
	frac := float64(g.muzzleFlash) / float64(muzzleFlashFrames)
	mx, my := g.muzzleWorld()
	cx, cy := cam.Apply(mx, my)
	c := col
	c.A = uint8(float64(col.A) * frac)
	render.FillCircle(dst, cx, cy, radius*g.dpr*frac, c)
}

// stepProjectiles ages the weapon cooldowns and advances every player projectile
// (one pool for all weapons). The whole travel segment is tested so fast bullets
// never tunnel through a thin wall.
func (g *Game) stepProjectiles() {
	g.tickWeapons()
	if g.muzzleFlash > 0 {
		g.muzzleFlash--
	}
	g.projectiles = g.stepPlayerShots(g.projectiles)
}

// stepPlayerShots advances the player projectiles, dropping those that expire or
// land, and returns the survivors. A direct shot (aoe==0) damages the enemy it
// hits / sparks off a wall; an AoE shot (aoe>0, e.g. a missile) detonates an area
// blast at the impact point instead. Each shot carries its own dmg/col/aoe/look.
func (g *Game) stepPlayerShots(shots []projectile) []projectile {
	kept := shots[:0]
	for i := range shots {
		p := shots[i]
		p.px, p.py = p.x, p.y
		if p.seek > 0 {
			if tx, ty, ok := g.nearestEnemyFrom(p.x, p.y, homingRange); ok {
				p.vx, p.vy = steerToward(p.vx, p.vy, tx-p.x, ty-p.y, p.seek)
			}
		}
		nx, ny := p.x+p.vx, p.y+p.vy
		p.life--
		if p.life <= 0 {
			continue
		}
		rx, ry, hitRock := g.bulletRockHit(p.px, p.py, nx, ny)
		if hitRock {
			if p.aoe > 0 {
				g.explodeAt(rx, ry, p.dmg, p.aoe, p.col)
			} else {
				g.dig(rx, ry, p.dmg)             // SPIKE: the shot eats a bite of rock
				g.impactBurst(rx, ry, &p, false) // missed into a wall: sparks, no reward shake
			}
			continue
		}
		hit := g.bulletHitsEnemy(p.px, p.py, nx, ny)
		if hit >= 0 {
			if p.aoe > 0 {
				g.explodeAt(nx, ny, p.dmg, p.aoe, p.col)
			} else {
				g.damageEnemy(hit, p.dmg, p.col)
				g.impactBurst(nx, ny, &p, true) // connected: sparks + a small punch
			}
			continue
		}
		// Pickups are breakable by deliberate player fire (enemies checked first, so
		// a shot never favors a crate over a threat). No score, no reward shake.
		if hit := g.bulletHitsPickup(p.px, p.py, nx, ny); hit >= 0 {
			if p.aoe > 0 {
				g.explodeAt(nx, ny, p.dmg, p.aoe, p.col)
			}
			g.damagePickup(hit, p.dmg)
			g.impactBurst(nx, ny, &p, false)
			continue
		}
		p.x, p.y = nx, ny
		if p.aoe > 0 {
			g.emitMissileTrail(p.x, p.y)
		}
		kept = append(kept, p)
	}
	return kept
}

// impactBurst is the small-scale cousin of the missile blast: a direct (non-AoE)
// shot striking a wall or enemy throws a few sparks in the shot's own glow color,
// scaled by its damage, so a front-gun ping is tiny and a heavier round is punchier.
// hitEnemy adds a light screen shake, so connecting feels rewarding while a wall
// miss just sparks.
func (g *Game) impactBurst(x, y float64, p *projectile, hitEnemy bool) {
	col := p.rglow
	n := min(impactSparksBase+p.dmg*impactSparksPerDmg, impactMaxSparks)
	g.emitBurst(x, y, effects.Burst{
		N: n, Col: col, Style: effects.StyleStreak,
		SpeedMin: 1.5, SpeedMax: 3.0 + float64(p.dmg), LifeMin: 6, LifeMax: 14, Drag: 0.82,
	})
	if hitEnemy {
		g.addShake(impactShakeBase + float64(p.dmg)*impactShakePerDmg)
	}
}

// stepEnemyShots advances enemy bullets, dropping those that expire, hit a wall,
// or connect with the player (which damages the player). The whole travel
// segment is tested so fast shots never tunnel past the ship.
func (g *Game) stepEnemyShots() {
	// Iterate a snapshot of the slice: a hit calls hurtPlayer, which may respawn
	// the ship and reset game state; the loop bounds must not depend on that.
	shots := g.enemyShots
	kept := shots[:0]
	for i := range shots {
		p := shots[i]
		p.px, p.py = p.x, p.y
		nx, ny := p.x+p.vx, p.y+p.vy
		p.life--
		if p.life <= 0 {
			continue
		}
		rx, ry, hitRock := g.bulletRockHit(p.px, p.py, nx, ny)
		if hitRock {
			g.dig(rx, ry, p.dmg) // SPIKE: enemy fire chews the rock too
			// The player's wall sparks (impactBurst) in the shot's own color. The
			// spark COUNT is not shared: impactBurst scales it by damage, and enemy
			// damage is in player-health units (16-32) while player damage is in
			// enemy-hull units (1), so the same formula would read as a much bigger
			// burst for the same event.
			sparks := wallSparks
			sparks.Col = p.rglow
			g.emitBurst(rx, ry, sparks)
			continue
		}
		if g.invuln <= 0 && distPointSegmentSq(g.x, g.y, p.px, p.py, nx, ny) <= g.radius*g.radius {
			if g.reflectTime > 0 {
				p.x, p.y = nx, ny
				g.reflectShot(&p) // deflect it back as a player shot instead of taking damage
				continue
			}
			dmg := p.dmg
			if dmg <= 0 {
				dmg = enemyBulletDamage // grunts / direct-built shots use the baseline
			}
			g.hurtPlayer(dmg) // a live bubble makes this a no-op (absorbed)
			continue
		}
		if g.enemyShotHitsAlly(p.px, p.py, nx, ny) {
			continue // an escort took it (and maybe went down)
		}
		p.x, p.y = nx, ny
		kept = append(kept, p)
	}
	g.enemyShots = kept
}

// bulletHitsWall reports whether the travel segment comes within bulletHitRadius
// of any wall. A distance test (not a strict crossing test) is used so a bullet
// that lands exactly on a wall line — e.g. a slow shot whose travel divides the
// gap evenly — still registers instead of tunnelling through.
func (g *Game) bulletHitsWall(ax, ay, bx, by float64) bool {
	_, _, ok := g.bulletRockHit(ax, ay, bx, by)
	return ok
}

// bulletRockHit walks the bullet's travel and reports the first point that is inside
// rock — the face it struck, and where the bite gets taken. With a region field this
// is the truth (a tunnel the player dug lets the shot through); procedural maps have
// no field and fall back to the wall segments.
func (g *Game) bulletRockHit(ax, ay, bx, by float64) (float64, float64, bool) {
	f := g.flood
	if f == nil {
		const hitSq = bulletHitRadius * bulletHitRadius
		for _, s := range g.segs {
			if segmentDistSq(ax, ay, bx, by, s.ax, s.ay, s.bx, s.by) <= hitSq {
				return bx, by, true
			}
		}
		return 0, 0, false
	}
	length := math.Hypot(bx-ax, by-ay)
	n := max(int(length/(f.cell*0.5)), 1)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, y := ax+(bx-ax)*t, ay+(by-ay)*t
		if f.rockAt(x, y) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// segmentDistSq is the squared minimum distance between segments p1-p2 and
// p3-p4: zero if they cross, else the smallest endpoint-to-segment distance.
func segmentDistSq(x1, y1, x2, y2, x3, y3, x4, y4 float64) float64 {
	if segmentsIntersect(x1, y1, x2, y2, x3, y3, x4, y4) {
		return 0
	}
	d := distPointSegmentSq(x1, y1, x3, y3, x4, y4)
	d = math.Min(d, distPointSegmentSq(x2, y2, x3, y3, x4, y4))
	d = math.Min(d, distPointSegmentSq(x3, y3, x1, y1, x2, y2))
	d = math.Min(d, distPointSegmentSq(x4, y4, x1, y1, x2, y2))
	return d
}

// bulletHitsEnemy returns the index of the first enemy whose hit circle the
// travel segment passes within, or -1 if none. Testing the whole segment keeps
// fast bullets from tunnelling past a small enemy.
func (g *Game) bulletHitsEnemy(ax, ay, bx, by float64) int {
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		if distPointSegmentSq(e.x, e.y, ax, ay, bx, by) <= e.radius*e.radius {
			return i
		}
	}
	return -1
}

// boltTrail drags a wake behind a bolt: the frame's travel segment repeated back
// down its own path, each step thinner and dimmer, so the shot reads as light
// fading out behind it instead of a mark that stops dead at its tail. It is the
// same additive light as the halo — the wake IS the glow, smeared along the path —
// so it needs no particles and nothing to simulate.
func (g *Game) boltTrail(dst *ebiten.Image, x0, y0, x1, y1, width float64, col color.RGBA, gain float64) {
	dx := (x0 - x1) * boltTrailSpan // travel pointing back down the path
	dy := (y0 - y1) * boltTrailSpan
	for i := range boltTrailSteps {
		f := 1 - float64(i)/boltTrailSteps // full behind the bolt, fading to nothing at the tail
		ax, ay := x0+dx*float64(i), y0+dy*float64(i)
		render.StrokeLineAdd(dst, ax, ay, ax+dx, ay+dy, width*f*g.dpr, col, gain*f)
	}
}

// drawBolt draws one projectile streak the way kutta composites its glowing smoke:
// a wide dim halo with the bright core added over it, both ADDING light rather
// than painting over the world, trailing a short wake. The bloom pass then lays
// the soft outer halo on top of that.
func (g *Game) drawBolt(dst *ebiten.Image, x0, y0, x1, y1, width float64, core, halo color.RGBA) {
	g.boltTrail(dst, x0, y0, x1, y1, width*boltHaloWidth, halo, boltHaloGain)
	render.StrokeLineAdd(dst, x0, y0, x1, y1, width*boltHaloWidth*g.dpr, halo, boltHaloGain)
	render.StrokeLineAdd(dst, x0, y0, x1, y1, width*g.dpr, core, 1)
}

// drawBolts draws a projectile pool as streaks from each shot's previous to its
// current position, each bullet in its own color and width (so one pool can carry
// shots from every equipped weapon). glow selects the wider, softer stroke that
// feeds the bloom instead of the bolt itself.
//
// Both pools draw through here: an enemy shot used to take a separate path with
// the colors hardcoded, which is how it drifted into being a different effect
// rather than the same one in another color.
func (g *Game) drawBolts(dst *ebiten.Image, cam ebiten.GeoM, shots []projectile, glow bool) {
	for i := range shots {
		p := &shots[i]
		x0, y0 := cam.Apply(p.px, p.py)
		x1, y1 := cam.Apply(p.x, p.y)
		if glow {
			g.boltTrail(dst, x0, y0, x1, y1, p.glowW, p.rglow, 1)
			render.StrokeLine(dst, x0, y0, x1, y1, p.glowW*g.dpr, p.rglow)
			continue
		}
		g.drawBolt(dst, x0, y0, x1, y1, p.width, p.rcol, p.rglow)
	}
}

// segmentsIntersect reports whether segments p1-p2 and p3-p4 cross, using the
// standard orientation test.
func segmentsIntersect(x1, y1, x2, y2, x3, y3, x4, y4 float64) bool {
	d1 := orient(x3, y3, x4, y4, x1, y1)
	d2 := orient(x3, y3, x4, y4, x2, y2)
	d3 := orient(x1, y1, x2, y2, x3, y3)
	d4 := orient(x1, y1, x2, y2, x4, y4)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return false
}

// orient is the signed area (z of the cross product) of (a->b) x (a->c); its
// sign tells which side of line a-b point c lies on.
func orient(ax, ay, bx, by, cx, cy float64) float64 {
	return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
}
