package filoio

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crgimenes/filo"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// The level document. Walls are the shape grammar under the name (wall ...); the rest
// follows the asset's conventions — a positional identity, then tagged options, with a
// bool as a presence flag and the kind of a shape deciding its arity:
//
//	(level "map0001"
//	  (version 2)
//	  (size 1480 2660)
//	  (music "music/One_Heart_Remaining.mp3")
//	  (player-start 100 100 -90)
//	  (entry "north" 40 10 -90)
//	  (wall "outer" (stroke "#80ffff") (width 2) (fill "transparent") (glow 1.4)
//	    (path (move 20 20) (line 460 20) (close)))
//	  (spawn "e1" "tank" 300 220 90 (kind "enemy"))
//	  (spawn "exit" "portal" 480 40 0 (kind "portal") (target "map0002:north"))
//	  (zone "cp" "circle" 200 200 10 (trigger "checkpoint"))
//	  (resolution "cleared" "spawn" (target "shield") (at 120 120))
//	  (horde (types "enemy" "rusher") (interval 24) (max-alive 18) (ramp 600) (seed 7))
//	  (editor (snap) (snap-radius 8) (grid) (grid-size 8)))
//
// A spawn's asset is a NAME, not a file path: the loader knows where assets live and
// what extension they carry. The v1 enemies/power_ups split died with JSON, so nothing
// here migrates.

// levelBuiltins are the forms unique to a level document.
func levelBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"level":        biTagged("level"),
		"title":        biTagged("title"),
		"wall":         biTagged("wall"),
		"player-start": biTagged("player-start"),
		"entry":        biTagged("entry"),
		"spawn":        biTagged("spawn"),
		"zone":         biTagged("zone"),
		"boss":         biTagged("boss"),
		"resolution":   biTagged("resolution"),
		"horde":        biTagged("horde"),
		"music":        biTagged("music"),
		"music-seed":   biTagged("music-seed"),
		"target":       biTagged("target"),
		"trigger":      biTagged("trigger"),
		"at":           biTagged("at"),
		"types":        biTagged("types"),
		"interval":     biTagged("interval"),
		"max-alive":    biTagged("max-alive"),
		"ramp":         biTagged("ramp"),
	}
}

