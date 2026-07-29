package filoio

import (
	"context"
	"fmt"

	"github.com/crgimenes/filo"

	"github.com/crgimenes/linefire/asset"
)

// The shape grammar, shared by the asset document and the level document (a level's
// walls are asset.Layer values). Path commands name the curve rather than a one-letter
// op, and the endpoint always comes LAST, the way SVG reads:
//
//	(layer "walls"
//	  (stroke "#80ffff") (width 2) (fill "transparent") (glow 1.4) (hidden)
//	  (path (move 10 10) (line 90 10) (quad 95 50 90 90) (cubic 60 95 20 95 10 90) (close))
//	  (rect 20 20 200 120)          ; sugar for a closed rectangle
//	  (circle 300 200 60))          ; sugar for a closed circle (4 cubic arcs)
//
// hidden is a presence flag: written only when true. rect and circle are GENERATORS:
// they run at load and expand to a (path ...), so the model — and therefore a Save —
// holds explicit geometry, exactly as kutta's (naca ...) resolves to points. With
// arithmetic they compose, e.g. (rect m m (- w (* 2 m)) (- h (* 2 m))) for an inset.

// shapeBuiltins are the forms of the shape grammar. Both documents register them, so
// one dialect draws a ship's hull and a map's walls. The form that OPENS a layer is not
// here: an asset calls it (layer ...), a map calls it (wall ...), and the contents are
// identical — the name says what the geometry is for.
func shapeBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"path":   biTagged("path"),
		"move":   biTagged("move"),
		"line":   biTagged("line"),
		"quad":   biTagged("quad"),
		"cubic":  biTagged("cubic"),
		"close":  biTagged("close"),
		"rect":   biRect,
		"circle": biCircle,
		"stroke": biTagged("stroke"),
		"width":  biTagged("width"),
		"fill":   biTagged("fill"),
		"glow":   biTagged("glow"),
		"hidden": biTagged("hidden"),
	}
}

// vnums turns floats back into filo values, so a generator can emit the same (move ..)
// and (line ..) shapes the hand-written forms produce — and decodeLayer stays unaware
// that a path was computed rather than typed.
func vnums(xs ...float64) []filo.Value {
	out := make([]filo.Value, len(xs))
	for i, x := range xs {
		out[i] = filo.VNum(x)
	}
	return out
}

// biRect expands (rect x y w h) into a closed rectangle path, corner at (x, y).
func biRect(_ context.Context, args []filo.Value) (filo.Value, error) {
	n, err := numbers(args)
	if err != nil || len(n) != 4 {
		return filo.Value{}, fmt.Errorf("rect: want (rect x y w h)")
	}
	x, y, w, h := n[0], n[1], n[2], n[3]
	return tagged("path",
		tagged("move", vnums(x, y)...),
		tagged("line", vnums(x+w, y)...),
		tagged("line", vnums(x+w, y+h)...),
		tagged("line", vnums(x, y+h)...),
		tagged("close"),
	), nil
}

// circleK is the control-point offset that makes a cubic Bézier quarter-arc round: a
// circle drawn as four such arcs is visually exact (max radius error < 0.02%).
const circleK = 0.5522847498307936

// biCircle expands (circle cx cy r) into a closed circle path — four cubic arcs from
// the rightmost point, clockwise. Curves flatten downstream (render, flood fill), so
// there is no segment count to pick.
func biCircle(_ context.Context, args []filo.Value) (filo.Value, error) {
	n, err := numbers(args)
	if err != nil || len(n) != 3 {
		return filo.Value{}, fmt.Errorf("circle: want (circle cx cy r)")
	}
	cx, cy, r := n[0], n[1], n[2]
	k := r * circleK
	return tagged("path",
		tagged("move", vnums(cx+r, cy)...),
		tagged("cubic", vnums(cx+r, cy+k, cx+k, cy+r, cx, cy+r)...),
		tagged("cubic", vnums(cx-k, cy+r, cx-r, cy+k, cx-r, cy)...),
		tagged("cubic", vnums(cx-r, cy-k, cx-k, cy-r, cx, cy-r)...),
		tagged("cubic", vnums(cx+k, cy-r, cx+r, cy-k, cx+r, cy)...),
		tagged("close"),
	), nil
}

// commonBuiltins are the forms both documents share outside the shape grammar.
func commonBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"version":      biTagged("version"),
		"kind":         biTagged("kind"),
		"tags":         biTagged("tags"),
		"size":         biTagged("size"),
		"seed":         biTagged("seed"),
		"editor":       biTagged("editor"),
		"snap":         biTagged("snap"),
		"snap-radius":  biTagged("snap-radius"),
		"grid":         biTagged("grid"),
		"grid-size":    biTagged("grid-size"),
		"snap-to-grid": biTagged("snap-to-grid"),
	}
}

