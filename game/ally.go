package game

import (
	"github.com/hajimehoshi/ebiten/v2"

	"math"
)

// Allies are friendly auto-firing companions. An ESCORT is aggressive but dutiful: it darts
// at the nearest enemy within a short PATROL range (never across the map), routing around
// walls with A* like the enemies do, holds a shooting standoff, and regroups in a loose
// trailing slot when there is nothing near to fight; a leash keeps it close to the ship.
// A DRONE orbits the ship, a passive close-in defender. Both acquire the nearest enemy and
// fire into the PLAYER projectile pool (so their shots damage enemies and reuse all
// the shot/collision logic), and both are drawn as a small copy of the player hull,
// so they read as "the same color as your ship". They are combat mods: fly-over
// pickups that STACK over a run, carried across maps and reset on restart (until
// checkpoints — see T24). v1 they are immortal helpers; nothing damages them yet
// (enemies still target only the player), so mortality is a later upgrade.

type allyMode int

const (
	modeEscort allyMode = iota // holds a slot in the trailing formation
	modeOrbit                  // circles the ship (a drone)
)

const (
	maxEscorts = 3 // escort pickups past this are wasted
	maxDrones  = 4 // drone pickups past this are wasted

	allyBulletSpeed  = 8.0   // world units per frame
	allyBulletLife   = 80    // frames
	allyShotDamage   = 1     // hull damage per ally shot (independent of the player's damage mod)
	allyFireInterval = 24    // frames between an ally's shots
	allyRange        = 460.0 // engage range; under the computer's, so kills stay on-screen

	droneOrbitRadius = 46.0 // world units from the ship center
	droneOrbitSpeed  = 0.03 // radians per frame
	escortSpacing    = 34.0 // lateral world units per regroup rank
	escortBack       = 30.0 // trailing world units per regroup rank

	huntRange    = 320.0 // only hunt enemies within this of the SHIP — its job is to guard, not roam
	huntDrop     = 380.0 // an engaged hunt only breaks past this (hysteresis: no formation<->hunt flicker at the border)
	huntStandoff = 120.0 // distance an escort holds from its prey while shooting
	huntLeash    = 280.0 // hard cap on roam from the ship, so it never strays far from its charge
	huntSpeed    = 4.0   // world units per frame an escort moves while hunting or regrouping

	// formationRideDist: within this of its slot the escort RIDES it — position locked
	// to the slot, hull matching the SHIP's heading — instead of steering at the point
	// it already sits on. The slot moves with every turn (a rank-2 slot sweeps ~5 wu
	// per turning frame, plus the ship's speed), so it must exceed the slot's own
	// per-frame travel or the ride breaks into chase jitter every frame.
	formationRideDist = 16.0

	escortScale = 0.62 // player-hull scale for an escort
	droneScale  = 0.46 // player-hull scale for a drone
)

// ally is a friendly companion (escort or drone). Position is world space; angle is
// degrees, matching the ship convention (0 = +x, the hull mesh points up).
type ally struct {
	mode    allyMode
	x, y    float64
	angle   float64
	fireCD  int
	hits    int  // enemy shots it can still take before it is destroyed
	hunting bool // escort only: engaged with a nearby enemy (breaks the formation)

	// A* navigation state for an aggressive escort, mirroring the enemy pursuit fields.
	path     []vec2
	pathStep int
	repathCD int
}

// addAlly adds a companion of the given mode up to its cap, starting at the ship so
// it eases into formation instead of flying in from a stale position. Reports
// whether one was actually added.
func (g *Game) addAlly(mode allyMode) bool {
	limit, label := maxEscorts, "ESCORT"
	if mode == modeOrbit {
		limit, label = maxDrones, "DRONE"
	}
	if g.countAllies(mode) >= limit {
		g.logf("%s  maxed", label)
		return false
	}
	g.allies = append(g.allies, ally{mode: mode, x: g.x, y: g.y, angle: g.angle, hits: allyHits})
	g.logf("%s  +1  (%d)", label, g.countAllies(mode))
	return true
}

