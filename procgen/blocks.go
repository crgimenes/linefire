package procgen

import (
	"fmt"
	"math"
	"math/rand/v2"

	"linefire/asset"
	"linefire/level"
)

// enemyArchKinds are the enemy archetype Kinds the generator scatters; each maps to
// a gameassets/<kind>.json asset and to a runtime archetype.
var enemyArchKinds = []string{"enemy", "turret", "rusher", "sniper", "tank"}

// blockRect is an axis-aligned wall block in world coordinates.
type blockRect struct{ x, y, w, h float64 }

// GenBlocks is the default chunk algorithm: a mostly-open arena with a clear border
// ring (so neighbouring chunks always connect along the shared edge) and a clear
// central disc (a safe spawn / crossing lane), with scattered rectangular wall
// blocks and a few enemies in the open. Fully deterministic in (seed, cx, cy).
func GenBlocks(seed int64, cx, cy int) Chunk {
	s1, s2 := chunkSeed(seed, cx, cy)
	// #nosec G404 -- deterministic procedural generation, not cryptographic
	rng := rand.New(rand.NewPCG(s1, s2))

	ox, oy := float64(cx)*ChunkSize, float64(cy)*ChunkSize
	margin := ChunkSize * 0.18 // clear border ring -> chunks always connect
	lo, hi := margin, ChunkSize-margin
	ccx, ccy := ChunkCenter(cx, cy)
	clearR := ChunkSize * 0.14 // clear central disc -> safe spawn / crossing

	var blocks []blockRect
	var paths []asset.Path
	nBlocks := 3 + rng.IntN(5) // 3..7
	for range nBlocks {
		bw := ChunkSize*0.08 + rng.Float64()*ChunkSize*0.16
		bh := ChunkSize*0.08 + rng.Float64()*ChunkSize*0.16
		bx := ox + lo + rng.Float64()*(hi-lo-bw)
		by := oy + lo + rng.Float64()*(hi-lo-bh)
		if math.Hypot(bx+bw/2-ccx, by+bh/2-ccy) < clearR+math.Max(bw, bh)/2 {
			continue // do not block the central disc
		}
		blocks = append(blocks, blockRect{bx, by, bw, bh})
		paths = append(paths, rectPath(bx, by, bw, bh))
	}

	var spawns []level.Spawn
	nEnemies := 2 + rng.IntN(4) // 2..5
	for i := range nEnemies {
		ex, ey, ok := 0.0, 0.0, false
		for range 16 {
			ex = ox + lo + rng.Float64()*(hi-lo)
			ey = oy + lo + rng.Float64()*(hi-lo)
			if math.Hypot(ex-ccx, ey-ccy) < clearR {
				continue
			}
			if pointInAnyBlock(ex, ey, blocks, 12) {
				continue
			}
			ok = true
			break
		}
		if !ok {
			continue
		}
		kind := enemyArchKinds[rng.IntN(len(enemyArchKinds))]
		spawns = append(spawns, level.Spawn{
			Name:  fmt.Sprintf("e%d_%d_%d", cx, cy, i),
			Asset: kind,
			Kind:  kind,
			X:     math.Round(ex),
			Y:     math.Round(ey),
			Angle: -90,
		})
	}

	return Chunk{
		CX: cx, CY: cy,
		Walls: []asset.Layer{{
			Name: "walls", Stroke: "#80ffff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8, Paths: paths,
		}},
		Spawns: spawns,
	}
}

// pointInAnyBlock reports whether (x, y) lies inside any block grown by pad on
// every side (so enemies do not spawn touching a wall).
func pointInAnyBlock(x, y float64, blocks []blockRect, pad float64) bool {
	for _, b := range blocks {
		if x >= b.x-pad && x <= b.x+b.w+pad && y >= b.y-pad && y <= b.y+b.h+pad {
			return true
		}
	}
	return false
}

// rectPath builds a closed rectangle wall path at (x,y) with size (w,h), rounded to
// whole units so the saved JSON stays clean.
func rectPath(x, y, w, h float64) asset.Path {
	x, y, w, h = math.Round(x), math.Round(y), math.Round(w), math.Round(h)
	return asset.Path{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: x, Y: y},
		{Op: asset.OpLineTo, X: x + w, Y: y},
		{Op: asset.OpLineTo, X: x + w, Y: y + h},
		{Op: asset.OpLineTo, X: x, Y: y + h},
		{Op: asset.OpClose},
	}}
}
