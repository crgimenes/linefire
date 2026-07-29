package filoio

import (
	"context"
	"fmt"
	"strings"

	"github.com/crgimenes/filo"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// The procedural chunk document — a level's walls and spawns without any of the
// authored furniture, keyed by grid coordinate:
//
//	(chunk 0 -1
//	  (wall "gen" (stroke "#80ffff") (width 2) (fill "transparent") (glow 0.8)
//	    (path (move 0 0) (line 100 0) (close)))
//	  (spawn "e0_0_1" "rusher" 40 -60 -90 (kind "rusher")))
//
// It is a CACHE, regenerated from the seed whenever a file is missing, so it carries no
// version: a grammar change just invalidates the directory.
//
// The codec speaks in fields rather than in a struct, so procgen keeps owning its Chunk
// type and filoio does not have to import it (which would close a cycle).

// ExtChunk is the file extension for a persisted procedural chunk.
const ExtChunk = ".lfc"

// chunkBuiltins are the forms a chunk needs beyond the shape grammar.
func chunkBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"wall":   biTagged("wall"),
		"spawn":  biTagged("spawn"),
		"kind":   biTagged("kind"),
		"target": biTagged("target"),
	}
}

// ParseChunk evaluates Filo source into one generated tile.
func ParseChunk(src string) (cx, cy int, walls []asset.Layer, spawns []level.Spawn, err error) {
	var found bool
	var decodeErr error

	bi := chunkBuiltins()
	bi["chunk"] = func(_ context.Context, args []filo.Value) (filo.Value, error) {
		found = true
		cx, cy, walls, spawns, decodeErr = decodeChunk(args)
		return filo.VNum(0), nil
	}

	f, err := newEngine(shapeBuiltins(), bi)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	defer f.Close()

	err = f.DoString(src)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if decodeErr != nil {
		return 0, 0, nil, nil, decodeErr
	}
	if !found {
		return 0, 0, nil, nil, fmt.Errorf("no (chunk ...) form found")
	}
	return cx, cy, walls, spawns, nil
}

// decodeChunk walks the tagged tree of a (chunk cx cy ...) form.
func decodeChunk(args []filo.Value) (int, int, []asset.Layer, []level.Spawn, error) {
	if len(args) < 2 {
		return 0, 0, nil, nil, fmt.Errorf("chunk: want (chunk cx cy ...)")
	}
	coord, err := numbers(args[:2])
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("chunk: %w", err)
	}
	var walls []asset.Layer
	var spawns []level.Spawn
	for _, v := range args[2:] {
		tag, items, ok := tagOf(v)
		if !ok {
			return 0, 0, nil, nil, fmt.Errorf("chunk: unexpected element")
		}
		switch tag {
		case "wall":
			w, err := decodeLayer("wall", items)
			if err != nil {
				return 0, 0, nil, nil, err
			}
			walls = append(walls, w)
		case "spawn":
			s, err := decodeSpawn(items)
			if err != nil {
				return 0, 0, nil, nil, err
			}
			spawns = append(spawns, s)
		default:
			return 0, 0, nil, nil, fmt.Errorf("chunk: unknown element %q", tag)
		}
	}
	return int(coord[0]), int(coord[1]), walls, spawns, nil
}

// EmitChunk renders one generated tile as Filo text that ParseChunk reads back.
func EmitChunk(cx, cy int, walls []asset.Layer, spawns []level.Spawn) string {
	var sb strings.Builder
	b := builder{&sb}
	b.printf("(chunk %d %d\n", cx, cy)
	for _, w := range walls {
		emitLayer(b, "  ", "wall", w)
	}
	for _, s := range spawns {
		emitSpawn(b, s)
	}
	b.printf(")\n")
	return sb.String()
}
