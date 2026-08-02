package game

import (
	"image/color"
	"io/fs"
	"math"
	"strings"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/render"
	"github.com/crgimenes/linefire/ship"
)

// entityKind classifies a runtime actor.
type entityKind int

const (
	kindEnemy entityKind = iota
	kindPowerUp
	kindPortal
	kindWeapon // a weapon lying on the map: fly over to equip or drop-swap
)

// entity is a runtime instance spawned from the level. It is kept separate from
// the loaded level/asset documents so the game never mutates editor data. The
// mesh is the asset's geometry in asset coordinates, drawn with a per-entity
// camera GeoM.
type entity struct {
	kind       entityKind
	align      alignment // map-marker classification (good/bad/ally/neutral)
	faction    int       // team: 0 is the campaign's horde (it preys on the player); skirmish ships are 1..N and prey on each other
	id         int       // stable ship id (enemyEntity deals them); 0 for anything else
	x, y       float64
	angle      float64
	radius     float64
	vx, vy     float64 // current steering velocity (smoothed, to damp jitter at corners)
	power      string  // power-up effect, or the weapon key (front/turret/...) for kindWeapon
	target     string  // portal destination "map" or "map:label" (kindPortal only)
	inside     bool    // player currently overlaps (kindWeapon: swap fires once per entry)
	spawn      int     // index into level.Spawns this came from (per-map persistence)
	hp         int     // remaining hits before the enemy is destroyed (0 for power-ups)
	fireCD     int     // frames until this enemy can fire again
	hitFlash   int     // frames the white impact flash still shows
	radar      float64 // current engage range; grows when the enemy is hit
	stationary bool    // turret archetype: never moves, only faces and fires
	boss       bool    // shielded (invulnerable) until every non-boss enemy on the map is dead
	fireEvery  int     // archetype fire interval (frames between shots; <=0 = baseline)
	shotDmg    int     // archetype damage per shot to the player
	shotSpeed  float64 // archetype projectile speed
	speedMul   float64 // archetype movement-speed multiplier (1 = baseline)
	orbitDir   float64 // +1 / -1: which way it circles the player
	combat     bool    // alerted: tracks the player through walls until they leave the radar
	standoff   float64 // preferred orbit distance (randomized per enemy so they spread out)
	path       []vec2  // A* waypoints toward the player when out of line of sight
	pathStep   int     // index of the next waypoint
	repathCD   int     // frames until the path is recomputed
	stuck      int     // frames of no progress while pursuing (triggers an unstick)
	wanderHead float64 // current heading while patrolling (radians)
	wanderCD   int     // frames until the patrol picks a new heading
	alertGrace int     // frames of combat left after losing the player (before giving up)
	hpMax      int     // what this hull was built with; a repair never goes past it
	shield     int     // salvaged shield: absorbs damage before the hull does
	dmgMod     int     // salvaged combat mods: harder bolts, shorter interval, wider volley
	rateMod    int
	fireMod    int
	pilot      *shipPilot // skirmish: the Filo program flying this hull (nil = house brain)
	a          *asset.Asset
	mesh       *render.Mesh // nil if the asset is missing
	glowMesh   *render.Mesh // glowing strokes for the bloom, nil if missing
}

const (
	// enemyHP is how many bullet hits an enemy takes before it is destroyed.
	enemyHP = ship.BaseHP
	// hitFlashFrames is how long an enemy's white impact flash lingers.
	hitFlashFrames = 6
	// radarRange is how close (world units) the player must be for an enemy to
	// engage; outside it the enemy stays idle.
	radarRange = ship.BaseRadar
	// radarHitBoost grows an enemy's engage range each time it is hit, so it
	// retaliates against fire from beyond its normal radar instead of being picked
	// off from afar. radarMaxMul caps the growth as a multiple of the base radar.
	radarHitBoost = 1.6
	radarMaxMul   = 2.5
	// radarCombatMul widens the radar the moment an enemy engages, so a fast,
	// nimble player cannot shake it the instant they pull back.
	radarCombatMul = 2.0

	// Movement when engaged: approach to a standoff distance and orbit there
	// (close enough to threaten, far enough not to sit on the player).
	enemySpeed        = 1.8               // world units per frame
	enemyStandoff     = ship.BaseStandoff // preferred distance from the player
	enemyStandoffBand = 24                // deadband around the standoff where it holds position
	enemyOrbit        = 0.9               // tangential weight relative to the radial approach
	enemySepDist      = 64                // enemies closer than this push each other apart
	enemySepWeight    = 1.2               // strength of that separation in the steering mix
	repathInterval    = 18                // frames between A* recomputations while pursuing
	stuckEps          = 0.3               // movement below this per frame counts as no progress
	stuckLimit        = 8                 // frames of no progress before forcing an unstick
	velSmooth         = 0.25              // how fast the steering velocity follows the target (inertia)
	enemyTurnRate     = 8.0               // max degrees the enemy turns per frame (no snap-spinning)

	patrolSpeed     = 0.7 // world units per frame while idle/wandering
	patrolHold      = 90  // frames an enemy commits to a heading before changing it
	patrolTurnRange = 0.9 // radians the heading can swing when it changes
	combatGrace     = 120 // frames an enemy keeps chasing after the player leaves the radar
)

