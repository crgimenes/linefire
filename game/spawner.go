package game

import (
	"linefire/filoio"
	"math"
	"math/rand/v2"

	"linefire/asset"
	"linefire/level"
	"linefire/render"
)

// Horde spawner: a stage can declare a time-based enemy spawner (level.Horde) that
// drips enemies in around the player, holds at a live-count cap, and ramps the
// pressure over time. It reuses the enemy archetypes and the shared entity pool, so
// a wave is just more of the same enemies the AI already drives — and the render
// reprojection keeps a crowd cheap. Everything derives from the horde's seed, so
// wave timing is deterministic and testable without a window.

const (
	hordeDefaultInterval = 45  // frames between spawns when unset (~0.75s)
	hordeMinInterval     = 12  // interval floor as difficulty ramps
	hordeIntervalStep    = 3   // frames the interval drops per difficulty step
	hordeDefaultMaxAlive = 12  // live-enemy cap when unset
	hordeMaxAliveStep    = 2   // live cap growth per difficulty step
	hordeMaxAliveCap     = 40  // ceiling as the cap ramps
	hordeDefaultRamp     = 360 // frames per difficulty step when unset (~6s)
	hordeSpawnDist       = 460 // fallback spawn ring radius, world units
	hordeSpawnMargin     = 48  // world units past the view edge to spawn (off-screen)
	hordeSpawnClearance  = 16  // radius that must be wall-free at a spawn point
	hordeSpawnTries      = 10  // ring positions tried per spawn before giving up this frame
)

// horde is the live spawner state built from a level.Horde.
type horde struct {
	rng      *rand.Rand
	types    []string
	interval int // current (ramps down)
	maxAlive int // current (ramps up)
	ramp     int // frames per difficulty step (0 uses the default)
	cd       int // frames until the next spawn attempt
	elapsed  int
	assets   map[string]hordeAsset // enemy kind -> loaded asset + meshes (cached)
}

type hordeAsset struct {
	a    *asset.Asset
	mesh *render.Mesh
	glow *render.Mesh
}

// newHorde builds the spawner from a level config, filling defaults. Returns nil
// when the config declares no enemy types (disabled).
func newHorde(cfg level.Horde) *horde {
	if len(cfg.Types) == 0 {
		return nil
	}
	seed := uint64(cfg.Seed) // #nosec G115 -- reinterpreting the seed's bits is the intent
	h := &horde{
		// #nosec G404 -- deterministic wave timing, not a security boundary
		rng:      rand.New(rand.NewPCG(seed, 0x484f524445)), // stream = "HORDE"
		types:    append([]string(nil), cfg.Types...),
		interval: cfg.Interval,
		maxAlive: cfg.MaxAlive,
		ramp:     cfg.Ramp,
		assets:   map[string]hordeAsset{},
	}
	if h.interval <= 0 {
		h.interval = hordeDefaultInterval
	}
	if h.maxAlive <= 0 {
		h.maxAlive = hordeDefaultMaxAlive
	}
	if h.ramp <= 0 {
		h.ramp = hordeDefaultRamp
	}
	return h
}

// stepHorde advances the spawner one frame: ramps difficulty on schedule, then
// spawns one enemy when off cooldown and below the live cap.
func (g *Game) stepHorde() {
	h := g.horde
	if h == nil {
		return
	}
	if h.elapsed == 0 {
		g.logf("HORDE incoming")
	}
	h.elapsed++
	if h.elapsed%h.ramp == 0 {
		h.interval = max(hordeMinInterval, h.interval-hordeIntervalStep)
		if h.maxAlive < hordeMaxAliveCap {
			h.maxAlive += hordeMaxAliveStep
			g.logf("HORDE intensifies")
		}
	}
	if h.cd > 0 {
		h.cd--
		return
	}
	if g.enemiesLeft() >= h.maxAlive {
		return // density cap reached: hold until a slot frees
	}
	if g.spawnHordeEnemy() {
		h.cd = h.interval
	}
}

