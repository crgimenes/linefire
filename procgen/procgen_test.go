package procgen

import (
	"fmt"
	"os"
	"testing"

	"linefire/asset"
	"linefire/level"
)

// dump renders a value for comparison. It replaced a json.Marshal helper when the JSON
// encoder left the project; determinism is all these tests need from it.
func dump(t *testing.T, v any) string {
	t.Helper()
	return fmt.Sprintf("%#v", v)
}

func TestGenBlocksDeterministic(t *testing.T) {
	a := GenBlocks(42, 3, -2)
	b := GenBlocks(42, 3, -2)
	if dump(t, a) != dump(t, b) {
		t.Fatal("same seed+coords must generate an identical chunk")
	}
	c := GenBlocks(42, 3, -1) // different chunk
	if dump(t, a) == dump(t, c) {
		t.Fatal("different chunk coords should differ")
	}
}

func TestChunkOf(t *testing.T) {
	cases := []struct {
		x, y           float64
		wantCX, wantCY int
	}{
		{0, 0, 0, 0},
		{ChunkSize - 1, 1, 0, 0},
		{ChunkSize, ChunkSize, 1, 1},
		{-1, -ChunkSize - 1, -1, -2},
	}
	for _, c := range cases {
		cx, cy := ChunkOf(c.x, c.y)
		if cx != c.wantCX || cy != c.wantCY {
			t.Fatalf("ChunkOf(%v,%v) = (%d,%d), want (%d,%d)", c.x, c.y, cx, cy, c.wantCX, c.wantCY)
		}
	}
}

func TestBorderRingStaysClear(t *testing.T) {
	// Every wall vertex must sit inside the chunk's inner region, leaving a clear
	// border ring so adjacent chunks connect along the shared edge.
	c := GenBlocks(7, 0, 0)
	margin := ChunkSize * 0.18
	for _, layer := range c.Walls {
		for _, p := range layer.Paths {
			for _, cmd := range p.Commands {
				if cmd.Op == asset.OpClose {
					continue
				}
				if cmd.X < margin-0.5 || cmd.X > ChunkSize-margin+0.5 ||
					cmd.Y < margin-0.5 || cmd.Y > ChunkSize-margin+0.5 {
					t.Fatalf("wall vertex (%v,%v) intrudes into the clear border ring", cmd.X, cmd.Y)
				}
			}
		}
	}
}

func TestPersistenceReusesSavedChunk(t *testing.T) {
	w := NewWorld(t.TempDir(), 99)

	first, err := w.Get(2, 3) // generates + saves
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_, statErr := os.Stat(w.chunkPath(2, 3))
	if statErr != nil {
		t.Fatalf("chunk should be saved: %v", statErr)
	}

	// Swap in a generator that would produce something different; Get must still
	// return the SAVED chunk, proving reuse over regeneration.
	w.Gen = func(seed int64, cx, cy int) Chunk {
		return Chunk{CX: cx, CY: cy, Walls: []asset.Layer{{Name: "other"}}}
	}
	again, err := w.Get(2, 3)
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	if dump(t, first) != dump(t, again) {
		t.Fatal("an existing chunk must be reused from disk, not regenerated")
	}
}

func TestEnsureCreatesNeighborhood(t *testing.T) {
	w := NewWorld(t.TempDir(), 1)
	err := w.Ensure(0, 0)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			_, err := os.Stat(w.chunkPath(dx, dy))
			if err != nil {
				t.Fatalf("chunk (%d,%d) should exist after Ensure: %v", dx, dy, err)
			}
		}
	}
}

func TestAssembleValidLevel(t *testing.T) {
	w := NewWorld(t.TempDir(), 5)
	lvl, err := w.Assemble(0, 0)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	wx, wy := ChunkCenter(0, 0)
	if lvl.PlayerStart.X != wx || lvl.PlayerStart.Y != wy {
		t.Fatalf("player should start at the chunk center (%v,%v), got (%v,%v)", wx, wy, lvl.PlayerStart.X, lvl.PlayerStart.Y)
	}
	if len(lvl.Walls) == 0 || len(lvl.Walls[0].Paths) == 0 {
		t.Fatal("assembled level should have walls from the neighbourhood")
	}
	err = level.Validate(lvl)
	if err != nil {
		t.Fatalf("assembled procedural level should validate: %v", err)
	}
}
