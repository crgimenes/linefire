package filoio

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crgimenes/filo"

	"github.com/crgimenes/linefire/asset"
)

// The asset document. The name is positional (like Filo's own (fn name ...)), every
// other field is a tagged form, and a bool is a PRESENCE flag — written only when true,
// so a file shows what is set rather than a wall of falses:
//
//	(asset "tank"
//	  (version 2)
//	  (kind "enemy")
//	  (size 64 64)
//	  (origin 32 32)
//	  (layer "hull" (stroke "#80ffff") (width 2) (fill "transparent") (glow 1.4)
//	    (path (move 8 8) (line 56 8) (close)))
//	  (hardpoint "muzzle" "weapon" 32 4 -90)
//	  (collision "circle" 32 32 20)
//	  (sound "fire" "shot_zap" (volume 0.3))
//	  (editor (grid) (grid-size 8)))
//
// Loading never migrates: the JSON era ended at version 2, so a Filo asset is born
// current. A future version bumps this constant and migrates in decodeAsset.

// assetBuiltins are the forms unique to an asset document.
func assetBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"asset":      biTagged("asset"),
		"layer":      biTagged("layer"),
		"origin":     biTagged("origin"),
		"hardpoint":  biTagged("hardpoint"),
		"collision":  biTagged("collision"),
		"sound":      biTagged("sound"),
		"volume":     biTagged("volume"),
		"continuous": biTagged("continuous"),
		"muted":      biTagged("muted"),
	}
}