const (
	allyHits      = 4    // enemy shots a companion survives before it is destroyed
	allyHitRadius = 14.0 // world radius an enemy shot must reach to hit a companion
)

// enemyShotHitsAlly consumes an enemy shot that strikes a companion, taking one of its hits and
// destroying it when they run out. Reports whether the shot was absorbed by a companion.
func (g *Game) enemyShotHitsAlly(px, py, nx, ny float64) bool {
	r2 := allyHitRadius * allyHitRadius
	for i := range g.allies {
		if distPointSegmentSq(g.allies[i].x, g.allies[i].y, px, py, nx, ny) <= r2 {
			g.damageAlly(i)
			return true
		}
	}
	return false
}

// damageAlly takes one hit off a companion, sparking on a glancing blow and blowing it up (with a
// heads-up in the feed) when its hits are spent.
func (g *Game) damageAlly(i int) {
	a := &g.allies[i]
	a.hits--
	if a.hits > 0 {
		g.emitBurst(a.x, a.y, shieldSparks)
		return
	}
	g.emitExplosion(a.x, a.y)
	g.addShake(deathShake * 0.4)
	g.logf("%s  down", allyLabel(a.mode))
	g.allies = append(g.allies[:i], g.allies[i+1:]...)
}

// allyLabel names a companion mode for the feed.
func allyLabel(mode allyMode) string {
	if mode == modeOrbit {
		return "DRONE"
	}
	return "ESCORT"
}

// countAllies counts current companions of a mode (for the cap and even spacing).
func (g *Game) countAllies(mode allyMode) int {
	n := 0
	for i := range g.allies {
		if g.allies[i].mode == mode {
			n++
		}
	}
	return n
}

// snapAlliesToShip drops every companion onto the ship, called after a map change so
// they re-form here instead of streaking across the new world.
func (g *Game) snapAlliesToShip() {
	for i := range g.allies {
		g.allies[i].x, g.allies[i].y = g.x, g.y
		g.allies[i].path, g.allies[i].pathStep, g.allies[i].repathCD = nil, 0, 0 // the old map's route is stale
	}
}

// stepAllies moves every companion (escort formation / drone orbit) and lets it fire
// at the nearest enemy. Cheap no-op when the squad is empty.
func (g *Game) stepAllies() {
	if len(g.allies) == 0 {
		return
	}
	g.orbitPhase += droneOrbitSpeed
	nEscort, nOrbit := g.countAllies(modeEscort), g.countAllies(modeOrbit)
	var iEscort, iOrbit int
	for i := range g.allies {
		a := &g.allies[i]
		switch a.mode {
		case modeOrbit:
			g.stepOrbit(a, iOrbit, nOrbit)
			iOrbit++
		default:
			g.stepEscort(a, iEscort, nEscort)
			iEscort++
		}
		g.allyFire(a)
	}
}

// escortSlot is the loose trailing-V regroup position for escort idx: each rank sits
// further back and wider, alternating sides. Used when there is nothing to hunt.
func (g *Game) escortSlot(idx int) (float64, float64) {
	rank := float64(idx/2 + 1)
	side := 1.0
	if idx%2 == 1 {
		side = -1.0
	}
	rad := g.angle * math.Pi / 180
	fx, fy := math.Cos(rad), math.Sin(rad)
	back := rank * escortBack
	lat := side * rank * escortSpacing
	return g.x - fx*back - fy*lat, g.y - fy*back + fx*lat
}

