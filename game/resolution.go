package game

import (
	"strings"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/render"
)

// Resolutions turn a met stage condition into an outcome, once: destroying every
// enemy can open the exit (warp like a portal), send the player back where they
// came from, or drop a reward on the map. Which resolutions already fired is part
// of the per-map run state, so revisiting a map does not replay its outcome.

// levelHasEnemies reports whether the level spawns any enemy, judged from the
// spawn categories (not the live entities, which per-map state may have culled).
func levelHasEnemies(lvl *level.Level) bool {
	if lvl == nil {
		return false
	}
	if lvl.Horde != nil && len(lvl.Horde.Types) > 0 {
		return true // a spawner map has enemies even before the first wave lands
	}
	for _, s := range lvl.Spawns {
		if spawnEntityKind(s.Kind) == kindEnemy {
			return true
		}
	}
	return false
}

// updateResolutions fires every pending resolution whose condition is met, in
// order. It reports whether a routine swapped the world (exit/return), so the
// caller must stop touching stale state this frame.
func (g *Game) updateResolutions() bool {
	if g.level == nil || g.creditsMode {
		return false // the attract demo is a spectacle, not a run: it never warps or wins
	}
	for i := range g.level.Resolutions {
		r := &g.level.Resolutions[i]
		if g.curMapState().resolved[i] {
			continue
		}
		if !g.resolutionMet(r.On) {
			continue
		}
		g.curMapState().resolved[i] = true
		if g.runResolution(r) {
			return true
		}
	}
	return false
}

// resolutionMet evaluates a condition name. "cleared" only counts on maps that
// actually spawned enemies, so an empty map does not resolve on arrival.
func (g *Game) resolutionMet(on string) bool {
	if on == level.OnCleared {
		return g.hadEnemies && g.enemiesLeft() == 0
	}
	return false
}

// runResolution executes a routine and reports whether it swapped the world.
func (g *Game) runResolution(r *level.Resolution) bool {
	switch r.Do {
	case level.DoSpawn:
		g.spawnReward(r.Target, r.X, r.Y)
		return false
	case level.DoPortal:
		g.spawnPortalEntity(r.Target, r.X, r.Y)
		return false
	case level.DoExit:
		return g.warpTo(r.Target)
	case level.DoReturn:
		if g.cameFrom == "" {
			return false // nowhere to go back to (the run started here)
		}
		return g.warpTo(g.cameFrom)
	case level.DoWin:
		g.winGame()
		return true // freeze the frame; the victory screen takes over next tick
	}
	return false
}

// warpTo leaves to "map" or "map:label" with portal semantics: the same map is a
// teleport in place (state kept), another map is loaded with the run carried over.
// A target that fails to load leaves the resolution spent but the player in place.
func (g *Game) warpTo(target string) bool {
	name, label := parsePortalTarget(target)
	if name == g.mapName {
		g.placeAtEntry(label)
		g.portalGrace = portalGraceFrames
		return false // same world, just moved
	}
	lvl, err := g.loadMapByName(name)
	if err != nil {
		return false
	}
	g.enterMap(lvl, name, label, false)
	return true
}

// spawnReward drops an asset (a power-up, loot) at a world position, with pickup
// feedback so the reward draws the eye. A missing asset still spawns a marker
// entity, like map spawns do.
func (g *Game) spawnReward(ref string, x, y float64) {
	a, err := filoio.LoadAssetFS(g.content, g.mapDir, ref)
	if err != nil {
		a = nil
	}
	var mesh, glow *render.Mesh
	kind := ""
	if a != nil {
		mesh = render.BuildLayersMesh(a.Layers)
		glow = render.BuildGlowMesh(a.Layers)
		kind = a.Kind
	}
	e := entity{
		align: alignGood, x: x, y: y,
		spawn: -1, // not from a level spawn: no per-map consumed slot
		hp:    pickupHP, a: a, mesh: mesh, glowMesh: glow, radius: assetRadius(a),
	}
	if key, isWeapon := strings.CutPrefix(kind, "weapon-"); isWeapon {
		e.kind, e.power = kindWeapon, key // a weapon drop joins the arsenal on pickup
	} else {
		e.kind, e.power = kindPowerUp, kind
		if e.power == "" {
			e.power = powerHeal
		}
	}
	g.entities = append(g.entities, e)
	g.emitBurst(x, y, pickupMotes)
	g.playEvent(a, "swap") // an arrival chord: something new is on the map
	g.logf("REWARD  dropped on the map")
}

// spawnPortalEntity tears a portal open at a world position, leading to target ("map[:label]" or a
// runtime name like "@rift"). Used by the DoPortal resolution — the boss's death portal, and the
// forward portal a Rift reveals on clear. A brief grace keeps the fresh portal from firing before
// the player has flown to it.
func (g *Game) spawnPortalEntity(target string, x, y float64) {
	a, err := filoio.LoadAssetFS(g.content, g.mapDir, "portal")
	if err != nil {
		a = nil // a portal draws as its own ring + particles, so a missing asset still works
	}
	var mesh, glow *render.Mesh
	if a != nil {
		mesh = render.BuildLayersMesh(a.Layers)
		glow = render.BuildGlowMesh(a.Layers)
	}
	r := assetRadius(a)
	if r < portalMinRadius {
		r = portalMinRadius // never leave a portal too small to fly into
	}
	// Carve a clearing so the portal is NEVER buried: a generated cave's exit cell can be clipped by
	// a simplified wall, which would open the portal inside solid rock — invisible and unreachable,
	// a dead end in the one-way Rift. The exit cell sits in the open region, so the clearing joins
	// the portal to reachable space. No-op where (x,y) is already open (an authored map's portal).
	if g.flood != nil {
		g.digAt(x, y, r+portalClearMargin)
	}
	g.entities = append(g.entities, entity{
		kind: kindPortal, align: alignAlly, x: x, y: y,
		spawn: -1, target: target, a: a, mesh: mesh, glowMesh: glow, radius: r,
	})
	g.emitBurst(x, y, pickupMotes)
	g.spawnShockwave(x, y, 90)
	g.portalGrace = portalGraceFrames
	g.logf("A PORTAL TEARS OPEN")
}