// spawnEntityKind maps a spawn's category (from the asset) to a runtime entity
// kind: "enemy" is hostile, "portal" warps to another map, and every other
// category ("heal", "shield", "score", empty/unknown) is treated as a pickup.
func spawnEntityKind(category string) entityKind {
	if strings.HasPrefix(category, "weapon-") {
		return kindWeapon
	}
	switch category {
	case "enemy", "turret", "rusher", "sniper", "tank":
		return kindEnemy
	case "portal":
		return kindPortal
	default:
		return kindPowerUp
	}
}

// archetypeFor returns the profile for an enemy Kind. The table itself lives in
// the ship package, shared with Linefire Skirmish, which fields the same types.
func archetypeFor(kind string) ship.Profile {
	return ship.For(kind)
}

// moveSpeed is the enemy's per-frame movement speed: the base scaled by its
// archetype multiplier (defaulting to the baseline when unset, e.g. in tests that
// build an entity directly).
func (e *entity) moveSpeed() float64 {
	m := e.speedMul
	if m <= 0 {
		m = 1
	}
	return enemySpeed * m
}

// buildEntities instantiates runtime entities from a level's spawns. A spawn names its
// asset ("turret"); content/dir is where those live. Each is loaded and triangulated
// once (cached by name); a missing/invalid asset yields a nil-mesh entity (drawn as a
// marker), never an error that stops the game.
func buildEntities(content fs.FS, l *level.Level, dir string) []entity {
	load := func(ref string) (*asset.Asset, *render.Mesh, *render.Mesh) {
		return loadCachedAsset(content, dir, ref) // session cache: shared across every world rebuild
	}

	var es []entity
	add := func(kind entityKind, s level.Spawn) {
		a, m, gm := load(s.Asset)
		var arch ship.Profile
		if kind == kindEnemy {
			arch = archetypeFor(s.Kind)
		}
		hp := 0
		if kind == kindEnemy {
			hp = arch.HP
		}
		if kind == kindPowerUp || kind == kindWeapon {
			hp = pickupHP // breakable by deliberate player fire
		}
		radar := 0.0
		if kind == kindEnemy {
			radar = arch.Radar
		}
		power := ""
		if kind == kindPowerUp {
			power = s.Kind
			if power == "" {
				power = powerHeal
			}
		}
		if kind == kindWeapon {
			power = strings.TrimPrefix(s.Kind, "weapon-") // the weapon key: front/turret/...
		}
		target := ""
		if kind == kindPortal {
			target = s.Target
		}
		align := alignNeutral
		switch kind {
		case kindEnemy:
			align = alignBad
		case kindPowerUp, kindWeapon:
			align = alignGood
		case kindPortal:
			align = alignAlly // portals show as friendly navigation markers (blue)
		}
		es = append(es, entity{
			kind: kind, align: align, x: s.X, y: s.Y, angle: s.Angle,
			hp: hp, radar: radar, power: power, target: target,
			stationary: arch.Stationary, fireEvery: arch.FireEvery,
			shotDmg: arch.ShotDamage, shotSpeed: arch.ShotSpeed, speedMul: arch.SpeedMul,
			a: a, mesh: m, glowMesh: gm, radius: assetRadius(a),
		})
	}
	for i, s := range l.Spawns {
		kind := spawnEntityKind(s.Kind)
		add(kind, s)
		es[len(es)-1].spawn = i // remember the source spawn for per-map persistence
		if kind != kindEnemy {
			continue
		}
		e := &es[len(es)-1]
		e.boss = s.Boss // shielded until its escorts are cleared
		// Alternate the orbit direction so a group does not all circle the same way,
		// and vary the standoff (from the archetype's preferred distance) so they
		// spread to different orbit radii (no single file behind the player).
		e.orbitDir = 1.0
		if i%2 == 1 {
			e.orbitDir = -1.0
		}
		e.standoff = archetypeFor(s.Kind).Standoff * (0.8 + randFloat()*0.5)
		e.wanderHead = s.Angle * math.Pi / 180
	}
	return es
}

