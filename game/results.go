package game

import (
	"github.com/crgimenes/linefire/filoio"
)

// Clearing a level is a NON-BLOCKING beat: the moment every objective is met, the run
// records the clear time (and any new best) and flashes a HUD banner that fades on its
// own — the game never stops. Map loads are near-instant, so pausing on a full-screen
// results card only broke the sense of continuity. Winning the whole campaign is the
// one exception: that freezes on the victory screen (see victory.go).

// clearBannerFrames is how long the level-clear banner lingers (~4s at 60 TPS).
const clearBannerFrames = 240

// syncLevelCleared latches the clear state when ENTERING a map: an already-complete map
// (revisited after being cleared, its progress restored from mapState) arrives latched,
// so the clear does NOT re-fire — that would log a ~0s clear time as a record and flash
// the banner again. A freshly-entered, still-incomplete map unlatches, so finishing it
// this visit still counts. Call after the map is built and its saved state applied.
func (g *Game) syncLevelCleared() {
	g.levelCleared = g.objectivesComplete()
}

// checkLevelClear latches the win the first frame the objectives are all met, records
// the clear time and the best-time record, and raises the non-blocking banner.
func (g *Game) checkLevelClear() {
	if g.levelCleared || !g.objectivesComplete() {
		return
	}
	g.levelCleared = true

	g.clearTicks = max(g.runTicks-g.levelStartTicks, 0)

	g.bestTicks, g.newRecord = g.clearTicks, false
	if g.sfx != nil && g.sfx.cfgPath != "" {
		best, record, _ := filoio.RecordBestTime(g.sfx.cfgPath, g.mapName, g.clearTicks)
		g.bestTicks, g.newRecord = best, record
	}

	g.clearBannerTicks = clearBannerFrames
	note := "LEVEL CLEAR  " + formatRunTime(g.clearTicks)
	if g.newRecord {
		note += "  * NEW RECORD *"
	}
	g.logf("%s", note)
	g.sfx.play(soundReq{"clear_warm", 707, 0.6}) // a short fanfare, over the still-running game
}

// clearBannerLines is the text shown while the level-clear banner is up.
func (g *Game) clearBannerLines() []string {
	head := "LEVEL CLEAR"
	if g.newRecord {
		head = "LEVEL CLEAR   * NEW RECORD *"
	}
	return []string{head, "TIME  " + formatRunTime(g.clearTicks) + "    BEST  " + formatRunTime(g.bestTicks)}
}
