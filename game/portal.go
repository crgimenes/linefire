package game

import (
	"image/color"
	"strings"

	"linefire/filoio"
	"linefire/level"
)

// portalGraceFrames is how long after a warp portals stay inert, so the player
// does not instantly re-trigger one near the arrival point.
const portalGraceFrames = 45

// portalMinRadius is the smallest trigger radius a runtime-spawned portal gets, so a portal opened
// by a resolution is always big enough to fly into even if its asset is missing or tiny.
const portalMinRadius = 24.0

// portalClearMargin is how much rock (beyond the portal's own radius) a materializing portal carves
// away, so there is a lane to fly into it even when it opens against a wall.
const portalClearMargin = 28.0

var (
	// portalAuthoredColor marks a portal that leads to a hand-authored stage (teal-blue).
	portalAuthoredColor = color.RGBA{0x50, 0xc8, 0xff, 0xff}
	// portalBonusColor marks a portal into a bonus/procedural room (amber = treasure).
	portalBonusColor = color.RGBA{0xff, 0xc0, 0x40, 0xff}
)

// portalStyle returns the accent colour and spiral spin (+1/-1) for a portal by its
// destination: a runtime target ("@bonus"/"@proc"...) reads as a bonus room, everything else
// as an authored stage. Colour tells the player where the portal goes before they take it.
func portalStyle(target string) (color.RGBA, float64) {
	if strings.HasPrefix(target, "@") {
		return portalBonusColor, -1
	}
	return portalAuthoredColor, 1
}

// parsePortalTarget splits a portal target "map" or "map:label" into the map name
// and the optional entry label.
func parsePortalTarget(target string) (mapName, label string) {
	before, after, ok := strings.Cut(target, ":")
	if ok {
		return before, after
	}
	return target, ""
}

// loadMapByName resolves a map name (file stem) to a level in the same directory
// as the current map and loads it. Maps reference each other by name, so they
// stay easy to keep track of.
func (g *Game) loadMapByName(name string) (*level.Level, error) {
	return filoio.LoadLevelFS(g.content, g.mapDir, name)
}

// placeAtEntry moves the player to a named entry of the current level (position
// and facing), or the PlayerStart when the label is empty or unknown.
func (g *Game) placeAtEntry(label string) {
	x, y, a := g.level.PlayerStart.X, g.level.PlayerStart.Y, g.level.PlayerStart.Angle
	e, ok := g.level.EntryByName(label)
	if ok {
		x, y, a = e.X, e.Y, e.Angle
	}
	g.x, g.y, g.angle = x, y, a
	g.prevX, g.prevY, g.prevAngle = x, y, a
}

