package filoio

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// canonical drops the distinction between a nil slice and an empty one, which carries
// no information but which reflect.DeepEqual reports. The JSON encoder wrote
// "hardpoints": [] for an asset with none; Filo simply omits the form. Everything else
// stays untouched, so a genuinely lost field still fails the comparison.
func canonical(a *asset.Asset) *asset.Asset {
	c := a.Clone()
	nilIf := func(n int, set func()) {
		if n == 0 {
			set()
		}
	}
	nilIf(len(c.Layers), func() { c.Layers = nil })
	nilIf(len(c.Hardpoints), func() { c.Hardpoints = nil })
	nilIf(len(c.Collisions), func() { c.Collisions = nil })
	nilIf(len(c.Sounds), func() { c.Sounds = nil })
	nilIf(len(c.Tags), func() { c.Tags = nil })
	for i := range c.Layers {
		l := &c.Layers[i]
		nilIf(len(l.Paths), func() { l.Paths = nil })
		for j := range l.Paths {
			p := &l.Paths[j]
			nilIf(len(p.Commands), func() { p.Commands = nil })
			for k := range p.Commands {
				cmd := &p.Commands[k]
				nilIf(len(cmd.Ctrl), func() { cmd.Ctrl = nil })
			}
		}
	}
	return c
}

// TestAssetRoundTripsEveryRealAsset is the contract the editor rests on. For every asset
// the game actually ships, it proves file -> model -> Filo -> model is the identity. A
// lossy Save silently destroys an author's work and nothing else in the pipeline would
// notice; this is the test that makes Save safe.
func TestAssetRoundTripsEveryRealAsset(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "gameassets", "*"+ExtAsset))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no assets to check: %v", err)
	}
	for _, p := range paths {
		want, err := LoadAsset(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		t.Run(filepath.Base(p), func(t *testing.T) {
			src := EmitAsset(want)
			got, err := ParseAsset(src)
			if err != nil {
				t.Fatalf("re-parsing what we emitted failed: %v\n--- emitted ---\n%s", err, src)
			}
			if !reflect.DeepEqual(canonical(want), canonical(got)) {
				t.Fatalf("round trip lost data\n--- emitted ---\n%s\n--- want %+v\n--- got  %+v", src, want, got)
			}
		})
	}
	if len(paths) < 23 {
		t.Fatalf("expected the whole asset library, only saw %d", len(paths))
	}
	t.Logf("round-tripped %d real assets", len(paths))
}

// TestEmitKeepsNumbersClean carries over a property the JSON encoder used to guard: an
// integer coordinate is written as "32", never "32.0" or "3.2e+01", so a hand edit and a
// diff both stay readable.
func TestEmitKeepsNumbersClean(t *testing.T) {
	a := asset.New()
	a.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 32, Y: 4},
		{Op: asset.OpLineTo, X: 12.5, Y: 1e7},
		{Op: asset.OpClose},
	}}}
	src := EmitAsset(a)
	for _, want := range []string{"(move 32 4)", "(line 12.5 1e+07)"} {
		if !strings.Contains(src, want) {
			t.Errorf("expected %q in:\n%s", want, src)
		}
	}
	// A close command carries no coordinates: there are none to carry.
	if !strings.Contains(src, "(close)") || strings.Contains(src, "(close 0 0)") {
		t.Errorf("close must be bare:\n%s", src)
	}
}

// TestAssetRejectsALevel locks a hole the migration closes. Fed a map, the JSON reader
// returned a valid, empty asset; the Filo reader cannot, because the document announces
// what it is in its very first form.
func TestAssetRejectsALevel(t *testing.T) {
	_, err := ParseAsset(`(level "map0001" (version 1) (size 512 512))`)
	if err == nil {
		t.Fatal("the asset reader must refuse a level document")
	}
}