// orbitHold keeps an engaged, visible enemy near its standoff distance and
// strafes around its target. It runs only when the target is in sight and close
// (so the enemy is in the open); the long approach is all A*. Wall clearance is
// handled by the navgrid heat field, not by steering, so there is nothing to
// fight at a wall tip.
func (g *Game) orbitHold(e *entity, tx, ty, dist, sd float64) {
	dx, dy := tx-e.x, ty-e.y
	if dist == 0 {
		return
	}
	ux, uy := dx/dist, dy/dist // unit vector toward the target

	radial := 0.0
	if dist < sd-enemyStandoffBand {
		radial = -1 // too close: back off so it never sits on the ship
	}
	perpX, perpY := -uy, ux
	vx := ux*radial + perpX*e.orbitDir*enemyOrbit
	vy := uy*radial + perpY*e.orbitDir*enemyOrbit
	g.applyEnemyMove(e, vx, vy, e.moveSpeed())
}

// applyEnemyMove adds separation to a desired velocity, normalizes to the given
// speed, smooths it (inertia) and moves, sliding along walls. Wall clearance is
// the navgrid heat field's job (it keeps A* paths cool), so there is no wall
// avoidance here to fight the path at corners.
func (g *Game) applyEnemyMove(e *entity, vx, vy, speed float64) {
	sx, sy := g.separation(e)
	vx += sx
	vy += sy
	m := math.Hypot(vx, vy)
	if m > 0 {
		vx, vy = vx/m*speed, vy/m*speed
	}
	e.vx += (vx - e.vx) * velSmooth
	e.vy += (vy - e.vy) * velSmooth

	bx, by := e.x, e.y
	g.moveEnemy(e, e.vx, e.vy)
	// Kill velocity components a wall blocked, so the smoothed velocity stops
	// pushing into the wall and can redirect along it next frame.
	if e.vx != 0 && e.x == bx {
		e.vx = 0
	}
	if e.vy != 0 && e.y == by {
		e.vy = 0
	}
}

// shouldHold reports whether an engaged enemy sits at its standoff and orbits
// instead of closing in. It never holds when the target sits inside rock BLASTED
// OPEN: an orbit needs open space all around the target, and a burrow has none —
// so the enemy stops circling the mouth and comes in after it.
func (g *Game) shouldHold(tx, ty, dist, sd float64, los bool) bool {
	if !los || dist > sd+enemyStandoffBand {
		return false
	}
	return !g.targetInDug(tx, ty)
}

// targetInDug reports whether a point is inside a tunnel or crater blasted out
// of the rock (as opposed to an authored corridor).
func (g *Game) targetInDug(x, y float64) bool {
	return g.flood != nil && g.flood.dugAt(x, y)
}

// pursuePath chases the target around walls using the A* path, recomputed on a
// cooldown since the target keeps moving. Without a usable path it heads straight
// (sliding along walls) as a fallback.
func (g *Game) pursuePath(e *entity, tx, ty float64) {
	if g.nav == nil {
		g.steerToward(e, tx, ty)
		return
	}
	if e.repathCD > 0 {
		e.repathCD--
	} else {
		e.path = g.nav.findPath(e.x, e.y, tx, ty)
		e.pathStep = 0
		e.repathCD = repathInterval
	}

	// Drop waypoints already reached, then smooth: skip ahead to the furthest one
	// still in direct sight, so the enemy cuts corners instead of stepping cell to
	// cell. Wall avoidance keeps the shortcut clear of the walls.
	for e.pathStep < len(e.path) && math.Hypot(e.path[e.pathStep].x-e.x, e.path[e.pathStep].y-e.y) < navCell*0.6 {
		e.pathStep++
	}
	for e.pathStep+1 < len(e.path) && g.clearPath(e.x, e.y, e.path[e.pathStep+1].x, e.path[e.pathStep+1].y, e.radius) {
		e.pathStep++
	}

	if e.pathStep < len(e.path) {
		wp := e.path[e.pathStep]
		g.steerToward(e, wp.x, wp.y)
	} else {
		g.steerToward(e, tx, ty)
	}
}

