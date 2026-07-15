// Package procgen generates the Linefire procedural map as a grid of chunks. A
// chunk is one square tile of the world (walls + spawns) produced deterministically
// from a world seed and the chunk coordinates, so the same chunk always looks the
// same — and once generated it is saved, so prior generation (and, later, server
// edits) is reused instead of regenerated. The map feels endless because the game
// only keeps the 3x3 neighbourhood around the player loaded, streaming new chunks in
// as the ship moves.
package procgen

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// ChunkSize is the world-unit side length of one chunk.
const ChunkSize = 1200.0

// Generator turns a seed and chunk coordinates into a chunk. The default is
// GenBlocks; swapping it is how different map algorithms are tried.
type Generator func(seed int64, cx, cy int) Chunk

// Chunk is one generated tile: walls and spawns already in WORLD coordinates
// (offset to the chunk's position), so chunks stitch together without translation.
type Chunk struct {
	CX, CY int
	Walls  []asset.Layer
	Spawns []level.Spawn
}

// World is a procedural map instance: a seed, a directory it persists chunks to,
// and the algorithm that generates them.
type World struct {
	Dir  string
	Seed int64
	Gen  Generator
}

// NewWorld returns a world that persists under dir and generates with GenBlocks.
func NewWorld(dir string, seed int64) *World {
	return &World{Dir: dir, Seed: seed, Gen: GenBlocks}
}

// ChunkOf returns the chunk coordinates containing world point (x, y).
func ChunkOf(x, y float64) (int, int) {
	return int(math.Floor(x / ChunkSize)), int(math.Floor(y / ChunkSize))
}

// ChunkCenter returns the world-space center of chunk (cx, cy).
func ChunkCenter(cx, cy int) (float64, float64) {
	return float64(cx)*ChunkSize + ChunkSize/2, float64(cy)*ChunkSize + ChunkSize/2
}

func (w *World) seedDir() string {
	return filepath.Join(w.Dir, fmt.Sprintf("seed_%d", w.Seed))
}

func (w *World) chunkPath(cx, cy int) string {
	return filepath.Join(w.seedDir(), fmt.Sprintf("%d_%d%s", cx, cy, filoio.ExtChunk))
}

// Get returns a chunk, loading it from disk when it was generated before (so prior
// generation and future server edits are reused) or generating and saving it.
func (w *World) Get(cx, cy int) (Chunk, error) {
	c, ok, err := w.load(cx, cy)
	if err != nil {
		return Chunk{}, err
	}
	if ok {
		return c, nil
	}
	c = w.Gen(w.Seed, cx, cy)
	c.CX, c.CY = cx, cy
	err = w.save(c)
	if err != nil {
		return Chunk{}, err
	}
	return c, nil
}

func (w *World) load(cx, cy int) (Chunk, bool, error) {
	// #nosec G304 -- path built from the trusted world dir + integer chunk coords
	data, err := os.ReadFile(w.chunkPath(cx, cy))
	if os.IsNotExist(err) {
		return Chunk{}, false, nil
	}
	if err != nil {
		return Chunk{}, false, err
	}
	var c Chunk
	c.CX, c.CY, c.Walls, c.Spawns, err = filoio.ParseChunk(string(data))
	if err != nil {
		return Chunk{}, false, err
	}
	return c, true, nil
}

func (w *World) save(c Chunk) error {
	data := []byte(filoio.EmitChunk(c.CX, c.CY, c.Walls, c.Spawns))
	err := os.MkdirAll(w.seedDir(), 0o750)
	if err != nil {
		return err
	}
	return os.WriteFile(w.chunkPath(c.CX, c.CY), data, 0o600)
}

// Ensure generates and saves the 3x3 neighbourhood around (cx, cy) — the chunk the
// player is in plus the eight around it — reusing any that already exist.
func (w *World) Ensure(cx, cy int) error {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			_, err := w.Get(cx+dx, cy+dy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Assemble stitches the 3x3 neighbourhood around (cx, cy) into one runnable level,
// with the player starting at the center of the (cx, cy) chunk. Walls from every
// chunk merge into one layer; spawns concatenate (all already in world coords).
func (w *World) Assemble(cx, cy int) (*level.Level, error) {
	lvl := level.New()
	lvl.Name = fmt.Sprintf("procedural %d,%d", cx, cy)
	lvl.Walls = []asset.Layer{{Name: "walls", Stroke: "#80ffff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8}}
	lvl.Spawns = nil
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			c, err := w.Get(cx+dx, cy+dy)
			if err != nil {
				return nil, err
			}
			for _, layer := range c.Walls {
				lvl.Walls[0].Paths = append(lvl.Walls[0].Paths, layer.Paths...)
			}
			lvl.Spawns = append(lvl.Spawns, c.Spawns...)
		}
	}
	sx, sy := ChunkCenter(cx, cy)
	lvl.PlayerStart = level.Start{X: sx, Y: sy, Angle: -90}
	lvl.Size = asset.Size{W: ChunkSize * 3, H: ChunkSize * 3}
	return lvl, nil
}

// chunkSeed derives two PCG seeds from the world seed and chunk coords so each
// chunk's generation is reproducible (and independent of its neighbours).
func chunkSeed(seed int64, cx, cy int) (uint64, uint64) {
	// #nosec G115 -- intentional wraparound: int coords hashed into uint seeds
	a := splitmix(uint64(seed)*0x9e3779b97f4a7c15 + uint64(uint32(cx)))
	// #nosec G115 -- intentional wraparound: int coords hashed into uint seeds
	b := splitmix(a ^ (uint64(uint32(cy))*0xc2b2ae3d27d4eb4f + 0x165667b19e3779f9))
	return a, b
}

// splitmix is the SplitMix64 finalizer, a fast high-quality integer hash.
func splitmix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
