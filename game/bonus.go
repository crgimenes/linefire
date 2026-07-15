package game

import "linefire/procgen"

// Bonus rooms: an OPTIONAL portal in an authored level ("@bonus" or "@bonus:returnTarget")
// warps to a procedurally generated cave (procgen.GenCaveRoom) full of loot and a few guards.
// The room's exit portal returns to returnTarget, so the detour rejoins the campaign — the
// player can take the normal way forward OR dip into the bonus for extra, random loot. Each
// dip generates a FRESH cave (the seed mixes the run clock), so the reward for exploring is a
// surprise rather than the same room every run.

// bonusMapName is the reserved name the runtime uses while inside a generated bonus room.
const bonusMapName = "@bonus"

// enterBonus generates and enters the bonus cave for the current map. returnTarget is where
// the room's exit portal sends the player back; blank defaults to the source map's from_bonus
// entry, so the authored level only needs a "from_bonus" entry to receive the returning ship.
func (g *Game) enterBonus(returnTarget string) {
	if returnTarget == "" {
		returnTarget = g.mapName + ":from_bonus"
	}
	lvl := procgen.GenCaveRoom(g.bonusSeed(), returnTarget)
	lvl.Music = g.randomTrack() // procedural rooms carry no authored theme; give them a random one
	g.enterMap(lvl, bonusMapName, "", false)
}

// bonusSeed mixes the source map name (FNV-1a) with the run clock, so each time the player dips
// into a bonus it is a different cave, while a given entry stays reproducible.
func (g *Game) bonusSeed() int64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(g.mapName); i++ {
		h ^= uint64(g.mapName[i])
		h *= 1099511628211
	}
	h ^= uint64(g.runTicks) // #nosec G115 -- run-clock nonce; the wraparound is intentional
	h *= 1099511628211
	return int64(h) // #nosec G115 -- hashing into a seed; the wraparound is intentional
}