// turnToward rotates current toward target (degrees) by at most maxStep, taking
// the short way around, so ships turn smoothly instead of snapping.
func turnToward(current, target, maxStep float64) float64 {
	diff := normDeg(target - current)
	if diff > maxStep {
		diff = maxStep
	} else if diff < -maxStep {
		diff = -maxStep
	}
	return current + diff
}

// normDeg wraps an angle in degrees to (-180, 180].
func normDeg(a float64) float64 {
	for a > 180 {
		a -= 360
	}
	for a <= -180 {
		a += 360
	}
	return a
}

// unstick frees an enemy whose pursuit steering deadlocked at a wall corner: it
// forces a fresh path next frame and nudges sideways (trying both directions) to
// slip off the corner.
func (g *Game) unstick(e *entity, tx, ty float64) {
	e.repathCD = 0
	e.orbitDir = -e.orbitDir // if pinned while orbiting, try the other way around
	dx, dy := tx-e.x, ty-e.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	// Redirect the smoothed velocity sideways so its inertia stops pulling the
	// ship back into the wall it was pinned against. Sideways is relative to the
	// TARGET the ship is actually hunting: measured against anything else (this
	// once read the player, which in skirmish is the parked ghost), a wedged
	// ship can be handed two escape directions that are both walls, forever.
	nx, ny := -dy/d, dx/d // perpendicular to the target direction
	e.vx, e.vy = nx*e.moveSpeed(), ny*e.moveSpeed()
	bx, by := e.x, e.y
	g.moveEnemy(e, e.vx, e.vy)
	if e.x == bx && e.y == by {
		e.vx, e.vy = -e.vx, -e.vy
		g.moveEnemy(e, e.vx, e.vy)
	}
}

// steerToward moves an enemy straight at a target (no orbit). Used for path
// following; separation and wall avoidance are applied by applyEnemyMove.
func (g *Game) steerToward(e *entity, tx, ty float64) {
	dx, dy := tx-e.x, ty-e.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	g.applyEnemyMove(e, dx/d, dy/d, e.moveSpeed())
}

// separation returns a steering vector that pushes self away from nearby
// enemies, stronger the closer they are, so a group spreads out instead of
// overlapping on the orbit.
func (g *Game) separation(self *entity) (float64, float64) {
	var sx, sy float64
	for i := range g.entities {
		other := &g.entities[i]
		if other == self || other.kind != kindEnemy {
			continue
		}
		dx, dy := self.x-other.x, self.y-other.y
		d := math.Hypot(dx, dy)
		if d == 0 || d >= enemySepDist {
			continue
		}
		w := (enemySepDist - d) / enemySepDist // 1 when touching, 0 at the edge
		sx += dx / d * w
		sy += dy / d * w
	}
	return sx * enemySepWeight, sy * enemySepWeight
}

// moveEnemy applies a velocity to an enemy one axis at a time so it slides along
// walls instead of passing through them (same rule as the player).
func (g *Game) moveEnemy(e *entity, dx, dy float64) {
	if !g.collidesAt(e.x+dx, e.y, e.radius) {
		e.x += dx
	}
	if !g.collidesAt(e.x, e.y+dy, e.radius) {
		e.y += dy
	}
}