// spawnHordeEnemy places one enemy of a random configured kind on a ring around
// the player (varied so a clear spot is easy to find), at the first REACHABLE spot
// — one the enemy can actually navigate to the player from. Reports whether it
// spawned (a cramped frame may find none; it retries next tick).
func (g *Game) spawnHordeEnemy() bool {
	h := g.horde
	kind := h.types[h.rng.IntN(len(h.types))]
	maxDist := g.hordeSpawnDist()
	for range hordeSpawnTries {
		ang := h.rng.Float64() * 2 * math.Pi
		d := maxDist * (0.55 + 0.45*h.rng.Float64()) // vary the radius, not just the angle
		x := g.x + math.Cos(ang)*d
		y := g.y + math.Sin(ang)*d
		if !g.hordeReachable(x, y) {
			continue
		}
		ha := g.hordeAssetFor(kind)
		g.entities = append(g.entities, enemyEntity(kind, ha.a, ha.mesh, ha.glow, x, y, 0))
		return true
	}
	return false
}

// hordeReachable reports whether a horde enemy spawned at (x, y) could reach the
// player: in bounds, clear of walls, on a navigable cell, and with an actual A*
// path to the ship. This is the fix for enemies dripping OUTSIDE the map (past the
// walls or in a sealed pocket), where they could neither reach the player nor be
// reached — useless spawns.
func (g *Game) hordeReachable(x, y float64) bool {
	if !g.inBounds(x, y) || g.collidesAt(x, y, hordeSpawnClearance) {
		return false
	}
	if g.nav == nil {
		return true // no navgrid (a bare test/simple map): the bounds check is enough
	}
	cx, cy := g.nav.cellOf(x, y)
	if g.nav.isBlocked(cx, cy) {
		return false // too close to a wall to stand or move
	}
	return g.nav.findPath(x, y, g.x, g.y) != nil // must be able to path to the player
}

// hordeSpawnDist is the ring's outer radius: just past the visible edge so enemies
// arrive off-screen, but capped to the playable area so a spawn never aims outside
// the walls (a small map may force some spawns into view, which is fine).
func (g *Game) hordeSpawnDist() float64 {
	d := float64(hordeSpawnDist)
	if g.sw > 0 && g.camPixelScale() > 0 {
		d = 0.5*math.Hypot(float64(g.sw), float64(g.sh))/g.camPixelScale() + hordeSpawnMargin
	}
	if lim := 0.45 * math.Min(g.bounds.w(), g.bounds.h()); lim > 0 && d > lim {
		d = lim
	}
	return d
}

// inBounds reports whether a world point lies inside the map's bounding box.
func (g *Game) inBounds(x, y float64) bool {
	return x >= g.bounds.minX && x <= g.bounds.maxX && y >= g.bounds.minY && y <= g.bounds.maxY
}

// hordeAssetFor loads (and caches) the asset and meshes for an enemy kind, resolved
// as "<mapDir>/<kind>.json". A missing asset yields a nil-mesh marker enemy.
func (g *Game) hordeAssetFor(kind string) hordeAsset {
	h := g.horde
	if ha, ok := h.assets[kind]; ok {
		return ha
	}
	ha := hordeAsset{}
	a, err := filoio.LoadAssetFS(g.content, g.mapDir, kind)
	if err == nil {
		ha.a = a
		ha.mesh = render.BuildLayersMesh(a.Layers)
		ha.glow = render.BuildGlowMesh(a.Layers)
	}
	h.assets[kind] = ha
	return ha
}

// enemyEntity builds a live enemy from its archetype at a world position. Shared by
// buildEntities and the horde spawner so both make identical enemies. spawn is -1:
// a spawner enemy is not a level spawn, so it is not tracked for per-map
// persistence.
func enemyEntity(kind string, a *asset.Asset, mesh, glow *render.Mesh, x, y, angle float64) entity {
	arch := archetypeFor(kind)
	orbit := 1.0
	if randFloat() < 0.5 {
		orbit = -1.0
	}
	return entity{
		kind: kindEnemy, align: alignBad, x: x, y: y, angle: angle,
		hp: arch.hp, radar: arch.radar,
		stationary: arch.stationary, fireEvery: arch.fireEvery,
		shotDmg: arch.shotDmg, shotSpeed: arch.shotSpeed, speedMul: arch.speedMul,
		a: a, mesh: mesh, glowMesh: glow, radius: assetRadius(a),
		standoff:   arch.standoff * (0.8 + randFloat()*0.5),
		wanderHead: angle * math.Pi / 180,
		orbitDir:   orbit,
		spawn:      -1,
	}
}
