package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"linefire/level"
)

// Winning the game is the campaign's terminal state, distinct from a per-level clear: a
// level's (resolution "cleared" "win") fires when its last enemy dies, freezing the run
// on a victory screen. Unlike the results overlay, there is no "keep flying" — the game
// is finished. The RTA clock (runTicks) is the whole-campaign speedrun time.

// stageTitleFrames is how long the stage-title flash lingers on entering a level (~2.5s).
const stageTitleFrames = 150

// levelResult is one line of the campaign summary: a stage the player passed through,
// with the time spent on it and how many of its objectives were met.
type levelResult struct {
	title    string
	ticks    int
	objDone  int
	objTotal int
}

// levelTitle is the stage's display name: its homage Title if it has one, else the plain
// file stem. Titles are the nostalgic hook — a stage can be "River Raid 2600".
func (g *Game) levelTitle() string {
	if g.level != nil && g.level.Title != "" {
		return g.level.Title
	}
	return g.mapName
}

// recordLevelResult appends the current stage's contribution to the run summary: its
// title, the time spent on it, and objectives met. Called when a stage is left (enterMap)
// and when the finale is won, so the victory screen can break the run down per screen.
func (g *Game) recordLevelResult() {
	if g.level == nil {
		return
	}
	done := 0
	for i := range g.objectives {
		if g.objectives[i].done {
			done++
		}
	}
	g.runLog = append(g.runLog, levelResult{
		title:    g.levelTitle(),
		ticks:    max(g.runTicks-g.levelStartTicks, 0),
		objDone:  done,
		objTotal: len(g.objectives),
	})
}

// winGame latches the victory the moment the final map is cleared. It logs the finale in
// the run summary, captures the total run time for the frozen screen, and silences the
// loops (the fanfare plays over quiet, not over an engine hum).
func (g *Game) winGame() {
	if g.gameWon || g.endKind != endNone {
		return
	}
	g.recordLevelResult()     // the finale itself is not left via a portal, so log it here
	g.clearTicks = g.runTicks // the full-campaign RTA time
	if g.sfx != nil {
		g.sfx.stopLoops()
		g.sfx.stopMusic()
		g.sfx.play(soundReq{"clear_warm", 808, 0.7}) // a rising victory chord
	}
	g.logf("VICTORY  %s", formatRunTime(g.clearTicks))
	g.sealReturnPortals() // the trap closes: kill any way back, leaving only the boss's Rift portal
	g.beginEnding(endWin) // hold the victory screen so the final effects play out
}

// sealReturnPortals removes every portal EXCEPT the one leading into the Rift ("@rift"). Called
// when the finale boss falls: the return-to-stock-up portal vanishes, so continuing is one-way.
func (g *Game) sealReturnPortals() {
	kept := g.entities[:0]
	for _, e := range g.entities {
		if e.kind == kindPortal {
			dst, _ := parsePortalTarget(e.target)
			if dst != riftMapName {
				continue // drop it: no retreat once the boss is dead
			}
		}
		kept = append(kept, e)
	}
	g.entities = kept
}

// updateVictory owns input while the victory screen is up: Space/Enter CONTINUES the run (the
// boss's portal into the Rift is waiting in the room), C rolls the credits, R replays from the
// first map.
func (g *Game) updateVictory() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.restartCampaign()
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.enterCreditsReturning() // Esc will come back to this victory screen
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) || touchJustTapped() {
		g.resumeAfterWin() // keep flying: the boss's portal into the Rift is open where it fell
	}
	return nil
}

// resumeAfterWin dismisses the victory screen and un-freezes the finale room so the player can keep
// going. The lair now SWARMS — wave after wave — with the boss's Rift portal (opened where it fell)
// the only way onward; the return portal was already sealed when the boss died. Hold and score, or
// dive into the one-way Rift.
func (g *Game) resumeAfterWin() {
	g.gameWon = false
	g.horde = newHorde(level.Horde{
		Types: []string{"enemy", "rusher", "sniper", "tank"}, Interval: 40, MaxAlive: 14, Ramp: 360, Seed: 3,
	})
	g.logf("THE LAIR SWARMS — into the rift, or hold the line for score")
}

// restartCampaign reloads the first map for a fresh run. Falling back to the current
// level keeps a lone-map build (or a missing start file) playable rather than stuck —
// but that fallback is exactly the "R returned me to the last screen" symptom, so a
// failed reload is logged rather than swallowed, to leave a trace if it recurs.
func (g *Game) restartCampaign() {
	lvl, name := g.level, g.mapName
	warn := ""
	if g.startMap != "" && g.startMap != g.mapName {
		loaded, err := g.loadMapByName(g.startMap)
		if err == nil {
			lvl, name = loaded, g.startMap
		} else {
			warn = "RESTART  could not load start map " + g.startMap + " — staying here"
		}
	}
	mapDir, startMap := g.mapDir, g.startMap
	sfx := g.sfx // the audio context is a process singleton: carry it over
	*g = *newWithContent(g.content, g.player, lvl, mapDir, false)
	g.mapName, g.startMap, g.sfx = name, startMap, sfx
	if g.sfx != nil {
		g.sfx.demoMute = false // a fresh run is audible; drop any lingering credits-demo mute
	}
	if warn != "" {
		g.logf("%s", warn) // into the fresh feed, so a recurrence leaves a visible trace
	}
}

// victoryLines is the text block for the victory screen: a per-stage breakdown (title,
// time, objectives) followed by the campaign totals.
func (g *Game) victoryLines() []string {
	lines := []string{"VICTORY", "", "the campaign is complete", ""}
	for _, r := range g.runLog {
		lines = append(lines, victoryRow(r))
	}
	lines = append(lines,
		"",
		fmt.Sprintf("TOTAL  %s        SCORE  %d", formatRunTime(g.clearTicks), g.score),
		"",
		"Space: continue into the Rift    C: credits    R: play again",
	)
	return lines
}

// victoryRow formats one stage's summary line for the fixed 6px HUD font. drawCenterLines
// centers each line by its BYTE length, so the columns only line up if every row is the
// same length — hence the fixed-width fields, and ASCII only (a multibyte glyph would
// both mis-center the row and not render in the debug font anyway).
func victoryRow(r levelResult) string {
	obj := "   -   " // survival stage: no clear objective
	if r.objTotal > 0 {
		obj = fmt.Sprintf("OBJ %d/%d", r.objDone, r.objTotal)
	}
	return fmt.Sprintf("%-22.22s %8s   %-7.7s", r.title, formatRunTime(r.ticks), obj)
}