// LoadLevel reads and validates a level from a Filo document on disk.
func LoadLevel(path string) (*level.Level, error) {
	// #nosec G304 -- loading a user-supplied level path is the purpose of this package
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	l, err := ParseLevel(string(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// ParseLevel evaluates Filo source into a validated level.
func ParseLevel(src string) (*level.Level, error) {
	var out *level.Level
	var decodeErr error

	bi := levelBuiltins()
	bi["level"] = func(_ context.Context, args []filo.Value) (filo.Value, error) {
		out, decodeErr = decodeLevel(args)
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
		return nil, fmt.Errorf("no (level ...) form found")
	}
	if out.Version > level.CurrentVersion {
		return nil, fmt.Errorf("unsupported version %d (this build supports %d)", out.Version, level.CurrentVersion)
	}
	err = level.Validate(out)
	if err != nil {
		return nil, fmt.Errorf("invalid level: %w", err)
	}
	return out, nil
}

// decodeLevel walks the tagged tree of a (level "name" ...) form.
func decodeLevel(args []filo.Value) (*level.Level, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("level: want (level \"name\" ...)")
	}
	name, err := str("level", args[0], "name")
	if err != nil {
		return nil, err
	}
	l := &level.Level{Name: name}
	for _, v := range args[1:] {
		tag, items, ok := tagOf(v)
		if !ok {
			return nil, fmt.Errorf("level %q: unexpected element", name)
		}
		err = applyLevelField(l, tag, items)
		if err != nil {
			return nil, fmt.Errorf("level %q: %w", name, err)
		}
	}
	return l, nil
}

// applyLevelField sets one top-level field from its tagged form.
func applyLevelField(l *level.Level, tag string, items []filo.Value) error {
	simple, err := applyLevelHeader(l, tag, items)
	if err != nil || simple {
		return err
	}
	switch tag {
	case "wall":
		w, err := decodeLayer("wall", items)
		if err != nil {
			return err
		}
		l.Walls = append(l.Walls, w)
	case "entry":
		e, err := decodeEntry(items)
		if err != nil {
			return err
		}
		l.Entries = append(l.Entries, e)
	case "spawn":
		s, err := decodeSpawn(items)
		if err != nil {
			return err
		}
		l.Spawns = append(l.Spawns, s)
	case "zone":
		z, err := decodeZone(items)
		if err != nil {
			return err
		}
		l.Zones = append(l.Zones, z)
	case "resolution":
		r, err := decodeResolution(items)
		if err != nil {
			return err
		}
		l.Resolutions = append(l.Resolutions, r)
	case "horde":
		h, err := decodeHorde(items)
		if err != nil {
			return err
		}
		l.Horde = &h
	case "editor":
		e, err := decodeEditor(items)
		if err != nil {
			return err
		}
		l.Editor = e
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// applyLevelHeader handles the scalar fields, reporting whether it recognised the tag.
// Split out so applyLevelField stays within the project's complexity budget.
func applyLevelHeader(l *level.Level, tag string, items []filo.Value) (bool, error) {
	switch tag {
	case "version":
		v, err := one(tag, items)
		if err != nil {
			return true, err
		}
		l.Version = int(v)
	case "title":
		v, err := oneStr(tag, items)
		if err != nil {
			return true, err
		}
		l.Title = v
	case "tags":
		v, err := strs(tag, items)
		if err != nil {
			return true, err
		}
		l.Tags = v
	case "size":
		got, err := nums(tag, items, 2, "(size w h)")
		if err != nil {
			return true, err
		}
		l.Size = asset.Size{W: got[0], H: got[1]}
	case "music":
		v, err := oneStr(tag, items)
		if err != nil {
			return true, err
		}
		l.Music = v
	case "music-seed":
		v, err := one(tag, items)
		if err != nil {
			return true, err
		}
		l.MusicSeed = int64(v)
	case "player-start":
		got, err := nums(tag, items, 3, "(player-start x y angle)")
		if err != nil {
			return true, err
		}
		l.PlayerStart = level.Start{X: got[0], Y: got[1], Angle: got[2]}
	default:
		return false, nil
	}
	return true, nil
}

// decodeEntry reads (entry "name" x y angle).
func decodeEntry(items []filo.Value) (level.Entry, error) {
	if len(items) != 4 {
		return level.Entry{}, fmt.Errorf("entry: want (entry \"name\" x y angle)")
	}
	name, err := str("entry", items[0], "name")
	if err != nil {
		return level.Entry{}, err
	}
	got, err := numbers(items[1:])
	if err != nil {
		return level.Entry{}, fmt.Errorf("entry %q: %w", name, err)
	}
	return level.Entry{Name: name, X: got[0], Y: got[1], Angle: got[2]}, nil
}

// decodeSpawn reads (spawn "name" "asset" x y angle (kind "...") (target "...")).
func decodeSpawn(items []filo.Value) (level.Spawn, error) {
	if len(items) < 5 {
		return level.Spawn{}, fmt.Errorf("spawn: want (spawn \"name\" \"asset\" x y angle ...)")
	}
	name, err := str("spawn", items[0], "name")
	if err != nil {
		return level.Spawn{}, err
	}
	ref, err := str("spawn", items[1], "asset")
	if err != nil {
		return level.Spawn{}, err
	}
	got, err := numbers(items[2:5])
	if err != nil {
		return level.Spawn{}, fmt.Errorf("spawn %q: %w", name, err)
	}
	s := level.Spawn{Name: name, Asset: ref, X: got[0], Y: got[1], Angle: got[2]}
	for _, v := range items[5:] {
		tag, sub, ok := tagOf(v)
		if !ok {
			return level.Spawn{}, fmt.Errorf("spawn %q: unexpected element", name)
		}
		// boss is a presence flag: (boss) carries no argument, unlike (kind ...)/(target ...).
		if tag == "boss" {
			if len(sub) != 0 {
				return level.Spawn{}, fmt.Errorf("spawn %q: want (boss)", name)
			}
			s.Boss = true
			continue
		}
		text, err := oneStr(tag, sub)
		if err != nil {
			return level.Spawn{}, fmt.Errorf("spawn %q: %w", name, err)
		}
		switch tag {
		case "kind":
			s.Kind = text
		case "target":
			s.Target = text
		default:
			return level.Spawn{}, fmt.Errorf("spawn %q: unknown element %q", name, tag)
		}
	}
	return s, nil
}

// zoneArity mirrors collisionArity: the shape's kind decides how many numbers follow.
var zoneArity = map[string]int{
	level.ZoneCircle: 3, // cx cy r
	level.ZoneRect:   4, // x1 y1 x2 y2
}

// decodeZone reads (zone "name" "circle" cx cy r (trigger "...")).
func decodeZone(items []filo.Value) (level.Zone, error) {
	if len(items) < 2 {
		return level.Zone{}, fmt.Errorf("zone: want (zone \"name\" \"kind\" ...)")
	}
	name, err := str("zone", items[0], "name")
	if err != nil {
		return level.Zone{}, err
	}
	kind, err := str("zone", items[1], "kind")
	if err != nil {
		return level.Zone{}, err
	}
	want, ok := zoneArity[kind]
	if !ok {
		return level.Zone{}, fmt.Errorf("zone %q: unknown kind %q", name, kind)
	}
	if len(items) < 2+want {
		return level.Zone{}, fmt.Errorf("zone %q: a %s needs %d numbers", name, kind, want)
	}
	got, err := numbers(items[2 : 2+want])
	if err != nil {
		return level.Zone{}, fmt.Errorf("zone %q: %w", name, err)
	}
	z := level.Zone{Name: name, Kind: kind}
	if kind == level.ZoneCircle {
		z.Points = []asset.Point{{X: got[0], Y: got[1]}}
		z.Radius = got[2]
	} else {
		z.Points = []asset.Point{{X: got[0], Y: got[1]}, {X: got[2], Y: got[3]}}
	}
	for _, v := range items[2+want:] {
		tag, sub, ok := tagOf(v)
		if !ok || tag != "trigger" {
			return level.Zone{}, fmt.Errorf("zone %q: want (trigger \"event\")", name)
		}
		z.Trigger, err = oneStr(tag, sub)
		if err != nil {
			return level.Zone{}, fmt.Errorf("zone %q: %w", name, err)
		}
	}
	return z, nil
}

// decodeResolution reads (resolution "cleared" "spawn" (target "shield") (at x y)).
func decodeResolution(items []filo.Value) (level.Resolution, error) {
	if len(items) < 2 {
		return level.Resolution{}, fmt.Errorf("resolution: want (resolution \"on\" \"do\" ...)")
	}
	on, err := str("resolution", items[0], "condition")
	if err != nil {
		return level.Resolution{}, err
	}
	do, err := str("resolution", items[1], "routine")
	if err != nil {
		return level.Resolution{}, err
	}
	r := level.Resolution{On: on, Do: do}
	for _, v := range items[2:] {
		tag, sub, ok := tagOf(v)
		if !ok {
			return level.Resolution{}, fmt.Errorf("resolution %q: unexpected element", on)
		}
		switch tag {
		case "target":
			r.Target, err = oneStr(tag, sub)
		case "at":
			var got []float64
			got, err = nums(tag, sub, 2, "(at x y)")
			if err == nil {
				r.X, r.Y = got[0], got[1]
			}
		default:
			err = fmt.Errorf("unknown element %q", tag)
		}
		if err != nil {
			return level.Resolution{}, fmt.Errorf("resolution %q: %w", on, err)
		}
	}
	return r, nil
}

// decodeHorde reads (horde (types ...) (interval n) (max-alive n) (ramp n) (seed n)).
func decodeHorde(items []filo.Value) (level.Horde, error) {
	var h level.Horde
	for _, v := range items {
		tag, sub, ok := tagOf(v)
		if !ok {
			return h, fmt.Errorf("horde: unexpected element")
		}
		err := applyHordeField(&h, tag, sub)
		if err != nil {
			return h, fmt.Errorf("horde: %w", err)
		}
	}
	return h, nil
}

// applyHordeField sets one field of the spawner.
func applyHordeField(h *level.Horde, tag string, sub []filo.Value) error {
	if tag == "types" {
		v, err := strs(tag, sub)
		if err != nil {
			return err
		}
		h.Types = v
		return nil
	}
	v, err := one(tag, sub)
	if err != nil {
		return err
	}
	switch tag {
	case "interval":
		h.Interval = int(v)
	case "max-alive":
		h.MaxAlive = int(v)
	case "ramp":
		h.Ramp = int(v)
	case "seed":
		h.Seed = int64(v)
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// SaveLevel validates and writes a level as Filo text.
func SaveLevel(path string, l *level.Level) error {
	err := level.Validate(l)
	if err != nil {
		return fmt.Errorf("refusing to save invalid level: %w", err)
	}
	// #nosec G306 -- level documents are project files, world-readable by design
	err = os.WriteFile(path, []byte(EmitLevel(l)), 0o644)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// EmitLevel renders a level as Filo text that ParseLevel reads back identically.
func EmitLevel(l *level.Level) string {
	var sb strings.Builder
	b := builder{&sb}
	b.printf("(level %q\n", l.Name)
	b.printf("  (version %d)\n", l.Version)
	if l.Title != "" {
		b.printf("  (title %q)\n", l.Title)
	}
	if len(l.Tags) > 0 {
		b.printf("  (tags")
		for _, t := range l.Tags {
			b.printf(" %q", t)
		}
		b.printf(")\n")
	}
	b.printf("  (size %s %s)\n", num(l.Size.W), num(l.Size.H))
	if l.Music != "" {
		b.printf("  (music %q)\n", l.Music)
	}
	if l.MusicSeed != 0 {
		b.printf("  (music-seed %d)\n", l.MusicSeed)
	}
	b.printf("  (player-start %s %s %s)\n", num(l.PlayerStart.X), num(l.PlayerStart.Y), num(l.PlayerStart.Angle))
	for _, e := range l.Entries {
		b.printf("  (entry %q %s %s %s)\n", e.Name, num(e.X), num(e.Y), num(e.Angle))
	}
	for _, w := range l.Walls {
		emitLayer(b, "  ", "wall", w)
	}
	for _, s := range l.Spawns {
		emitSpawn(b, s)
	}
	for _, z := range l.Zones {
		emitZone(b, z)
	}
	for _, r := range l.Resolutions {
		emitResolution(b, r)
	}
	if l.Horde != nil {
		emitHorde(b, *l.Horde)
	}
	emitEditor(b, l.Editor)
	b.printf(")\n")
	return sb.String()
}

// emitSpawn writes one placed asset.
func emitSpawn(b builder, s level.Spawn) {
	b.printf("  (spawn %q %q %s %s %s", s.Name, s.Asset, num(s.X), num(s.Y), num(s.Angle))
	if s.Kind != "" {
		b.printf(" (kind %q)", s.Kind)
	}
	if s.Target != "" {
		b.printf(" (target %q)", s.Target)
	}
	if s.Boss {
		b.printf(" (boss)") // presence flag: written only when the enemy is the shielded boss
	}
	b.printf(")\n")
}

// emitZone writes one trigger region, its numbers laid out by kind (see zoneArity).
func emitZone(b builder, z level.Zone) {
	b.printf("  (zone %q %q", z.Name, z.Kind)
	for _, p := range z.Points {
		b.printf(" %s %s", num(p.X), num(p.Y))
	}
	if z.Kind == level.ZoneCircle {
		b.printf(" %s", num(z.Radius))
	}
	if z.Trigger != "" {
		b.printf(" (trigger %q)", z.Trigger)
	}
	b.printf(")\n")
}

// emitResolution writes one condition -> routine outcome.
func emitResolution(b builder, r level.Resolution) {
	b.printf("  (resolution %q %q", r.On, r.Do)
	if r.Target != "" {
		b.printf(" (target %q)", r.Target)
	}
	if r.X != 0 || r.Y != 0 {
		b.printf(" (at %s %s)", num(r.X), num(r.Y))
	}
	b.printf(")\n")
}

// emitHorde writes the spawner, omitting every field left at its zero.
func emitHorde(b builder, h level.Horde) {
	b.printf("  (horde")
	if len(h.Types) > 0 {
		b.printf(" (types")
		for _, t := range h.Types {
			b.printf(" %q", t)
		}
		b.printf(")")
	}
	pairs := []struct {
		tag string
		v   int64
	}{
		{"interval", int64(h.Interval)},
		{"max-alive", int64(h.MaxAlive)},
		{"ramp", int64(h.Ramp)},
		{"seed", h.Seed},
	}
	for _, p := range pairs {
		if p.v != 0 {
			b.printf(" (%s %d)", p.tag, p.v)
		}
	}
	b.printf(")\n")
}