// stepEscort flies an AGGRESSIVE escort idx: it flies FORMATION on its trailing slot
// (riding it in the ship's heading), BREAKS formation to dart at the nearest enemy
// within the ship's patrol range — routing around walls with A*, holding a shooting
// standoff once it has a clear shot — and re-forms when the fight is over. It never
// chases across the map. allyFire aims and shoots — here we only move it.
func (g *Game) stepEscort(a *ally, idx, n int) {
	_ = n
	// Hunt with hysteresis: engage inside huntRange, and once engaged only break
	// past huntDrop — a flat threshold flipped formation<->hunt every frame while
	// an enemy hovered at the border (the oscillation seen in playtest).
	rng := huntRange
	if a.hunting {
		rng = huntDrop
	}
	ex, ey, hunting := g.nearestEnemyFrom(g.x, g.y, rng)
	a.hunting = hunting
	if hunting {
		// At the standoff with a clear line: hold and let allyFire do the shooting.
		if math.Hypot(ex-a.x, ey-a.y) <= huntStandoff && g.clearPath(a.x, a.y, ex, ey, 0) {
			a.angle = math.Atan2(ey-a.y, ex-a.x) * 180 / math.Pi
			return
		}
		wx, wy := g.allyWaypoint(a, ex, ey)
		g.moveAllyToward(a, wx, wy)
		return
	}

	// Nothing to fight: formation. Near the slot the escort rides it rigidly in the
	// SHIP's heading. Steering at the slot every frame instead left the escort glued
	// on top of it, re-facing wherever the moving slot dragged it — "always pointing
	// at the player", jittering through every turn.
	gx, gy := g.escortSlot(idx)
	if math.Hypot(gx-a.x, gy-a.y) <= formationRideDist {
		a.x, a.y = gx, gy
		a.angle = g.angle
		a.path, a.pathStep = nil, 0 // the ride needs no route; drop any stale one
		return
	}
	wx, wy := g.allyWaypoint(a, gx, gy)
	g.moveAllyToward(a, wx, wy)
}

// allyWaypoint returns the next point an escort should fly toward on its way to (gx,gy): an
// A* waypoint routing around walls (recomputed on a cooldown, corner-cut by skipping to the
// furthest waypoint still in clear sight), or (gx,gy) straight when there is no nav grid
// (procedural maps). Mirrors the enemy pursuePath logic.
func (g *Game) allyWaypoint(a *ally, gx, gy float64) (float64, float64) {
	if g.nav == nil {
		return gx, gy
	}
	if a.repathCD > 0 {
		a.repathCD--
	} else {
		a.path = g.nav.findPath(a.x, a.y, gx, gy)
		a.pathStep = 0
		a.repathCD = repathInterval
	}
	for a.pathStep < len(a.path) && math.Hypot(a.path[a.pathStep].x-a.x, a.path[a.pathStep].y-a.y) < navCell*0.6 {
		a.pathStep++
	}
	for a.pathStep+1 < len(a.path) && g.clearPath(a.x, a.y, a.path[a.pathStep+1].x, a.path[a.pathStep+1].y, 0) {
		a.pathStep++
	}
	if a.pathStep < len(a.path) {
		return a.path[a.pathStep].x, a.path[a.pathStep].y
	}
	return gx, gy
}

// moveAllyToward steps an escort toward (tx,ty) at huntSpeed, FACING where it flies (the
// fix for companions sliding sideways), then leashes it to the ship so a chase never drags
// it off-screen.
func (g *Game) moveAllyToward(a *ally, tx, ty float64) {
	mx, my := tx-a.x, ty-a.y
	dist := math.Hypot(mx, my)
	if dist > 1e-6 {
		a.angle = math.Atan2(my, mx) * 180 / math.Pi // point in the direction of travel
	}
	if dist > huntSpeed {
		a.x += mx / dist * huntSpeed
		a.y += my / dist * huntSpeed
	} else {
		a.x, a.y = tx, ty
	}
	lx, ly := a.x-g.x, a.y-g.y
	ld := math.Hypot(lx, ly)
	if ld > huntLeash {
		a.x = g.x + lx/ld*huntLeash
		a.y = g.y + ly/ld*huntLeash
	}
}