// enemyTarget is what this ship hunts: the player in the campaign — its horde
// has no other prey — or the nearest ship of ANOTHER faction in skirmish, where
// the player ship does not exist and the factions prey on each other. ok is
// false when there is nothing left to hunt (the last foe just died): patrol.
func (g *Game) enemyTarget(e *entity) (tx, ty float64, ok bool) {
	if !g.skirmishMode {
		return g.x, g.y, true
	}
	best := -1
	bd := math.Inf(1)
	for i := range g.entities {
		o := &g.entities[i]
		if o.kind != kindEnemy || o.faction == e.faction || o.hp <= 0 {
			continue
		}
		d := (o.x-e.x)*(o.x-e.x) + (o.y-e.y)*(o.y-e.y)
		if d < bd {
			best, bd = i, d
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return g.entities[best].x, g.entities[best].y, true
}

// updateEnemies turns each enemy to face its target and fires at it on a
// cooldown. Position is left fixed for now; movement comes later.
func (g *Game) updateEnemies() {
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		if e.hitFlash > 0 {
			e.hitFlash--
		}
		if e.fireCD > 0 {
			e.fireCD--
		}

		// A driven ship is flown by its faction's external driver (IPC) or
		// its Filo program; the house brain below is what flies everyone
		// else — and any driver the moment it errors or goes silent.
		if d := g.ipcDrivers[e.faction]; d != nil && g.runIPCShip(d, e) {
			continue
		}
		if e.pilot != nil && g.runPilot(e) {
			continue
		}

		tx, ty, hunting := g.enemyTarget(e)
		if !hunting {
			if !e.stationary {
				g.patrol(e) // nothing left to hunt: wander
			}
			continue
		}
		dx, dy := tx-e.x, ty-e.y
		r := e.detectRange()
		inRange := dx*dx+dy*dy <= r*r
		los := inRange && g.lineOfSight(e.x, e.y, tx, ty)

		if !g.enemyEngaged(e, inRange, los) {
			if !e.stationary {
				g.patrol(e) // idle: wander instead of standing still
			}
			continue
		}

		dist := math.Hypot(dx, dy)
		if e.stationary {
			// Turret: never moves; only swings to track the target when in sight.
			if los {
				e.angle = turnToward(e.angle, math.Atan2(dy, dx)*180/math.Pi, enemyTurnRate)
			}
		} else {
			g.moveEnemyEngaged(e, tx, ty, dist, los)
		}

		// Fire only with a clear line of sight: it cannot shoot through walls.
		if e.fireCD > 0 || !los {
			continue
		}
		e.fireCD = e.fireInterval()
		g.enemyFire(e, tx, ty)
	}
}

// moveEnemyEngaged runs a mobile enemy's engaged movement: hold/orbit at the
// standoff when the target is in sight and close (it is in the open then), else
// navigate toward it by A* (heat-cost paths keep clear of walls) regardless of
// sight. It faces its aim/flight direction and breaks free if pinned at a corner.
func (g *Game) moveEnemyEngaged(e *entity, tx, ty, dist float64, los bool) {
	sd := e.standoff
	if sd <= 0 {
		sd = enemyStandoff
	}
	holding := g.shouldHold(tx, ty, dist, sd, los)

	bx, by := e.x, e.y
	if holding {
		g.orbitHold(e, tx, ty, dist, sd)
	} else {
		g.pursuePath(e, tx, ty)
	}
	moved := math.Hypot(e.x-bx, e.y-by)

	// Face the target to aim when visible, else face the flight direction.
	if los {
		e.angle = turnToward(e.angle, math.Atan2(ty-e.y, tx-e.x)*180/math.Pi, enemyTurnRate)
	} else if moved >= stuckEps {
		e.angle = turnToward(e.angle, math.Atan2(e.y-by, e.x-bx)*180/math.Pi, enemyTurnRate)
	}

	// Stuck detection: an enemy pinned against a wall while orbiting needs to break
	// free just like one stuck on a path.
	if moved < stuckEps {
		e.stuck++
		if e.stuck >= stuckLimit {
			g.unstick(e, tx, ty)
			e.stuck = 0
		}
	} else {
		e.stuck = 0
	}
}

// enemyEngaged updates and reports the enemy's combat state. It enters combat
// only when the player is within the base radar AND in sight (which also widens
// the radar); once in combat it stays alert and tracks the player through walls
// until the player leaves the widened radar entirely.
func (g *Game) enemyEngaged(e *entity, inRange, los bool) bool {
	if e.combat {
		if inRange {
			e.alertGrace = combatGrace // refresh while the player is in range
		} else {
			e.alertGrace-- // out of range: keep chasing for a grace period, then give up
			if e.alertGrace <= 0 {
				e.combat = false
			}
		}
		return e.combat
	}
	if inRange && los {
		e.combat = true
		e.alertGrace = combatGrace
		combat := radarRange * radarCombatMul
		if e.radar < combat {
			e.radar = combat
		}
	}
	return e.combat
}

// patrol moves an idle enemy along a heading it commits to for a while, so it
// cruises purposefully instead of jittering. It picks a new heading when the
// timer runs out, and turns away when it bumps into a wall.
func (g *Game) patrol(e *entity) {
	e.stuck = 0
	if e.wanderCD > 0 {
		e.wanderCD--
	} else {
		e.wanderHead += (g.simFloat()*2 - 1) * patrolTurnRange
		e.wanderCD = patrolHold + g.simIntN(patrolHold)
	}

	dx, dy := math.Cos(e.wanderHead), math.Sin(e.wanderHead)
	bx, by := e.x, e.y
	g.applyEnemyMove(e, dx, dy, patrolSpeed)
	if e.x == bx && e.y == by {
		// Blocked: turn 90-180 degrees and commit to the new heading.
		e.wanderHead += math.Pi/2 + g.simFloat()*math.Pi/2
		e.wanderCD = patrolHold
		e.vx, e.vy = 0, 0
		return
	}
	target := math.Atan2(e.y-by, e.x-bx) * 180 / math.Pi
	e.angle = turnToward(e.angle, target, enemyTurnRate)
}

// enemyFire spawns this hull's volley at the target's current position: one
// bolt, or a narrow fan of them once the ship has salvaged fire mods.
func (g *Game) enemyFire(e *entity, tx, ty float64) {
	dx, dy := tx-e.x, ty-e.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	aim := math.Atan2(dy, dx)
	bolts := 1 + e.fireMod
	spread := shipFanDegrees * math.Pi / 180
	for i := range bolts {
		// Centre the fan on the aim: one bolt flies straight, two straddle it.
		off := (float64(i) - float64(bolts-1)/2) * spread
		g.fireBolt(e, aim+off, tx, ty)
	}
	g.match.recordShot(e.faction)
	g.traceShot(e, tx, ty)
	// The enemy's own fire sound, when its asset declares one ("fire" has no
	// fallback on purpose: a full room of default pew-pew would swamp the mix).
	g.playEvent(e.a, "fire")
}

// fireBolt sends one bolt of this hull's volley along a heading in radians.
func (g *Game) fireBolt(e *entity, heading, tx, ty float64) {
	speed := e.shotSpeed
	if speed <= 0 {
		speed = enemyBulletSpeed
	}
	// The same render payload every other shot carries, so an enemy bolt is the
	// player's effect in another color rather than its own thing. A faction ship
	// fires in its faction's color: the bolt says whose it is at a glance.
	rcol, rglow := enemyShotColor, enemyShotGlowColor
	if e.faction > 0 {
		rcol, rglow = factionShotColors(e.faction)
	}
	g.enemyShots = append(g.enemyShots, projectile{
		x: e.x, y: e.y, px: e.x, py: e.y,
		vx:      math.Cos(heading) * speed,
		vy:      math.Sin(heading) * speed,
		life:    enemyBulletLife,
		dmg:     e.shotDmg,
		hullDmg: shipShotHullDamage + e.dmgMod, // what salvaged damage mods buy
		faction: e.faction,
		sid:     e.id,
		rcol:    rcol, rglow: rglow, width: bulletWidth, glowW: bulletGlowWidth,
	})
}

// detectRange is the enemy's current engage radius, defaulting to the base radar
// when unset (e.g. in tests that build an entity directly).
func (e *entity) detectRange() float64 {
	if e.radar <= 0 {
		return radarRange
	}
	return e.radar
}

// damageEnemy applies dmg to the enemy at index i, destroying it and scoring when
// its health runs out, and pops a floating damage number colored by its source. A
// surviving enemy widens its radar so it retaliates against fire from beyond range.
func (g *Game) damageEnemy(i, dmg int, col color.RGBA, by int) bool {
	e := &g.entities[i]
	if e.boss && g.escortsAlive() {
		// The boss is shielded until the room is cleared: fire glances off, no damage taken. The
		// spark tells the player their shots are wasted — kill the escorts first.
		e.hitFlash = hitFlashFrames
		g.emitBurst(e.x, e.y, shieldSparks)
		return false
	}
	// A salvaged shield takes the blow first, and a hull behind a shield that
	// held is not hurt at all.
	if e.shield > 0 {
		absorbed := min(e.shield, dmg)
		e.shield -= absorbed
		dmg -= absorbed
		g.emitBurst(e.x, e.y, shieldSparks)
		if dmg <= 0 {
			e.hitFlash = hitFlashFrames
			g.traceHit(e, by, 0)
			return false
		}
	}
	e.hp -= dmg
	g.spawnDamageNumber(e.x, e.y-e.radius, dmg, col)
	if e.hp > 0 {
		g.traceHit(e, by, dmg)
		e.hitFlash = hitFlashFrames
		e.radar = math.Min(e.detectRange()*radarHitBoost, radarRange*radarMaxMul)
		return false
	}
	g.traceDeath(e, by)
	x, y := e.x, e.y
	kind := ""
	if e.a != nil {
		kind = e.a.Kind // the archetype, for the loot table — read before the removal invalidates e
	}
	g.playEvent(e.a, "destroy") // before the removal invalidates e
	g.markConsumed(e.spawn)     // stays dead if the player revisits this map
	g.entities = append(g.entities[:i], g.entities[i+1:]...)
	g.score++
	g.emitExplosion(x, y)
	g.addShake(deathShake)
	g.maybeDropLoot(kind, x, y) // combat feeds the player
	g.revealBossIfClear()       // the last escort down drops the boss's shield, with a flourish
	return true
}

// spawnClearMargin is how much rock (beyond an entity's own radius) freeBuriedSpawns carves to
// un-bury it — enough to breach the clipping wall and rejoin the open region.
const spawnClearMargin = 24.0

// freeBuriedSpawns un-buries any enemy whose spawn point landed in solid rock. A procedural cave's
// cell centre can be clipped by a simplified (Douglas-Peucker) wall, so a guard placed there is
// embedded in rock: invisible on the map, unable to move, and impossible to reach — yet it still
// counts in enemiesLeft(), so a Rift room's cleared-> exit portal NEVER opens (a one-way dead end,
// the bug crg hit). Carving a small clearing rejoins it to the open region so it can move, be seen
// and be killed. Only runs where a region field exists (procedural + hand-made rock maps).
func (g *Game) freeBuriedSpawns() {
	if g.flood == nil {
		return
	}
	dug := false
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		if g.flood.rockAt(e.x, e.y) && g.flood.carve(e.x, e.y, e.radius+spawnClearMargin) {
			dug = true
		}
	}
	if dug {
		g.nav = buildNavgridFromFlood(g.flood) // enemies path through the fresh openings
	}
}