// decodeCommand turns one path element into a drawing command. The control points of a
// curve precede its endpoint, so (quad cx cy x y) and (cubic c1x c1y c2x c2y x y).
func decodeCommand(v filo.Value) (asset.Command, error) {
	tag, items, ok := tagOf(v)
	if !ok {
		return asset.Command{}, fmt.Errorf("path: expected a command, e.g. (line x y)")
	}
	switch tag {
	case "move", "line":
		got, err := nums(tag, items, 2, "("+tag+" x y)")
		if err != nil {
			return asset.Command{}, err
		}
		op := asset.OpMoveTo
		if tag == "line" {
			op = asset.OpLineTo
		}
		return asset.Command{Op: op, X: got[0], Y: got[1]}, nil

	case "quad":
		got, err := nums(tag, items, 4, "(quad cx cy x y)")
		if err != nil {
			return asset.Command{}, err
		}
		return asset.Command{Op: asset.OpQuadTo, X: got[2], Y: got[3],
			Ctrl: []asset.Point{{X: got[0], Y: got[1]}}}, nil

	case "cubic":
		got, err := nums(tag, items, 6, "(cubic c1x c1y c2x c2y x y)")
		if err != nil {
			return asset.Command{}, err
		}
		return asset.Command{Op: asset.OpCubicTo, X: got[4], Y: got[5],
			Ctrl: []asset.Point{{X: got[0], Y: got[1]}, {X: got[2], Y: got[3]}}}, nil

	case "close":
		if len(items) != 0 {
			return asset.Command{}, fmt.Errorf("close: want (close)")
		}
		return asset.Command{Op: asset.OpClose}, nil
	}
	return asset.Command{}, fmt.Errorf("path: unknown command %q", tag)
}

// decodePath collects the commands of one (path ...) form.
func decodePath(items []filo.Value) (asset.Path, error) {
	cmds := make([]asset.Command, 0, len(items))
	for _, it := range items {
		c, err := decodeCommand(it)
		if err != nil {
			return asset.Path{}, err
		}
		cmds = append(cmds, c)
	}
	return asset.Path{Commands: cmds}, nil
}

// decodeLayer reads (layer "name" (stroke ...) ... (path ...) ...); form is the caller's
// name for it, so a map's error message says "wall" and not "layer".
func decodeLayer(form string, items []filo.Value) (asset.Layer, error) {
	if len(items) == 0 {
		return asset.Layer{}, fmt.Errorf("%s: want (%s \"name\" ...)", form, form)
	}
	name, err := str(form, items[0], "name")
	if err != nil {
		return asset.Layer{}, err
	}
	l := asset.Layer{Name: name}
	for _, it := range items[1:] {
		tag, sub, ok := tagOf(it)
		if !ok {
			return asset.Layer{}, fmt.Errorf("%s %q: unexpected element", form, name)
		}
		err = applyLayerField(&l, tag, sub)
		if err != nil {
			return asset.Layer{}, fmt.Errorf("%s %q: %w", form, name, err)
		}
	}
	return l, nil
}

// applyLayerField sets one field of a layer from its tagged form.
func applyLayerField(l *asset.Layer, tag string, sub []filo.Value) error {
	switch tag {
	case "stroke":
		v, err := oneStr(tag, sub)
		if err != nil {
			return err
		}
		l.Stroke = v
	case "fill":
		v, err := oneStr(tag, sub)
		if err != nil {
			return err
		}
		l.Fill = v
	case "width":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		l.StrokeWidth = v
	case "glow":
		v, err := one(tag, sub)
		if err != nil {
			return err
		}
		l.Glow = v
	case "hidden":
		if len(sub) != 0 {
			return fmt.Errorf("hidden: want (hidden)")
		}
		l.Hidden = true
	case "path":
		p, err := decodePath(sub)
		if err != nil {
			return err
		}
		l.Paths = append(l.Paths, p)
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// emitLayer writes a layer under the caller's form name. Paths go one per line: they
// are the bulk of a document, and a diff of a map should show which path moved.
func emitLayer(b builder, indent, form string, l asset.Layer) {
	b.printf("%s(%s %q\n", indent, form, l.Name)
	b.printf("%s  (stroke %q) (width %s) (fill %q) (glow %s)", indent, l.Stroke, num(l.StrokeWidth), l.Fill, num(l.Glow))
	if l.Hidden {
		b.printf(" (hidden)")
	}
	for _, p := range l.Paths {
		b.printf("\n%s  (path", indent)
		for _, c := range p.Commands {
			emitCommand(b, c)
		}
		b.printf(")")
	}
	b.printf(")\n")
}

// emitCommand writes one drawing command, control points before the endpoint.
func emitCommand(b builder, c asset.Command) {
	switch c.Op {
	case asset.OpMoveTo:
		b.printf(" (move %s %s)", num(c.X), num(c.Y))
	case asset.OpLineTo:
		b.printf(" (line %s %s)", num(c.X), num(c.Y))
	case asset.OpQuadTo:
		b.printf(" (quad %s %s %s %s)", num(c.Ctrl[0].X), num(c.Ctrl[0].Y), num(c.X), num(c.Y))
	case asset.OpCubicTo:
		b.printf(" (cubic %s %s %s %s %s %s)",
			num(c.Ctrl[0].X), num(c.Ctrl[0].Y), num(c.Ctrl[1].X), num(c.Ctrl[1].Y), num(c.X), num(c.Y))
	case asset.OpClose:
		b.printf(" (close)")
	}
}