// stepOrbit places drone idx (of n) on the shared orbit, evenly phased so drones
// spread around the ship. Faces along its travel unless firing.
func (g *Game) stepOrbit(a *ally, idx, n int) {
	if n < 1 {
		n = 1
	}
	base := g.orbitPhase + float64(idx)*2*math.Pi/float64(n)
	a.x = g.x + droneOrbitRadius*math.Cos(base)
	a.y = g.y + droneOrbitRadius*math.Sin(base)
	a.angle = (base + math.Pi/2) * 180 / math.Pi // tangent to the orbit
}

// allyFire aims the companion at the nearest enemy in range and, off cooldown, adds
// a player-colored shot to the projectile pool. Aiming happens even on cooldown, so
// a companion visibly tracks its target between shots.
func (g *Game) allyFire(a *ally) {
	if a.fireCD > 0 {
		a.fireCD--
	}
	if a.mode == modeEscort && !a.hunting {
		return // in formation: hold the shape and the heading until enemies come near
	}
	ex, ey, ok := g.nearestEnemyFrom(a.x, a.y, allyRange)
	if !ok {
		return
	}
	dx, dy := ex-a.x, ey-a.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	a.angle = math.Atan2(dy, dx) * 180 / math.Pi
	if a.fireCD > 0 {
		return
	}
	g.projectiles = append(g.projectiles, projectile{
		x: a.x, y: a.y, px: a.x, py: a.y,
		vx: dx / d * allyBulletSpeed, vy: dy / d * allyBulletSpeed,
		life: allyBulletLife, dmg: allyShotDamage, col: damageColor,
		rcol: bulletColor, rglow: bulletGlowColor, width: bulletWidth, glowW: bulletGlowWidth,
	})
	a.fireCD = allyFireInterval
	g.sfx.play(soundReq{"shot_soft", 180, 0.2}) // quiet: a squad should not be a wall of noise
}

// nearestEnemyFrom returns the position of the nearest enemy to (x,y) in line of
// sight within rng, if any.
func (g *Game) nearestEnemyFrom(x, y, rng float64) (float64, float64, bool) {
	best := rng * rng
	var bx, by float64
	found := false
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		dx, dy := e.x-x, e.y-y
		d := dx*dx + dy*dy
		if d < best && g.lineOfSight(x, y, e.x, e.y) {
			best, bx, by, found = d, e.x, e.y, true
		}
	}
	return bx, by, found
}

// allyScale is the player-hull scale a companion draws at.
func allyScale(mode allyMode) float64 {
	if mode == modeOrbit {
		return droneScale
	}
	return escortScale
}

// allyGeoMAt is the hull transform for a companion: the player mesh scaled down,
// oriented like the ship, placed in the world through the camera.
func (g *Game) allyGeoMAt(x, y, angle, scale, camX, camY, camAngle float64) ebiten.GeoM {
	var m ebiten.GeoM
	if g.player != nil {
		m.Translate(-g.player.Origin.X, -g.player.Origin.Y)
	}
	m.Scale(scale, scale)
	m.Rotate((angle + 90) * math.Pi / 180)
	m.Translate(x, y)
	g.appendCameraAt(&m, camX, camY, camAngle)
	return m
}

// drawAllies strokes each companion as a small player hull (crisp pass).
func (g *Game) drawAllies(dst *ebiten.Image, camX, camY, camAngle float64) {
	if g.playerMesh == nil {
		return
	}
	for i := range g.allies {
		a := &g.allies[i]
		g.playerMesh.Draw(dst, g.allyGeoMAt(a.x, a.y, a.angle, allyScale(a.mode), camX, camY, camAngle), true)
	}
}

// drawAllyGlow strokes each companion's emissive layer into the bloom pass.
func (g *Game) drawAllyGlow(emissive *ebiten.Image, camX, camY, camAngle float64) {
	if g.playerGlow == nil {
		return
	}
	for i := range g.allies {
		a := &g.allies[i]
		g.playerGlow.Draw(emissive, g.allyGeoMAt(a.x, a.y, a.angle, allyScale(a.mode), camX, camY, camAngle), true)
	}
}