// escortsAlive reports whether any non-boss enemy is still alive. While it is true every boss on
// the map is shielded (see damageEnemy); the player must clear the room before a boss can be hurt.
func (g *Game) escortsAlive() bool {
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindEnemy && !e.boss {
			return true
		}
	}
	return false
}

// revealBossIfClear fires the shield-collapse flourish the moment the last escort dies, so the
// player sees the boss become vulnerable. A no-op while escorts remain or there is no boss (so it
// fires at most once per fight — the frame the room clears down to the boss alone).
func (g *Game) revealBossIfClear() {
	if g.escortsAlive() {
		return
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindEnemy && e.boss {
			g.emitBurst(e.x, e.y, shieldSparks)
			g.spawnShockwave(e.x, e.y, 120)
			g.logf("THE BOSS IS EXPOSED — its guard is down")
			return
		}
	}
}

// resolveContacts damages the player and an enemy when their hulls overlap, so
// ramming costs both. A brief i-frame keeps it from draining every frame.
func (g *Game) resolveContacts() {
	if g.skirmishMode || g.invuln > 0 || g.over {
		return // in skirmish there is no player hull: nothing rams a ghost
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		rr := g.radius + e.radius
		dx, dy := g.x-e.x, g.y-e.y
		if dx*dx+dy*dy > rr*rr {
			continue
		}
		g.damageEnemy(i, ramEnemyDamage, damageColor, 0)
		g.hurtPlayer(contactDamage)
		if g.invuln < contactInvuln {
			g.invuln = contactInvuln
		}
		return // one contact per frame; the player is now briefly invulnerable
	}
}

// enemiesLeft counts the live enemies.
func (g *Game) enemiesLeft() int {
	n := 0
	for i := range g.entities {
		if g.entities[i].kind == kindEnemy {
			n++
		}
	}
	return n
}