// TestAssetGrammarByHand pins the surface an author (or an AI) has to write, so a
// refactor cannot quietly rename a form. It also covers what the shipped assets do not
// exercise: a cubic curve, a hidden layer, a triangle hitbox, presence-flag sounds.
func TestAssetGrammarByHand(t *testing.T) {
	src := `
(asset "probe"
  (version 2)
  (kind "enemy")
  (tags "flying" "armored")
  (size 64 48)
  (origin 32 24)
  (layer "hull"
    (stroke "#80ffff") (width 1.5) (fill "transparent") (glow 1.25)
    (path (move 0 0) (line 10 0) (quad 15 5 10 10) (cubic 8 12 2 12 0 10) (close))
    (path (move 4 4) (line 6 4)))
  (layer "guides" (stroke "#333333") (width 1) (fill "transparent") (glow 0) (hidden)
    (path (move 0 0) (line 1 1)))
  (hardpoint "muzzle" "weapon" 32 2 -90)
  (collision "triangle" 0 0 10 0 5 9)
  (collision "circle" 32 24 20)
  (sound "fire" "shot_zap" (seed 101) (volume 0.3))
  (sound "thruster" "engine" (continuous) (muted))
  (editor (snap) (snap-radius 8) (grid) (grid-size 4) (snap-to-grid)))
`
	a, err := ParseAsset(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if a.Name != "probe" || a.Kind != "enemy" || !reflect.DeepEqual(a.Tags, []string{"flying", "armored"}) {
		t.Fatalf("header wrong: %+v", a)
	}
	if len(a.Layers) != 2 || !a.Layers[1].Hidden {
		t.Fatalf("layers wrong: %+v", a.Layers)
	}
	cmds := a.Layers[0].Paths[0].Commands
	if len(cmds) != 5 || cmds[2].Op != asset.OpQuadTo || len(cmds[2].Ctrl) != 1 {
		t.Fatalf("quad wrong: %+v", cmds)
	}
	if cmds[3].Op != asset.OpCubicTo || len(cmds[3].Ctrl) != 2 || cmds[3].X != 0 || cmds[3].Y != 10 {
		t.Fatalf("cubic wrong (endpoint must come last): %+v", cmds[3])
	}
	if a.Collisions[0].Kind != asset.CollisionTriangle || len(a.Collisions[0].Points) != 3 {
		t.Fatalf("triangle wrong: %+v", a.Collisions[0])
	}
	if a.Collisions[1].Radius != 20 {
		t.Fatalf("circle radius wrong: %+v", a.Collisions[1])
	}
	if !a.Sounds[1].Continuous || !a.Sounds[1].Muted || a.Sounds[0].Seed != 101 {
		t.Fatalf("sounds wrong: %+v", a.Sounds)
	}
	if !a.Editor.SnapEnabled || !a.Editor.GridEnabled || !a.Editor.SnapToGrid || a.Editor.GridSize != 4 {
		t.Fatalf("editor wrong: %+v", a.Editor)
	}

	// And it survives its own emitter.
	again, err := ParseAsset(EmitAsset(a))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !reflect.DeepEqual(canonical(a), canonical(again)) {
		t.Fatalf("hand-written asset did not survive a round trip\n%s", EmitAsset(a))
	}
}

// TestAssetIsAProgramNotJustData is the whole reason crg chose the language over
// Marshal/Unmarshal: an exception can be COMPUTED in the file, with no new struct field
// and no new case in the engine.
func TestAssetIsAProgramNotJustData(t *testing.T) {
	src := `
(def w 64)
(def half (/ w 2))
(asset "computed"
  (version 2)
  (size w w)
  (origin half half)
  (layer "main" (stroke "#80ffff") (width 1) (fill "transparent") (glow 1)
    (path (move 0 0) (line w 0) (line w w) (line 0 w) (close)))
  (editor (snap) (snap-radius 8) (grid) (grid-size 8)))
`
	a, err := ParseAsset(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Size.W != 64 || a.Origin.X != 32 {
		t.Fatalf("inline arithmetic did not reach the model: %+v", a)
	}
	last := a.Layers[0].Paths[0].Commands[2]
	if last.X != 64 || last.Y != 64 {
		t.Fatalf("a bound symbol did not reach a path command: %+v", last)
	}
}

// TestAssetRejectsBadInput: a document that lies must fail loudly, naming the form.
func TestAssetRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"no asset form":  `(version 2)`,
		"unknown form":   `(asset "x" (version 2) (size 1 1) (origin 0 0) (colour "red"))`,
		"bad arity":      `(asset "x" (version 2) (size 1 1 1) (origin 0 0))`,
		"unknown hitbox": `(asset "x" (version 2) (size 1 1) (origin 0 0) (collision "blob" 1 2))`,
		"future version": `(asset "x" (version 99) (size 1 1) (origin 0 0))`,
		"unknown command": `(asset "x" (version 2) (size 1 1) (origin 0 0)
			(layer "l" (stroke "#fff") (width 1) (fill "transparent") (glow 0) (path (arc 1 2))))`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseAsset(src)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestSaveAssetRefusesInvalid: Save validates first, so a broken document never lands on
// disk and overwrites a good one.
func TestSaveAssetRefusesInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad"+ExtAsset)
	err := SaveAsset(path, &asset.Asset{Name: "bad"}) // no version, no layers
	if err == nil {
		t.Fatal("saving an invalid asset must fail")
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		t.Fatal("an invalid asset must not touch the disk")
	}
}

// TestSaveAssetWritesReadableFilo: a save must produce a file LoadAsset accepts.
func TestSaveAssetWritesReadableFilo(t *testing.T) {
	want := asset.New()
	path := filepath.Join(t.TempDir(), "ok"+ExtAsset)
	err := SaveAsset(path, want)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadAsset(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(canonical(want), canonical(got)) {
		t.Fatalf("save/load lost data\nwant %+v\ngot  %+v", want, got)
	}
	src, err := os.ReadFile(path) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(src), "(asset ") {
		t.Fatalf("a saved asset should open with the (asset ...) form, got %.40q", src)
	}
}
