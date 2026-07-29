package editor

import (
	"fmt"
	"math"

	"github.com/crgimenes/linefire/asset"
)

// scopeID selects what a mirror/rotate transform applies to.
type scopeID int

const (
	scopeAsset scopeID = iota // every layer's geometry, hardpoints and collision
	scopeLayer                // only the current layer's paths
	scopePath                 // only the path of the selected vertex
)

// String renders the scope for the properties panel.
func (s scopeID) String() string {
	switch s {
	case scopeLayer:
		return "layer"
	case scopePath:
		return "path"
	default:
		return "asset"
	}
}

// cycleScope advances the transform scope: asset -> layer -> path -> asset.
func (e *Editor) cycleScope() {
	e.scope = (e.scope + 1) % 3
	e.status = "transform scope: " + e.scope.String()
}

// pointXform is a coordinate transform plus its effect on a hardpoint angle.
type pointXform struct {
	point func(x, y float64) (float64, float64)
	angle func(a float64) float64
}

// normalizeAngle wraps an angle in degrees to (-180, 180].
func normalizeAngle(a float64) float64 {
	for a > 180 {
		a -= 360
	}
	for a <= -180 {
		a += 360
	}
	return a
}

// flipHorizontal mirrors the current scope across the vertical axis through the
// origin.
func (e *Editor) flipHorizontal() {
	ox := e.asset.Origin.X
	ok := e.applyXform(pointXform{
		point: func(x, y float64) (float64, float64) { return 2*ox - x, y },
		angle: func(a float64) float64 { return normalizeAngle(180 - a) },
	})
	if ok {
		e.status = "flip H (" + e.scope.String() + ")"
	}
}

// flipVertical mirrors the current scope across the horizontal axis through the
// origin.
func (e *Editor) flipVertical() {
	oy := e.asset.Origin.Y
	ok := e.applyXform(pointXform{
		point: func(x, y float64) (float64, float64) { return x, 2*oy - y },
		angle: func(a float64) float64 { return normalizeAngle(-a) },
	})
	if ok {
		e.status = "flip V (" + e.scope.String() + ")"
	}
}

// cosSinDeg returns cosine and sine of an angle in degrees, using exact values
// for multiples of 90 so right-angle rotations keep integer coordinates clean.
func cosSinDeg(deg float64) (float64, float64) {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	switch d {
	case 0:
		return 1, 0
	case 90:
		return 0, 1
	case 180:
		return -1, 0
	case 270:
		return 0, -1
	}
	rad := deg * math.Pi / 180
	return math.Cos(rad), math.Sin(rad)
}

// rotate turns the current scope by deg degrees around the origin.
func (e *Editor) rotate(deg float64) {
	ox, oy := e.asset.Origin.X, e.asset.Origin.Y
	cos, sin := cosSinDeg(deg)
	ok := e.applyXform(pointXform{
		point: func(x, y float64) (float64, float64) {
			dx, dy := x-ox, y-oy
			return ox + dx*cos - dy*sin, oy + dx*sin + dy*cos
		},
		angle: func(a float64) float64 { return normalizeAngle(a + deg) },
	})
	if ok {
		e.status = fmt.Sprintf("rotate %g (%s)", deg, e.scope)
	}
}

// applyXform runs a transform over the current scope. It clones first and
// reverts if the result somehow fails validation, keeping the document valid.
// It returns false when there was nothing to transform.
func (e *Editor) applyXform(t pointXform) bool {
	before := e.asset.Clone()

	switch e.scope {
	case scopePath:
		p := e.selectedPath()
		if p == nil {
			e.status = "select a path point first"
			return false
		}
		xformPath(p, t)
	case scopeLayer:
		layer := e.currentLayer()
		for i := range layer.Paths {
			xformPath(&layer.Paths[i], t)
		}
	default: // scopeAsset
		for li := range e.asset.Layers {
			for pi := range e.asset.Layers[li].Paths {
				xformPath(&e.asset.Layers[li].Paths[pi], t)
			}
		}
		for i := range e.asset.Hardpoints {
			hp := &e.asset.Hardpoints[i]
			hp.X, hp.Y = t.point(hp.X, hp.Y)
			hp.Angle = t.angle(hp.Angle)
		}
		for si := range e.asset.Collisions {
			pts := e.asset.Collisions[si].Points
			for pi := range pts {
				pts[pi].X, pts[pi].Y = t.point(pts[pi].X, pts[pi].Y)
			}
		}
	}

	err := asset.Validate(e.asset)
	if err != nil {
		*e.asset = *before
		e.status = "transform rejected: " + err.Error()
		return false
	}
	e.markDirty()
	return true
}

// selectedPath returns the path of the currently selected vertex, or nil.
func (e *Editor) selectedPath() *asset.Path {
	if !e.hasActive || e.active.kind != handleVertex {
		return nil
	}
	if e.active.layer >= len(e.asset.Layers) {
		return nil
	}
	layer := &e.asset.Layers[e.active.layer]
	if e.active.path >= len(layer.Paths) {
		return nil
	}
	return &layer.Paths[e.active.path]
}

// xformPath applies a point transform to every drawable command of a path.
func xformPath(p *asset.Path, t pointXform) {
	for i := range p.Commands {
		c := &p.Commands[i]
		if c.Op == asset.OpClose {
			continue
		}
		c.X, c.Y = t.point(c.X, c.Y)
	}
}
