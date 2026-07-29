package game

import "github.com/crgimenes/linefire/filoio"

// End-of-run pacing. A death or a campaign win used to freeze onto its screen the same frame it
// happened, cutting off the payoff — the DEVOURER collapsing, the shot that killed you, the last
// enemy popping. Instead the outcome is QUEUED: the world keeps simulating its EFFECTS (no player
// control, no fresh damage, no new triggers) for endDelayFrames, then the screen appears. The
// time/record lines are already logged to the feed at the trigger, so they read during the wait.

// beginEnding queues the death or victory screen behind the delay. First one wins; a death during
// the win wait (or vice-versa) is ignored.
func (g *Game) beginEnding(kind int) {
	if g.endKind != endNone || g.over || g.gameWon {
		return
	}
	g.endKind = kind
	g.endTicks = endDelayFrames
	if kind == endDeath {
		g.shipDeathBlast()
	}
}

// shipDeathBlast is the player ship's destruction: a big multi-layer explosion, a shockwave, a
// hard shake and the boom — louder and wider than an enemy pop, so it reads as YOUR ship going
// up. The engine hum and music cut to silence, which the explosion punches through.
func (g *Game) shipDeathBlast() {
	g.emitExplosion(g.x, g.y)                 // streaks + glowing chunks
	g.emitBurst(g.x, g.y, missileBlast)       // a wide, fast spray
	g.emitBurst(g.x, g.y, missileBlastChunks) // heavy tumbling debris
	g.spawnShockwave(g.x, g.y, 200)
	g.addShake(deathShake * 2.4)
	g.playEvent(g.player, "destroy") // the explosion boom
	if g.sfx != nil {
		g.sfx.stopLoops()
		g.sfx.stopMusic() // silence reads as defeat
	}
}

// stepEnding runs one frame of the aftermath, then latches the queued screen when the delay ends.
// It sits ahead of the run clock in Update, so the RTA stays frozen at the captured time.
func (g *Game) stepEnding() error {
	g.endTicks--
	if g.endTicks <= 0 {
		g.finalizeScore() // a finished run's score counts toward the arcade HI, win or lose
		switch g.endKind {
		case endWin:
			g.gameWon = true
		case endDeath:
			g.over = true
			g.overIdle = 0 // start the attract-return countdown fresh
		}
		g.endKind = endNone
		return nil
	}
	g.prevX, g.prevY, g.prevAngle = g.x, g.y, g.angle // the ship is frozen: no motion-blur smear
	g.stepEndingEffects()
	return nil
}

// finalizeScore records the run's score as the arcade HI when it beats the session best,
// flagging g.newHighScore for the end screen and persisting it (read-modify-write, so it
// never wipes audio settings or best times). The HI lives on the soundBank, which survives
// every world reset, so it carries across a restart. No-op headless (no sfx).
func (g *Game) finalizeScore() {
	if g.sfx == nil || g.score <= g.sfx.highScore {
		return
	}
	g.sfx.highScore = g.score
	g.newHighScore = true
	if g.sfx.cfgPath != "" {
		_, _, _ = filoio.RecordHighScore(g.sfx.cfgPath, g.score)
	}
}

// stepEndingEffects advances only the world's motion and cosmetics — enemies, bullets, mines, the
// black hole, particles, shockwaves, the log. It deliberately omits player input, pickups, portals
// and the win/lose checks, so nothing new happens while the final moment plays.
func (g *Game) stepEndingEffects() {
	g.stepProjectiles()
	g.stepAllies()
	g.updateEnemies()
	g.stepMines()
	g.stepDevourer()
	g.stepEnemyShots()
	g.stepParticles()
	g.stepShockwaves()
	g.stepFloaters()
	g.stepLog()
	g.decayShake()
}