// enterMap swaps the world to a new level while preserving run-wide progress
// (health, shield, lives, score) and the window/DPI layout. Per-map state (walls,
// entities, nav, fog) is rebuilt fresh, so each map is explored from scratch.
// This is how levels are "stacked": a portal carries the run into the next map.
// The player lands at the named entry (label), falling back to the PlayerStart.
func (g *Game) enterMap(lvl *level.Level, name, label string, simple bool) {
	g.recordLevelResult() // log the stage we are leaving for the campaign summary
	g.bindMapState()      // the map we are leaving keeps the tunnels it lost and the fog it lifted
	health, shield, lives, score := g.health, g.shield, g.lives, g.score
	dpr, sw, sh, winW, winH := g.dpr, g.sw, g.sh, g.winW, g.winH
	overlay := g.overlay
	mapDir := g.mapDir
	startMap := g.startMap
	runLog := g.runLog
	mapStates := g.mapStates
	arsenal, slotArsIdx := g.arsenal, g.slotArsIdx
	devourerAmmo := g.devourerAmmo
	digDepth := g.digDepth
	edgeReturn := g.edgeReturn
	endless := g.endless
	hasComputer, autoFire, fireLevel := g.hasComputer, g.autoFire, g.fireLevel
	rateLevel, damageLevel, seekLevel := g.rateLevel, g.damageLevel, g.seekLevel
	bubbleTime, reflectTime := g.bubbleTime, g.reflectTime
	godMode := g.godMode
	allies := g.allies
	cp := g.checkpoint
	runTicks := g.runTicks
	sfx, feed := g.sfx, g.log
	from := g.mapName // where a "return" resolution goes back to

	*g = *newWithContent(g.content, g.player, lvl, mapDir, simple)

	g.health, g.shield, g.lives, g.score = health, shield, lives, score
	g.dpr, g.sw, g.sh, g.winW, g.winH = dpr, sw, sh, winW, winH
	g.overlay = overlay
	g.mapDir = mapDir
	g.startMap = startMap
	g.runLog = runLog
	g.mapStates = mapStates
	g.arsenal, g.slotArsIdx = arsenal, slotArsIdx // the collected weapons + selected slots carry across maps
	g.devourerAmmo = devourerAmmo                 // DEVOURER charges follow the run (the live hole does not)
	g.digDepth = digDepth                         // how deep past the edges the run has dug (edge/rift difficulty)
	g.edgeReturn = edgeReturn                     // where an edge room's return portal leads
	g.endless = endless                           // once in the Rift, every screen stays one-way
	g.syncSlots()
	g.hasComputer, g.autoFire, g.fireLevel = hasComputer, autoFire, fireLevel   // run-wide upgrades carry across maps
	g.rateLevel, g.damageLevel, g.seekLevel = rateLevel, damageLevel, seekLevel // combat mods carry across maps too
	g.bubbleTime, g.reflectTime = bubbleTime, reflectTime                       // timed shields keep ticking across maps
	g.godMode = godMode                                                         // iddqd persists so you can fly through to far maps
	g.allies = allies                                                           // companions carry across maps
	g.checkpoint = cp                                                           // the last checkpoint follows the run
	g.runTicks = runTicks                                                       // the speedrun clock keeps running across maps
	g.sfx, g.log = sfx, feed                                                    // audio is a process singleton; the feed narrates the run
	g.cameFrom = from
	g.mapName = name
	// Landing on an authored (file-backed) map ends the dig chain: reset the depth and record this
	// as where a future edge room should return to. A generated "@" map keeps the carried values.
	if !strings.HasPrefix(name, "@") {
		g.digDepth, g.edgeReturn = 0, name
	}
	g.applyMapState()          // drop already-cleared spawns, restore finished objectives
	g.evictStaleProcMaps(name) // release the VRAM of procedural screens left behind (endless/edge chains)
	g.syncLevelCleared()       // a revisited, already-cleared map must not re-fire the results
	g.placeAtEntry(label)
	g.levelStartTicks = g.runTicks        // this level's clear time is measured from here
	g.snapAlliesToShip()                  // re-form them at the entry, not from the old map
	g.prewarmMusic()                      // render this map's themes now, not at the first engagement
	g.titleBannerTicks = stageTitleFrames // flash the new stage's homage title on arrival
	g.materializeShip()                   // particles rush in and coalesce the ship at the arrival point
	g.logf("WARP  %s", name)
}

// resolvePortals warps the player when they touch a portal. A target naming the
// current map teleports in place to the entry (no reload, so the map keeps its
// state); a different map is loaded with the run carried over. It reports whether
// the world was swapped, so the caller can stop using stale state this frame.
// Portals are inert during the grace period and when the target is empty or fails
// to load.
func (g *Game) resolvePortals() bool {
	if g.creditsMode {
		return false // the attract demo stays put; no warping out of the credits
	}
	if g.portalGrace > 0 {
		g.portalGrace--
		return false
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindPortal || e.target == "" {
			continue
		}
		rr := g.radius + e.radius
		dx, dy := g.x-e.x, g.y-e.y
		if dx*dx+dy*dy > rr*rr {
			continue
		}
		mapName, label := parsePortalTarget(e.target)
		if mapName == "" {
			continue
		}
		if mapName == procMapName {
			g.enterProcedural(e.target) // "@proc[:seed]" -> the chunked procedural world
			return true
		}
		if mapName == bonusMapName {
			g.enterBonus(label) // "@bonus[:returnTarget]" -> a generated bonus cave
			return true
		}
		if mapName == riftMapName {
			g.enterRift() // "@rift" -> the next endless one-way Rift screen
			return true
		}
		if mapName == g.mapName {
			g.placeAtEntry(label) // same map: shortcut to another point, no reload
			g.portalGrace = portalGraceFrames
			return false
		}
		lvl, err := g.loadMapByName(mapName)
		if err != nil {
			continue // missing/broken target: leave the portal inert this frame
		}
		g.enterMap(lvl, mapName, label, false)
		return true
	}
	return false
}
