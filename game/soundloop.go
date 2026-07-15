package game

import (
	"bytes"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"linefire/sfx"
)

// Continuous sounds (the thruster hum, the laser beam) loop while their state is
// active instead of playing once. The seamless loop recipes live in the shared sfx
// package (the editors preview the same audio); this file owns the players.

// loopPCM returns (and caches) the stereo PCM for a continuous base, sharing the
// bank's rendered cache under a "loop:" key so one render serves the whole session.
func (b *soundBank) loopPCM(base string, seed int64) []byte {
	key := "loop:" + soundKey(base, seed)
	if vs, ok := b.rendered[key]; ok {
		if len(vs) == 0 {
			return nil
		}
		return vs[0]
	}
	samples := sfx.LoopSamples(base, seed)
	if samples == nil {
		b.rendered[key] = nil
		return nil
	}
	pcm := sfx.Stereo16(samples)
	b.rendered[key] = [][]byte{pcm}
	return pcm
}

// setLoop starts or stops the named continuous sound. Starting an already-running
// loop (or stopping a stopped one) is a no-op, so it is safe to drive from state
// every frame. Nil-safe (tests) and silent for unknown bases or muted audio.
func (b *soundBank) setLoop(name string, r soundReq, on bool) {
	if b == nil {
		return
	}
	p := b.loops[name]
	if !on || b.silent() {
		if p != nil {
			_ = p.Close()
			delete(b.loops, name)
		}
		return
	}
	if p != nil {
		return // already humming
	}
	pcm := b.loopPCM(r.base, r.seed)
	if pcm == nil {
		return
	}
	loop := audio.NewInfiniteLoop(bytes.NewReader(pcm), int64(len(pcm)))
	player, err := b.ctx.NewPlayer(loop)
	if err != nil {
		return // no player, no sound; the game plays on
	}
	rel := 1.0
	if r.vol > 0 && r.vol < 1 {
		rel = r.vol
	}
	player.SetVolume(b.master * rel)
	player.Play()
	if b.loops == nil {
		b.loops = map[string]*audio.Player{}
	}
	if b.loopVol == nil {
		b.loopVol = map[string]float64{}
	}
	b.loops[name] = player
	b.loopVol[name] = rel // remembered so a live master change re-levels the hum
}

// stopLoops silences every continuous sound (pause, game over, mute).
func (b *soundBank) stopLoops() {
	if b == nil {
		return
	}
	for name, p := range b.loops {
		_ = p.Close()
		delete(b.loops, name)
	}
}

// updateLoops drives the continuous sounds from this frame's state: the engine hums
// while the ship is thrusting, and the beam sings while the laser is held. The
// thruster sound comes from the player asset's `sounds` block (event "thruster",
// with a built-in fallback); the beam comes from the laser weapon's fire sound.
func (g *Game) updateLoops(thrusting bool) {
	req, ok := resolveEvent(g.player, "thruster")
	g.sfx.setLoop("thruster", req, thrusting && ok)
	g.sfx.setLoop("beam", weaponLaser.fire, g.laserOn)
}
