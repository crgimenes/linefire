package game

import "testing"

// The loop recipes themselves (seamlessness, determinism) are tested in the sfx
// package; here we test the game-side loop driving.

func TestSetLoopAndStopLoopsNilSafe(t *testing.T) {
	var b *soundBank
	b.setLoop("thruster", soundReq{"engine", 700, 0.2}, true) // must not panic
	b.stopLoops()

	g := &Game{} // nil sfx: driving loops from state must be a no-op
	g.updateLoops(true)
}