// LoadAsset reads and validates an asset from a Filo document on disk.
func LoadAsset(path string) (*asset.Asset, error) {
	// #nosec G304 -- loading a user-supplied asset path is the purpose of this package
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	a, err := ParseAsset(string(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

// ParseAsset evaluates Filo source into a validated asset. Exported so the editor can
// parse an in-memory buffer, and so tests need no file.
func ParseAsset(src string) (*asset.Asset, error) {
	var out *asset.Asset
	var decodeErr error

	bi := assetBuiltins()
	bi["asset"] = func(_ context.Context, args []filo.Value) (filo.Value, error) {
		out, decodeErr = decodeAsset(args)
		return filo.VNum(0), nil
	}

	f, err := newEngine(shapeBuiltins(), commonBuiltins(), bi)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	err = f.DoString(src)
	if err != nil {
		return nil, err
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if out == nil {
		return nil, fmt.Errorf("no (asset ...) form found")
	}
	if out.Version > asset.CurrentVersion {
		return nil, fmt.Errorf("unsupported version %d (this build supports %d)", out.Version, asset.CurrentVersion)
	}
	err = asset.Validate(out)
	if err != nil {
		return nil, fmt.Errorf("invalid asset: %w", err)
	}
	return out, nil
}

// decodeAsset walks the tagged tree of an (asset "name" ...) form.
func decodeAsset(args []filo.Value) (*asset.Asset, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("asset: want (asset \"name\" ...)")
	}
	name, err := str("asset", args[0], "name")
	if err != nil {
		return nil, err
	}
	a := &asset.Asset{Name: name}
	for _, v := range args[1:] {
		tag, items, ok := tagOf(v)
		if !ok {
			return nil, fmt.Errorf("asset %q: unexpected element", name)
		}
		err = applyAssetField(a, tag, items)
		if err != nil {
			return nil, fmt.Errorf("asset %q: %w", name, err)
		}
	}
	return a, nil
}

// applyAssetField sets one top-level field from its tagged form.
func applyAssetField(a *asset.Asset, tag string, items []filo.Value) error {
	switch tag {
	case "version":
		v, err := one(tag, items)
		if err != nil {
			return err
		}
		a.Version = int(v)
	case "kind":
		v, err := oneStr(tag, items)
		if err != nil {
			return err
		}
		a.Kind = v
	case "tags":
		v, err := strs(tag, items)
		if err != nil {
			return err
		}
		a.Tags = v
	case "size":
		got, err := nums(tag, items, 2, "(size w h)")
		if err != nil {
			return err
		}
		a.Size = asset.Size{W: got[0], H: got[1]}
	case "origin":
		got, err := nums(tag, items, 2, "(origin x y)")
		if err != nil {
			return err
		}
		a.Origin = asset.Point{X: got[0], Y: got[1]}
	case "layer":
		l, err := decodeLayer("layer", items)
		if err != nil {
			return err
		}
		a.Layers = append(a.Layers, l)
	case "hardpoint":
		h, err := decodeHardpoint(items)
		if err != nil {
			return err
		}
		a.Hardpoints = append(a.Hardpoints, h)
	case "collision":
		c, err := decodeCollision(items)
		if err != nil {
			return err
		}
		a.Collisions = append(a.Collisions, c)
	case "sound":
		s, err := decodeSound(items)
		if err != nil {
			return err
		}
		a.Sounds = append(a.Sounds, s)
	case "editor":
		e, err := decodeEditor(items)
		if err != nil {
			return err
		}
		a.Editor = e
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// decodeHardpoint reads (hardpoint "name" "kind" x y angle).
func decodeHardpoint(items []filo.Value) (asset.Hardpoint, error) {
	if len(items) != 5 {
		return asset.Hardpoint{}, fmt.Errorf("hardpoint: want (hardpoint \"name\" \"kind\" x y angle)")
	}
	name, err := str("hardpoint", items[0], "name")
	if err != nil {
		return asset.Hardpoint{}, err
	}
	kind, err := str("hardpoint", items[1], "kind")
	if err != nil {
		return asset.Hardpoint{}, err
	}
	got, err := numbers(items[2:])
	if err != nil {
		return asset.Hardpoint{}, fmt.Errorf("hardpoint %q: %w", name, err)
	}
	return asset.Hardpoint{Name: name, Kind: kind, X: got[0], Y: got[1], Angle: got[2]}, nil
}

// collisionArity is how many numbers each hitbox primitive carries. The kind decides
// the shape, so the form stays positional and no radius/point bookkeeping leaks out.
var collisionArity = map[string]int{
	asset.CollisionCircle:   3, // cx cy r
	asset.CollisionRect:     4, // x1 y1 x2 y2
	asset.CollisionTriangle: 6, // x1 y1 x2 y2 x3 y3
}

// decodeCollision reads (collision "circle" cx cy r) and its rect/triangle siblings.
func decodeCollision(items []filo.Value) (asset.CollisionShape, error) {
	if len(items) == 0 {
		return asset.CollisionShape{}, fmt.Errorf("collision: want (collision \"kind\" ...)")
	}
	kind, err := str("collision", items[0], "kind")
	if err != nil {
		return asset.CollisionShape{}, err
	}
	want, ok := collisionArity[kind]
	if !ok {
		return asset.CollisionShape{}, fmt.Errorf("collision: unknown kind %q", kind)
	}
	got, err := nums("collision "+kind, items[1:], want, fmt.Sprintf("%d numbers", want))
	if err != nil {
		return asset.CollisionShape{}, err
	}
	if kind == asset.CollisionCircle {
		return asset.CollisionShape{Kind: kind, Points: []asset.Point{{X: got[0], Y: got[1]}}, Radius: got[2]}, nil
	}
	pts := make([]asset.Point, 0, want/2)
	for i := 0; i < want; i += 2 {
		pts = append(pts, asset.Point{X: got[i], Y: got[i+1]})
	}
	return asset.CollisionShape{Kind: kind, Points: pts}, nil
}

// decodeSound reads (sound "event" "base" (seed n) (volume v) (continuous) (muted)).
func decodeSound(items []filo.Value) (asset.Sound, error) {
	if len(items) < 2 {
		return asset.Sound{}, fmt.Errorf("sound: want (sound \"event\" \"base\" ...)")
	}
	event, err := str("sound", items[0], "event")
	if err != nil {
		return asset.Sound{}, err
	}
	base, err := str("sound", items[1], "base")
	if err != nil {
		return asset.Sound{}, err
	}
	s := asset.Sound{Event: event, Base: base}
	for _, v := range items[2:] {
		tag, sub, ok := tagOf(v)
		if !ok {
			return asset.Sound{}, fmt.Errorf("sound %q: unexpected element", event)
		}
		err = applySoundField(&s, tag, sub)
		if err != nil {
			return asset.Sound{}, fmt.Errorf("sound %q: %w", event, err)
		}
	}
	return s, nil
}

// applySoundField sets one optional field of a sound.
func applySoundField(s *asset.Sound, tag string, sub []filo.Value) error {
	switch tag {
	case "seed":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		s.Seed = int64(v)
	case "volume":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		s.Volume = v
	case "continuous":
		if len(sub) != 0 {
			return fmt.Errorf("continuous: want (continuous)")
		}
		s.Continuous = true
	case "muted":
		if len(sub) != 0 {
			return fmt.Errorf("muted: want (muted)")
		}
		s.Muted = true
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// decodeEditor reads (editor (snap) (snap-radius 8) (grid) (grid-size 8) (snap-to-grid)).
func decodeEditor(items []filo.Value) (asset.EditorSettings, error) {
	var e asset.EditorSettings
	for _, v := range items {
		tag, sub, ok := tagOf(v)
		if !ok {
			return e, fmt.Errorf("editor: unexpected element")
		}
		err := applyEditorField(&e, tag, sub)
		if err != nil {
			return e, fmt.Errorf("editor: %w", err)
		}
	}
	return e, nil
}

// applyEditorField sets one editor preference; the three bools are presence flags.
func applyEditorField(e *asset.EditorSettings, tag string, sub []filo.Value) error {
	flags := map[string]*bool{"snap": &e.SnapEnabled, "grid": &e.GridEnabled, "snap-to-grid": &e.SnapToGrid}
	flag, isFlag := flags[tag]
	if isFlag {
		if len(sub) != 0 {
			return fmt.Errorf("%s: want (%s)", tag, tag)
		}
		*flag = true
		return nil
	}
	switch tag {
	case "snap-radius":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		e.SnapRadiusPx = v
	case "grid-size":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		e.GridSize = v
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// SaveAsset validates and writes an asset as Filo text.
func SaveAsset(path string, a *asset.Asset) error {
	err := asset.Validate(a)
	if err != nil {
		return fmt.Errorf("refusing to save invalid asset: %w", err)
	}
	// #nosec G306 -- asset documents are project files, world-readable by design
	err = os.WriteFile(path, []byte(EmitAsset(a)), 0o644)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// EmitAsset renders an asset as Filo text that ParseAsset reads back identically. The
// round trip is the contract the editor rests on: a lossy save destroys the author's
// work, so filoio_test walks every real asset in gameassets and proves it.
func EmitAsset(a *asset.Asset) string {
	var sb strings.Builder
	b := builder{&sb}
	b.printf("(asset %q\n", a.Name)
	b.printf("  (version %d)\n", a.Version)
	if a.Kind != "" {
		b.printf("  (kind %q)\n", a.Kind)
	}
	if len(a.Tags) > 0 {
		b.printf("  (tags")
		for _, t := range a.Tags {
			b.printf(" %q", t)
		}
		b.printf(")\n")
	}
	b.printf("  (size %s %s)\n", num(a.Size.W), num(a.Size.H))
	b.printf("  (origin %s %s)\n", num(a.Origin.X), num(a.Origin.Y))
	for _, l := range a.Layers {
		emitLayer(b, "  ", "layer", l)
	}
	for _, h := range a.Hardpoints {
		b.printf("  (hardpoint %q %q %s %s %s)\n", h.Name, h.Kind, num(h.X), num(h.Y), num(h.Angle))
	}
	for _, c := range a.Collisions {
		emitCollision(b, c)
	}
	for _, s := range a.Sounds {
		emitSound(b, s)
	}
	emitEditor(b, a.Editor)
	b.printf(")\n")
	return sb.String()
}

// emitCollision writes a hitbox, its numbers laid out by kind (see collisionArity).
func emitCollision(b builder, c asset.CollisionShape) {
	b.printf("  (collision %q", c.Kind)
	for _, p := range c.Points {
		b.printf(" %s %s", num(p.X), num(p.Y))
	}
	if c.Kind == asset.CollisionCircle {
		b.printf(" %s", num(c.Radius))
	}
	b.printf(")\n")
}

// emitSound writes a sound, omitting every field left at its zero.
func emitSound(b builder, s asset.Sound) {
	b.printf("  (sound %q %q", s.Event, s.Base)
	if s.Seed != 0 {
		b.printf(" (seed %d)", s.Seed)
	}
	if s.Volume != 0 {
		b.printf(" (volume %s)", num(s.Volume))
	}
	if s.Continuous {
		b.printf(" (continuous)")
	}
	if s.Muted {
		b.printf(" (muted)")
	}
	b.printf(")\n")
}

// emitEditor writes the editor preferences; the bools appear only when set.
func emitEditor(b builder, e asset.EditorSettings) {
	b.printf("  (editor")
	if e.SnapEnabled {
		b.printf(" (snap)")
	}
	if e.SnapRadiusPx != 0 {
		b.printf(" (snap-radius %s)", num(e.SnapRadiusPx))
	}
	if e.GridEnabled {
		b.printf(" (grid)")
	}
	if e.GridSize != 0 {
		b.printf(" (grid-size %s)", num(e.GridSize))
	}
	if e.SnapToGrid {
		b.printf(" (snap-to-grid)")
	}
	b.printf(")\n")
}
